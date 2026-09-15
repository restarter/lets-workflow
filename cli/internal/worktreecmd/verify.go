//go:build unix

package worktreecmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VerifyCreate confirms post-add invariants: worktree's branch matches the
// plan, and any LETS-managed symlinks resolve (no dangling pointer to a
// missing main-side target).
func VerifyCreate(ctx context.Context, projectRoot, wtPath string, plan BranchPlan, storeLinks []StoreLink) error {
	_ = projectRoot
	out, err := exec.CommandContext(ctx, "git", "-C", wtPath, "branch", "--show-current").Output()
	if err != nil {
		return &Error{Code: ExitVerifyFailed, Kind: "verify_branch_query", Cause: err}
	}
	got := strings.TrimSpace(string(out))
	if got != plan.Branch {
		return &Error{
			Code:    ExitVerifyFailed,
			Kind:    "verify_branch_mismatch",
			Message: fmt.Sprintf("worktree branch %q != expected %q", got, plan.Branch),
		}
	}
	wtLets := filepath.Join(wtPath, ".lets")
	if fi, err := os.Lstat(wtLets); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if _, err := os.Stat(wtLets); err != nil {
			return &Error{Code: ExitVerifyFailed, Kind: "lets_symlink_broken", Cause: err}
		}
	}
	for _, sl := range storeLinks {
		p := filepath.Join(wtPath, filepath.FromSlash(sl.Path))
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(p); err != nil {
				return &Error{Code: ExitVerifyFailed, Kind: "store_link_broken", Message: sl.Path, Cause: err}
			}
		}
	}
	return nil
}
