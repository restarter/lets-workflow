//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// BranchPlan is the resolved outcome of ResolveBranch.
type BranchPlan struct {
	Mode   string // "attached" | "created"
	Branch string
	Base   string // empty for attached
}

// BranchMode selects the resolution strategy. Empty = auto-detect.
type BranchMode string

const (
	BranchAuto      BranchMode = ""
	BranchAttach    BranchMode = "attach"
	BranchNewBranch BranchMode = "new"
)

// ResolveBranch decides whether `lets worktree create <name>` should attach
// to an existing branch named <name> or create a new branch worktree-<name>
// off of <base>. Auto mode chooses based on whether refs/heads/<name> exists.
func ResolveBranch(ctx context.Context, projectRoot, name string, mode BranchMode, base string) (BranchPlan, error) {
	exists, err := branchExists(ctx, projectRoot, name)
	if err != nil {
		return BranchPlan{}, &Error{
			Code:    ExitGitFailed,
			Kind:    "git_show_ref_failed",
			Message: fmt.Sprintf("could not check if branch %q exists", name),
			Cause:   err,
		}
	}

	switch mode {
	case BranchAttach:
		if !exists {
			return BranchPlan{}, &Error{
				Code:        ExitBranchConflict,
				Kind:        "branch_missing",
				Message:     fmt.Sprintf("--attach requested but branch %q does not exist", name),
				Remediation: "drop --attach to create a new worktree-" + name + " branch, or create the branch first",
			}
		}
		return BranchPlan{Mode: "attached", Branch: name}, nil

	case BranchNewBranch:
		if exists {
			return BranchPlan{}, &Error{
				Code:        ExitBranchConflict,
				Kind:        "branch_exists",
				Message:     fmt.Sprintf("--new-branch requested but branch %q already exists", name),
				Remediation: "drop --new-branch to attach, or pick a different name",
			}
		}
		if err := validateBaseRef(ctx, projectRoot, base); err != nil {
			return BranchPlan{}, err
		}
		return BranchPlan{Mode: "created", Branch: "worktree-" + name, Base: base}, nil

	case BranchAuto:
		if exists {
			return BranchPlan{Mode: "attached", Branch: name}, nil
		}
		if err := validateBaseRef(ctx, projectRoot, base); err != nil {
			return BranchPlan{}, err
		}
		return BranchPlan{Mode: "created", Branch: "worktree-" + name, Base: base}, nil
	}
	return BranchPlan{}, &Error{Code: ExitUsage, Kind: "bad_branch_mode"}
}

func branchExists(ctx context.Context, projectRoot, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", projectRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func validateBaseRef(ctx context.Context, projectRoot, base string) error {
	if base == "" {
		return nil
	}
	if err := exec.CommandContext(ctx, "git", "-C", projectRoot, "rev-parse", "--verify", "--quiet", base).Run(); err != nil {
		return &Error{
			Code:        ExitBranchConflict,
			Kind:        "base_ref_missing",
			Message:     fmt.Sprintf("base ref %q does not resolve in this repo", base),
			Remediation: "pass --base <existing-ref> or omit to use LETS_MERGE_BRANCH default",
		}
	}
	return nil
}

// nameRE: positive allowlist. Must start with alnum, lowercase only (avoids
// APFS case-insensitive surprises), allows ._- internally, max 64 chars.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ValidateName performs structural validation then defers to git check-ref-format
// for the final word on whether the constructed branch name (worktree-<name>)
// is valid as a git ref.
func ValidateName(ctx context.Context, name string) error {
	if name == "" {
		return &Error{Code: ExitUsage, Kind: "empty_name", Message: "worktree name is empty"}
	}
	if !nameRE.MatchString(name) {
		return &Error{
			Code:        ExitUsage,
			Kind:        "invalid_name",
			Message:     fmt.Sprintf("worktree name %q does not match allowed pattern", name),
			Remediation: "use lowercase letters, digits, and . _ - (start with alnum); max 64 chars",
		}
	}
	if strings.Contains(name, "..") {
		return &Error{Code: ExitUsage, Kind: "invalid_name", Message: "worktree name cannot contain '..'"}
	}
	if strings.HasSuffix(name, ".lock") {
		return &Error{Code: ExitUsage, Kind: "invalid_name", Message: "worktree name cannot end with .lock (git reserved)"}
	}
	// Final pass: ask git itself.
	if err := exec.CommandContext(ctx, "git", "check-ref-format", "--branch", "worktree-"+name).Run(); err != nil {
		return &Error{
			Code:    ExitUsage,
			Kind:    "git_invalid_ref",
			Message: fmt.Sprintf("git rejects branch name %q as invalid ref", "worktree-"+name),
			Cause:   err,
		}
	}
	return nil
}
