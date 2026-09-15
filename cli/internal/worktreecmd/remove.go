//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// hasUserChanges returns true if `git status --porcelain` output contains any line
// that is not LETS-managed: `.lets`, a `.lets.pre-adopt*` directory adopt moved
// aside, each declared store link, and each link's parent directory entry (git
// shows an untracked `?? .beads/` for a parent create or adopt made). They are
// untracked relative to the worktree's branch HEAD and would otherwise mask the
// "is this worktree dirty?" question.
func hasUserChanges(porcelain string, links []trackeradapter.Link) bool {
	for _, l := range strings.Split(porcelain, "\n") {
		if l == "" {
			continue
		}
		// Porcelain lines are "XY <path>" (2 status chars + space + path).
		if len(l) < 4 {
			return true
		}
		path := strings.TrimSuffix(strings.TrimSpace(l[2:]), "/")
		if path == ".lets" || strings.HasPrefix(path, ".lets.pre-adopt") || isDeclaredLink(path, links) {
			continue
		}
		return true
	}
	return false
}

// isDeclaredLink reports whether path is a declared store link or one of its parents.
func isDeclaredLink(path string, links []trackeradapter.Link) bool {
	for _, l := range links {
		if path == l.Path {
			return true
		}
		for dir := filepath.ToSlash(filepath.Dir(l.Path)); dir != "." && dir != "/"; dir = filepath.ToSlash(filepath.Dir(dir)) {
			if path == dir {
				return true
			}
		}
	}
	return false
}

// RemoveOptions configures the remove flow.
type RemoveOptions struct {
	Name         string
	Force        bool
	DeleteBranch bool
	ForceBranch  bool   // if DeleteBranch and target unmerged: use -D instead of -d
	BranchOnly   bool   // skip worktree removal; just delete the branch (R3 follow-up)
	Branch       string // explicit branch name when BranchOnly=true
}

// Remove tears down a worktree (or in --branch-only mode, just the branch).
// Re-derives the actual branch via `git -C <wtPath> branch --show-current` so
// attach-mode worktrees (branch != worktree-<name>) are handled correctly.
func Remove(ctx context.Context, projectRoot string, opts RemoveOptions) (*RemoveResult, error) {
	res := &RemoveResult{
		Envelope: Envelope{
			SchemaVersion: SchemaVersion,
			Subcommand:    "remove",
			ProjectRoot:   projectRoot,
			Steps:         []Step{},
		},
	}
	addStep := func(status, msg string) {
		res.Steps = append(res.Steps, Step{Status: status, Message: msg})
	}
	fail := func(e *Error) (*RemoveResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}

	if err := ValidateName(ctx, opts.Name); err != nil {
		var e *Error
		if errors.As(err, &e) {
			return fail(e)
		}
		return fail(&Error{Code: ExitUsage, Kind: "name_validation", Cause: err})
	}

	// --branch-only path: skip all worktree FS ops, just delete the branch.
	if opts.BranchOnly {
		if opts.Branch == "" {
			return fail(&Error{
				Code:    ExitUsage,
				Kind:    "branch_only_no_branch",
				Message: "--branch-only requires --branch <name>",
			})
		}
		if !opts.DeleteBranch {
			return fail(&Error{
				Code:    ExitUsage,
				Kind:    "branch_only_no_delete",
				Message: "--branch-only requires --delete-branch",
			})
		}
		msg, e := deleteBranch(ctx, projectRoot, opts.Branch, opts.ForceBranch, mergeBranch(projectRoot))
		if e != nil {
			return fail(e)
		}
		addStep(StepOK, msg)
		res.OK = true
		res.Removed = &RemovedInfo{Name: opts.Name, Branch: opts.Branch, BranchDeleted: true}
		return res, nil
	}

	// Full worktree+branch removal path.
	wtPath := filepath.Join(projectRoot, ".worktrees", opts.Name)
	if _, err := os.Lstat(wtPath); err != nil {
		registered := worktreePaths(ctx, projectRoot)
		switch {
		case slices.Contains(registered, wtPath):
			// Registered but the directory is gone: the normal flow prunes it.
		case slices.ContainsFunc(registered, func(p string) bool { return filepath.Base(p) == opts.Name }):
			return fail(&Error{
				Code:        ExitGeneric,
				Kind:        "worktree_external",
				Message:     fmt.Sprintf("worktree %q lives outside %s/.worktrees/", opts.Name, projectRoot),
				Remediation: "archive it in Orca or run lets worktree release from inside it",
			})
		default:
			return removeAlreadyGone(ctx, projectRoot, opts, res, addStep, fail)
		}
	}
	addStep(StepOK, fmt.Sprintf("found worktree at %s", wtPath))

	// Re-derive actual branch (attach case: branch != worktree-<name>).
	branchOut, _ := exec.CommandContext(ctx, "git", "-C", wtPath, "branch", "--show-current").Output()
	branch := strings.TrimSpace(string(branchOut))

	// Safety check unless --force. The LETS-managed `.lets` and declared store
	// symlinks always appear as untracked in the worktree (their branch HEAD
	// commit doesn't carry these paths). Filter them out so a freshly-created
	// worktree is not falsely classified as "dirty".
	statusOut, _ := exec.CommandContext(ctx, "git", "-C", wtPath, "status", "--porcelain").Output()
	dirty := hasUserChanges(string(statusOut), loadStoreLinks(projectRoot, "", func(string, string) {}))
	if dirty && !opts.Force {
		return fail(&Error{
			Code:        ExitDirtyWorktree,
			Kind:        "dirty_worktree",
			Message:     "worktree has uncommitted changes",
			Remediation: "commit, stash, or run with --force to discard",
		})
	}
	addStep(StepOK, "safety check (clean or --force)")

	// Unpushed-commits safety net (parity with pre-rewrite markdown Step R2).
	// `git log @{u}.. --oneline` lists local commits not present in the
	// upstream branch. No upstream configured -> command fails: we surface
	// a warn step and continue (caller can still see the branch will be
	// deleted if --delete-branch is set). Skipped under --force.
	if !opts.Force {
		out, err := exec.CommandContext(ctx, "git", "-C", wtPath, "log", "@{u}..", "--oneline").CombinedOutput()
		switch {
		case err == nil && len(strings.TrimSpace(string(out))) > 0:
			count := strings.Count(strings.TrimSpace(string(out)), "\n") + 1
			return fail(&Error{
				Code:        ExitUnpushedCommits,
				Kind:        "unpushed_commits",
				Message:     fmt.Sprintf("worktree has %d unpushed commit(s) on branch %q", count, branch),
				Remediation: "push the branch (or pass --force to discard them along with the worktree)",
			})
		case err != nil:
			// Most common cause: no upstream configured (e.g. attach-mode worktree
			// pointing at a local-only branch). Don't block, but flag it so the
			// JSON envelope makes the gap visible.
			addStep(StepWarn, "skipped unpushed-commits check (no upstream configured)")
		default:
			addStep(StepOK, "no unpushed commits on upstream")
		}
	}

	// Path-descendant guard before any destructive op.
	if !pathDescendantOfWorktrees(projectRoot, wtPath) {
		return fail(&Error{
			Code:    ExitFilesystem,
			Kind:    "remove_refused_path_escape",
			Message: fmt.Sprintf("path %q outside %s/.worktrees/; refusing", wtPath, projectRoot),
		})
	}

	if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "remove", "--force", wtPath).CombinedOutput(); err != nil {
		return fail(&Error{
			Code:    ExitGitFailed,
			Kind:    "git_worktree_remove_failed",
			Message: redactCreds(strings.TrimSpace(string(out))),
			Cause:   err,
		})
	}
	_ = exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "prune").Run()
	addStep(StepOK, "git worktree remove + prune")

	// Clean the per-branch task-state file (and the legacy session ref) now that the
	// worktree is gone. Keyed by the RE-DERIVED branch, so attach-mode worktrees
	// (branch != worktree-<name>) are cleaned too - the dir name alone would miss them.
	if branch != "" {
		cleanTaskState(projectRoot, branch)
		addStep(StepOK, "removed task-state file")
	}

	// Combined-mode: --delete-branch in same call also deletes the branch.
	branchDeleted := false
	if opts.DeleteBranch && branch != "" {
		msg, e := deleteBranch(ctx, projectRoot, branch, opts.ForceBranch, mergeBranch(projectRoot))
		if e != nil {
			return fail(e)
		}
		branchDeleted = true
		addStep(StepOK, msg)
	}

	res.OK = true
	res.Removed = &RemovedInfo{
		Name:                  opts.Name,
		Path:                  wtPath,
		Branch:                branch,
		BranchDeleted:         branchDeleted,
		HadUncommittedChanges: dirty,
		Forced:                opts.Force,
	}
	return res, nil
}

// deleteBranch is the ONE branch-delete step shared by --branch-only, the combined
// remove, and the already-gone path. Unforced, a branch that is an ancestor of
// origin/<merge> (or local <merge>) is deleted with -D: `git branch -d` measures
// "merged" against the LOCAL merge-branch, which lags its origin in a worktree setup
// and would refuse a branch that is merged upstream. Anything else keeps -d, which
// git measures against the branch's own upstream when it has one (else HEAD): a
// pushed branch is deleted - its commits live on the remote - and a branch with
// commits nowhere else fails with branch_unmerged.
func deleteBranch(ctx context.Context, root, branch string, force bool, merge string) (string, *Error) {
	flag, note := "-d", ""
	switch {
	case force:
		flag = "-D"
	default:
		if ok, ref := mergedUpstream(ctx, root, branch, merge); ok {
			flag = "-D"
			note = fmt.Sprintf("merged into %s (merge-base); ", strings.TrimPrefix(strings.TrimPrefix(ref, "refs/remotes/"), "refs/heads/"))
		}
	}
	// `--` between flags and the branch name prevents git from treating a branch
	// name that starts with "-" as another flag.
	out, err := exec.CommandContext(ctx, "git", "-C", root, "branch", flag, "--", branch).CombinedOutput()
	if err != nil {
		if flag == "-d" && strings.Contains(string(out), "not fully merged") {
			return "", &Error{
				Code:        ExitBranchUnmerged,
				Kind:        "branch_unmerged",
				Message:     fmt.Sprintf("branch %q has unmerged commits", branch),
				Remediation: fmt.Sprintf("not merged into origin/%s or %s, and not pushed to its upstream; a squash/rebase merge is not detectable - pass --force-branch", merge, merge),
				Cause:       err,
			}
		}
		return "", &Error{
			Code:    ExitGitFailed,
			Kind:    "branch_delete_failed",
			Message: redactCreds(strings.TrimSpace(string(out))),
			Cause:   err,
		}
	}
	if note != "" {
		return fmt.Sprintf("branch %q %sdeleted with -D", branch, note), nil
	}
	return fmt.Sprintf("branch %q deleted", branch), nil
}

// worktreePaths lists every worktree path git knows for root (main checkout included).
func worktreePaths(ctx context.Context, root string) []string {
	out, _ := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, strings.TrimSpace(p))
		}
	}
	return paths
}

// cleanTaskState removes the per-branch task-state file (through taskstate, its one
// owner, which also sweeps stranded atomic-write temps) and the legacy session ref.
func cleanTaskState(projectRoot, branch string) {
	slug, ok := taskstate.Slug(branch)
	if !ok {
		return
	}
	letsDir := filepath.Join(projectRoot, ".lets")
	_ = taskstate.Remove(letsDir, slug, time.Now().Add(3*time.Second))
	_ = os.Remove(filepath.Join(letsDir, "sessions", ".session-start-ref-"+slug))
}

// removeAlreadyGone finishes the branch step for a worktree whose directory and git
// registration are both gone (removed by hand, by another tool, or by an earlier run
// that died after `git worktree remove`). Idempotent: with no candidate branch left
// it is still worktree_not_found.
func removeAlreadyGone(ctx context.Context, projectRoot string, opts RemoveOptions, res *RemoveResult,
	addStep func(status, msg string), fail func(*Error) (*RemoveResult, error)) (*RemoveResult, error) {
	var branch string
	for _, c := range []string{opts.Branch, conventionCandidate(ctx, projectRoot, opts.Name), "worktree-" + opts.Name, opts.Name} {
		if c != "" && refExists(ctx, projectRoot, "refs/heads/"+c) {
			branch = c
			break
		}
	}
	if branch == "" {
		return fail(&Error{
			Code:        ExitGeneric,
			Kind:        "worktree_not_found",
			Message:     fmt.Sprintf("worktree %q not found", opts.Name),
			Remediation: "run `lets worktree list` to see available worktrees",
		})
	}
	addStep(StepOK, fmt.Sprintf("worktree %q already gone; finishing the branch step for %q", opts.Name, branch))
	cleanTaskState(projectRoot, branch)
	branchDeleted := false
	if opts.DeleteBranch {
		msg, e := deleteBranch(ctx, projectRoot, branch, opts.ForceBranch, mergeBranch(projectRoot))
		if e != nil {
			return fail(e)
		}
		branchDeleted = true
		addStep(StepOK, msg)
	}
	res.OK = true
	res.Removed = &RemovedInfo{Name: opts.Name, Branch: branch, BranchDeleted: branchDeleted, AlreadyGone: true}
	return res, nil
}
