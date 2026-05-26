//go:build unix

package worktreecmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
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
	inWt, mainRoot := initcmd.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}

	res.ProjectRoot = mainRoot
	res.MainRoot = mainRoot
	res.InWorktree = inWt

	// Resolve `dir` (which may be a subdirectory of the worktree) to the
	// worktree's root so the .lets / .beads/.env symlink probes hit the
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
	wt := annotateWorktree(ctx, mainRoot, e)
	if inWt {
		wt.Name = filepath.Base(probeRoot)
	}
	res.Worktree = &wt
	res.OK = true
	return res, nil
}
