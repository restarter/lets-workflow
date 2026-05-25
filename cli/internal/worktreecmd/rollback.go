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
	if prevMainBranch != "" {
		if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "switch", prevMainBranch).CombinedOutput(); err != nil {
			rb.Succeeded = false
			result.Steps = append(result.Steps, Step{
				Status:  StepWarn,
				Message: fmt.Sprintf("rollback: failed to restore main to %s: %s", prevMainBranch, redactCreds(strings.TrimSpace(string(out)))),
			})
		} else {
			result.Steps = append(result.Steps, Step{
				Status:  StepOK,
				Message: fmt.Sprintf("rollback: main restored to %s", prevMainBranch),
			})
		}
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

func pathDescendantOfWorktrees(projectRoot, wtPath string) bool {
	rel, err := filepath.Rel(projectRoot, wtPath)
	if err != nil {
		return false
	}
	return strings.HasPrefix(rel, ".worktrees"+string(filepath.Separator)) && !strings.Contains(rel, "..")
}
