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
// to an existing branch or create a new one off of <base>, and which ref name
// to use. Auto mode chooses based on whether the resolved ref already exists.
//
// branch decouples the worktree directory NAME from the branch REF (lets-x5ucf):
//   - branch == "" → historical behavior: attach to refs/heads/<name>, or create
//     worktree-<name>. The dir-name validator forbids '/', so this path can never
//     reach a slash-bearing ref.
//   - branch != "" → the explicit ref OVERRIDES the name-derived branch entirely:
//     attach to (or create) refs/heads/<branch> verbatim — no "worktree-" prefix —
//     while the worktree dir keeps the sanitized <name>. This is the only path to a
//     git-flow ref like feature/x, since <branch> is validated by the ref grammar
//     (slashes allowed) instead of the stricter dir-name allowlist.
func ResolveBranch(ctx context.Context, projectRoot, name, branch string, mode BranchMode, base string) (BranchPlan, error) {
	// ref is what we look up / attach to; newBranch is what we create when not
	// attaching. With an explicit --branch both collapse to <branch>; otherwise
	// ref is <name> and newBranch is the historical worktree-<name> form.
	ref := name
	newBranch := "worktree-" + name
	if branch != "" {
		if err := validateBranchRef(ctx, branch); err != nil {
			return BranchPlan{}, err
		}
		ref = branch
		newBranch = branch
	}

	exists, err := branchExists(ctx, projectRoot, ref)
	if err != nil {
		return BranchPlan{}, &Error{
			Code:    ExitGitFailed,
			Kind:    "git_show_ref_failed",
			Message: fmt.Sprintf("could not check if branch %q exists", ref),
			Cause:   err,
		}
	}

	switch mode {
	case BranchAttach:
		if !exists {
			return BranchPlan{}, &Error{
				Code:        ExitBranchConflict,
				Kind:        "branch_missing",
				Message:     fmt.Sprintf("--attach requested but branch %q does not exist", ref),
				Remediation: "drop --attach to create a new " + newBranch + " branch, or create the branch first",
			}
		}
		return BranchPlan{Mode: "attached", Branch: ref}, nil

	case BranchNewBranch:
		if exists {
			return BranchPlan{}, &Error{
				Code:        ExitBranchConflict,
				Kind:        "branch_exists",
				Message:     fmt.Sprintf("--new-branch requested but branch %q already exists", ref),
				Remediation: "drop --new-branch to attach, or pick a different name",
			}
		}
		if err := validateBaseRef(ctx, projectRoot, base); err != nil {
			return BranchPlan{}, err
		}
		return BranchPlan{Mode: "created", Branch: newBranch, Base: base}, nil

	case BranchAuto:
		if exists {
			return BranchPlan{Mode: "attached", Branch: ref}, nil
		}
		if err := validateBaseRef(ctx, projectRoot, base); err != nil {
			return BranchPlan{}, err
		}
		return BranchPlan{Mode: "created", Branch: newBranch, Base: base}, nil
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

// validateBranchRef validates an explicit --branch ref. Unlike the worktree
// dir NAME (ValidateName), a branch ref MAY contain '/' so git-flow names like
// feature/x are accepted (lets-x5ucf). We reject a leading '-' ourselves (so
// the ref can never be mistaken for a flag when handed to `git worktree add`),
// then defer to `git check-ref-format` for the full ref grammar — it rejects
// '..', control chars, space, ~^:?*[\, a trailing '/' or '.lock', and '@{'.
func validateBranchRef(ctx context.Context, branch string) error {
	if strings.TrimSpace(branch) == "" {
		return &Error{Code: ExitUsage, Kind: "empty_branch", Message: "--branch value is empty"}
	}
	if strings.HasPrefix(branch, "-") {
		return &Error{
			Code:        ExitUsage,
			Kind:        "invalid_branch",
			Message:     fmt.Sprintf("branch ref %q cannot start with '-'", branch),
			Remediation: "pick a branch name that does not start with a dash",
		}
	}
	if err := exec.CommandContext(ctx, "git", "check-ref-format", "--branch", branch).Run(); err != nil {
		return &Error{
			Code:        ExitUsage,
			Kind:        "invalid_branch",
			Message:     fmt.Sprintf("git rejects branch ref %q as invalid", branch),
			Remediation: "use a valid git branch name (slashes allowed, e.g. feature/x; no spaces, '..', ~^:?*[ or trailing '/')",
			Cause:       err,
		}
	}
	return nil
}
