//go:build unix

package worktreecmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// Info classifies dir relative to git worktrees:
//   - in main repo: in_worktree=false, worktree=main row, main_root=project root
//   - in worktree: in_worktree=true, worktree=this worktree's row, main_root=main repo
//   - outside any repo: ok=false, error.kind=not_in_repo, exit=ExitNotInRepo
//
// dir is typically the current working directory but the signature lets
// callers (and tests) target a specific path.
func Info(ctx context.Context, dir string) (*InfoResult, error) {
	res := &InfoResult{
		Envelope: Envelope{
			SchemaVersion: SchemaVersion,
			Subcommand:    "info",
			Steps:         []Step{},
		},
	}
	// Use the consolidated initcmd detector (path-anchored variant).
	inWt, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}

	res.ProjectRoot = mainRoot
	res.MainRoot = mainRoot
	res.InWorktree = inWt

	// Resolve `dir` (which may be a subdirectory of the worktree) to the
	// worktree's root so the .lets / declared store link probes hit the
	// actual symlink locations. Falls back to the caller-supplied dir if
	// git can't resolve.
	probeRoot := dir
	if out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		if top := strings.TrimSpace(string(out)); top != "" {
			probeRoot = top
		}
	}

	branch := ""
	if out, err := exec.CommandContext(ctx, "git", "-C", probeRoot, "branch", "--show-current").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	headOut, _ := exec.CommandContext(ctx, "git", "-C", probeRoot, "rev-parse", "HEAD").Output()
	e := porcelainEntry{Path: probeRoot, Branch: branch, HEAD: strings.TrimSpace(string(headOut))}
	wt := annotateWorktree(ctx, mainRoot, e, loadStoreLinks(mainRoot, "", func(string, string) {}))
	if inWt {
		wt.Name = filepath.Base(probeRoot)
	}
	if slug, ok := taskstate.Slug(branch); ok {
		if st, err := taskstate.Read(filepath.Join(mainRoot, ".lets"), slug); err == nil && taskid.Valid(st.Task) {
			wt.Task = st.Task
		}
	}
	if cwd, err := os.Getwd(); err == nil && fsutil.SameDir(gitutil.ProjectRoot(cwd, 2*time.Second), probeRoot) {
		wt.OrcaWorktreeID = stripControl(os.Getenv("ORCA_WORKTREE_ID"))
	}
	res.Worktree = &wt
	res.OK = true
	return res, nil
}

// TaskCandidateFor returns ONLY the created-shape task candidate for dir's HEAD
// branch (or, with refFile, for the ref written in that file - the route for an
// untrusted ref such as a pull request head, which is never typed into a shell).
// No annotateWorktree, no git status: detect-task calls this on its hot path.
// pluginRoot is the adapter fallback ("" reads $CLAUDE_PLUGIN_ROOT).
func TaskCandidateFor(ctx context.Context, dir, refFile, pluginRoot string) (*InfoResult, error) {
	res := &InfoResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "info", Steps: []Step{}}}
	inWt, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	res.ProjectRoot, res.MainRoot, res.InWorktree = mainRoot, mainRoot, inWt
	cand := &TaskCandidate{}
	res.TaskCandidate = cand
	res.OK = true

	branch := ""
	if refFile != "" {
		data, err := os.ReadFile(refFile)
		if err != nil {
			cand.Reason = "ref_file_unreadable"
			return res, nil
		}
		ref := strings.TrimSpace(string(data))
		if ref == "" || strings.ContainsAny(ref, "\n\r") || exec.CommandContext(ctx, "git", "check-ref-format", "--branch", ref).Run() != nil {
			cand.Reason = "ref_invalid"
			return res, nil
		}
		branch = strings.TrimPrefix(ref, "refs/heads/")
	} else {
		branch = currentBranchOf(ctx, dir)
	}
	cand.Branch = branch
	if branch == "" {
		cand.Reason = "detached_head"
		return res, nil
	}
	home, _ := os.UserHomeDir()
	tracker := letsconfig.ResolvedEnv(mainRoot, home, nil)["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	pluginRoot, err := initcmd.DetectPluginRoot(pluginRoot)
	if err != nil {
		pluginRoot = ""
	}
	conv, reasons := trackeradapter.LoadConvention(mainRoot, tracker, pluginRoot)
	if !conv.Declared {
		cand.Reason = trackeradapter.ReasonConventionUndeclared
		if slices.Contains(reasons, trackeradapter.ReasonConventionDeclarationInvalid) {
			cand.Reason = trackeradapter.ReasonConventionDeclarationInvalid
		}
		return res, nil
	}
	id, tmpl, ok := conv.ParseBranch(branch, trackeradapter.Created)
	if !ok || !taskid.Valid(id) {
		cand.Reason = "no_match"
		return res, nil
	}
	cand.ID, cand.Template, cand.Source = id, tmpl, "created"
	return res, nil
}
