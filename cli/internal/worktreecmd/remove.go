//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hasUserChanges returns true if `git status --porcelain` output contains
// any line that isn't a known LETS-managed symlink. The `.lets` and
// `.beads/` paths are populated by Create after `git worktree add`, so
// they are untracked relative to the worktree's branch HEAD and would
// otherwise mask the "is this worktree dirty?" question.
func hasUserChanges(porcelain string) bool {
	for _, l := range strings.Split(porcelain, "\n") {
		if l == "" {
			continue
		}
		// Porcelain lines are "XY <path>" (2 status chars + space + path).
		if len(l) < 4 {
			return true
		}
		path := strings.TrimSpace(l[2:])
		if path == ".lets" || path == ".lets/" || path == ".beads" || path == ".beads/" || strings.HasPrefix(path, ".beads/") {
			continue
		}
		return true
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
		deleteFlag := "-d"
		if opts.ForceBranch {
			deleteFlag = "-D"
		}
		// `--` between flags and the branch name prevents git from treating a
		// branch name that starts with "-" as another flag.
		if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "branch", deleteFlag, "--", opts.Branch).CombinedOutput(); err != nil {
			kind := "branch_delete_failed"
			code := ExitGitFailed
			if !opts.ForceBranch && strings.Contains(string(out), "not fully merged") {
				kind = "branch_unmerged"
				code = ExitBranchUnmerged
			}
			return fail(&Error{
				Code:        code,
				Kind:        kind,
				Message:     redactCreds(strings.TrimSpace(string(out))),
				Remediation: "pass --force-branch to delete an unmerged branch",
				Cause:       err,
			})
		}
		addStep(StepOK, fmt.Sprintf("branch %q deleted", opts.Branch))
		res.OK = true
		res.Removed = &RemovedInfo{Name: opts.Name, Branch: opts.Branch, BranchDeleted: true}
		return res, nil
	}

	// Full worktree+branch removal path.
	wtPath := filepath.Join(projectRoot, ".worktrees", opts.Name)
	if _, err := os.Lstat(wtPath); err != nil {
		listOut, _ := exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "list").Output()
		if !strings.Contains(string(listOut), opts.Name) {
			return fail(&Error{
				Code:        ExitGeneric,
				Kind:        "worktree_not_found",
				Message:     fmt.Sprintf("worktree %q not found", opts.Name),
				Remediation: "run `lets worktree list` to see available worktrees",
			})
		}
	}
	addStep(StepOK, fmt.Sprintf("found worktree at %s", wtPath))

	// Re-derive actual branch (attach case: branch != worktree-<name>).
	branchOut, _ := exec.CommandContext(ctx, "git", "-C", wtPath, "branch", "--show-current").Output()
	branch := strings.TrimSpace(string(branchOut))

	// Safety check unless --force. The LETS-managed `.lets` and `.beads/`
	// symlinks always appear as untracked in the worktree (their branch HEAD
	// commit doesn't carry these paths). Filter them out so a freshly-created
	// worktree is not falsely classified as "dirty".
	statusOut, _ := exec.CommandContext(ctx, "git", "-C", wtPath, "status", "--porcelain").Output()
	dirty := hasUserChanges(string(statusOut))
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

	// Combined-mode: --delete-branch in same call also deletes the branch.
	branchDeleted := false
	if opts.DeleteBranch && branch != "" {
		deleteFlag := "-d"
		if opts.ForceBranch {
			deleteFlag = "-D"
		}
		// `--` separator: see branch-only path above.
		if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "branch", deleteFlag, "--", branch).CombinedOutput(); err != nil {
			if !opts.ForceBranch && strings.Contains(string(out), "not fully merged") {
				return fail(&Error{
					Code:        ExitBranchUnmerged,
					Kind:        "branch_unmerged",
					Message:     fmt.Sprintf("branch %q has unmerged commits", branch),
					Remediation: "pass --force-branch to delete anyway, or merge it first",
					Cause:       err,
				})
			}
			return fail(&Error{
				Code:    ExitGitFailed,
				Kind:    "branch_delete_failed",
				Message: redactCreds(strings.TrimSpace(string(out))),
				Cause:   err,
			})
		}
		branchDeleted = true
		addStep(StepOK, fmt.Sprintf("branch %q deleted", branch))
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
