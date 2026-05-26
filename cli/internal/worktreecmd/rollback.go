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

// rollback transitions Create from "git worktree added" to "fully undone".
// Best-effort; reports residual paths in CreateResult.Rollback (no log file).
//
// If prevMainBranch is non-empty, main repo had auto-switched via
// --switch-main-if-needed; this function restores it as the FIRST step
// so user's working state is restored even if other rollback steps fail.
//
// Safety: wtPath MUST be a descendant of <projectRoot>/.worktrees/ — defense-
// in-depth against future callers that might pass an attacker-controlled path.
func rollback(ctx context.Context, result *CreateResult, projectRoot, wtPath string, plan BranchPlan, prevMainBranch, reason string, cause error) (*CreateResult, error) {
	rb := &RollbackInfo{Attempted: true, Succeeded: true}
	if !pathDescendantOfWorktrees(projectRoot, wtPath) {
		rb.Succeeded = false
		rb.Residual = []string{wtPath}
		result.Rollback = rb
		result.OK = false
		result.Error = &ErrorInfo{
			Kind:    "rollback_refused_path_escape",
			Message: fmt.Sprintf("path %q outside %s/.worktrees/; refusing destructive rollback", wtPath, projectRoot),
		}
		return result, cause
	}

	var residual []string

	// 0) Restore main repo's branch if auto-switch happened (do this FIRST so
	//    user's working state is restored even if other steps fail).
	if !restoreMainBranch(ctx, projectRoot, prevMainBranch, &result.Envelope, "rollback: ") {
		rb.Succeeded = false
		residual = append(residual, fmt.Sprintf("main_repo_on_branch:%s (expected %s)", currentBranchOr(ctx, projectRoot), prevMainBranch))
	}

	// 1) git worktree remove --force.
	wtRemoveOK := false
	if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "remove", "--force", wtPath).CombinedOutput(); err != nil {
		rb.Succeeded = false
		result.Steps = append(result.Steps, Step{
			Status:  StepWarn,
			Message: fmt.Sprintf("rollback: git worktree remove failed: %s", redactCreds(strings.TrimSpace(string(out)))),
		})
	} else {
		wtRemoveOK = true
	}

	// 2) If dir still on disk, force-remove.
	dirRemoveOK := false
	if _, err := os.Stat(wtPath); err == nil {
		if err := os.RemoveAll(wtPath); err != nil {
			rb.Succeeded = false
			residual = append(residual, wtPath)
		} else {
			dirRemoveOK = true
		}
	} else {
		dirRemoveOK = true
	}

	// 3) Delete created branch ONLY if both prior steps succeeded.
	if plan.Mode == "created" && wtRemoveOK && dirRemoveOK {
		if err := exec.CommandContext(ctx, "git", "-C", projectRoot, "branch", "-D", plan.Branch).Run(); err != nil {
			rb.Succeeded = false
			residual = append(residual, "branch:"+plan.Branch)
		}
	}

	// 4) Prune worktree metadata.
	_ = exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "prune").Run()

	rb.Residual = residual
	result.Rollback = rb
	result.OK = false
	var cerr *Error
	if errors.As(cause, &cerr) {
		result.Error = &ErrorInfo{Kind: cerr.Kind, Message: cerr.Message, Remediation: cerr.Remediation}
		return result, cerr
	}
	e := &Error{
		Code:    ExitFilesystem,
		Kind:    "post_create_failed",
		Message: reason,
		Cause:   cause,
	}
	result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
	return result, e
}

// restoreMainBranch switches the main repo back to prev. Returns true on
// success (or no-op when prev == ""), false when the switch itself fails.
// Both branches of the result append a step into env.Steps using the given
// label prefix so log readers can distinguish inline failures ("restore: ")
// from rollback-driven ones ("rollback: ").
//
// Used by:
//   - rollback() before destructive cleanup (FIRST step).
//   - create.go Step 6 + Step 7 inline failure paths (gitignore or git
//     worktree add failure after --switch-main-if-needed already moved main).
//
// A false return tells the caller to surface a residual in the JSON envelope
// so partial state is visible alongside the primary error, not buried in
// Steps[]. Pre-S-3 the inline paths called a closure that only logged a warn
// step — review found that masked the partial state.
func restoreMainBranch(ctx context.Context, projectRoot, prev string, env *Envelope, stepPrefix string) bool {
	if prev == "" {
		return true
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "switch", prev).CombinedOutput(); err != nil {
		env.Steps = append(env.Steps, Step{
			Status:  StepWarn,
			Message: fmt.Sprintf("%sfailed to restore main to %s: %s", stepPrefix, prev, redactCreds(strings.TrimSpace(string(out)))),
		})
		return false
	}
	env.Steps = append(env.Steps, Step{
		Status:  StepOK,
		Message: fmt.Sprintf("%srestored main to %s", stepPrefix, prev),
	})
	return true
}

// currentBranchOr returns the current branch of dir, or "<unknown>" when
// the call fails. Used to enrich residual messages with where main actually
// is right now, not just where the caller wanted it back.
func currentBranchOr(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return "<unknown>"
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		return s
	}
	return "<detached>"
}

// pathDescendantOfWorktrees verifies wtPath sits beneath projectRoot/.worktrees/.
// Both paths are symlink-resolved first to defeat /var ↔ /private/var (macOS) and
// any user-installed symlink at the repo root — without this, a symlinked
// project root flips the relative path into something starting with ".." and
// remove() refuses on a perfectly legitimate worktree.
func pathDescendantOfWorktrees(projectRoot, wtPath string) bool {
	resolve := func(p string) string {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return real
		}
		return p
	}
	rel, err := filepath.Rel(resolve(projectRoot), resolve(wtPath))
	if err != nil {
		return false
	}
	return strings.HasPrefix(rel, ".worktrees"+string(filepath.Separator)) && !strings.Contains(rel, "..")
}
