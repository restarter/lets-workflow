//go:build unix

package worktreecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateRelativeSymlink at linkPath pointing to targetAbs (resolved via filepath.Rel).
// Validates: target exists; relative-result is a descendant of projectRoot
// (no .. escape). Defense-in-depth against future caller bugs.
func CreateRelativeSymlink(linkPath, targetAbs, projectRoot string) error {
	if _, err := os.Lstat(targetAbs); err != nil {
		if os.IsNotExist(err) {
			return &Error{
				Code:        ExitSymlinkSourceMissing,
				Kind:        "symlink_source_missing",
				Message:     fmt.Sprintf("symlink target %q does not exist", targetAbs),
				Remediation: "ensure the main repo has this path before creating a worktree",
				Cause:       err,
			}
		}
		return &Error{Code: ExitFilesystem, Kind: "lstat_failed", Message: targetAbs, Cause: err}
	}
	// Verify target stays within projectRoot.
	rel, err := filepath.Rel(projectRoot, targetAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return &Error{
			Code:    ExitFilesystem,
			Kind:    "symlink_target_escapes_repo",
			Message: fmt.Sprintf("symlink target %q is outside project root %q", targetAbs, projectRoot),
		}
	}
	relFromLink, err := filepath.Rel(filepath.Dir(linkPath), targetAbs)
	if err != nil {
		return &Error{Code: ExitFilesystem, Kind: "rel_path_failed", Cause: err}
	}
	if err := os.Symlink(relFromLink, linkPath); err != nil {
		return &Error{
			Code:    ExitFilesystem,
			Kind:    "symlink_failed",
			Message: fmt.Sprintf("symlink %q -> %q", linkPath, relFromLink),
			Cause:   err,
		}
	}
	return nil
}
