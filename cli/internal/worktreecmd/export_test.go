//go:build unix

package worktreecmd

import (
	"context"
	"os"

	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// Test-only exports. Allows _test.go in package worktreecmd_test to drive
// internal helpers without making them part of the public API.

var PerformRollbackForTesting = rollback

var RedactCredsForTesting = redactCreds

var MergedUpstreamForTesting = mergedUpstream

// LinkSharedForTesting drives linkShared; mode 0 = create, 1 = adopt.
func LinkSharedForTesting(mainRoot, wtRoot string, links []trackeradapter.Link, adopt bool) ([]Step, []StoreLink, string, error) {
	mode := modeCreate
	if adopt {
		mode = modeAdopt
	}
	out, err := linkShared(context.Background(), mainRoot, wtRoot, links, mode)
	return out.Steps, out.StoreLinks, out.MovedAside, err
}

// SetForEachRef swaps the branch lister for a test and returns the restorer.
func SetForEachRef(f func(context.Context, string) ([]byte, error)) (restore func()) {
	old := forEachRef
	forEachRef = f
	return func() { forEachRef = old }
}

// SetBeforeTaskStateRemove runs f between release's marker and its removal; returns the restorer.
func SetBeforeTaskStateRemove(f func()) (restore func()) {
	old := beforeTaskStateRemove
	beforeTaskStateRemove = f
	return func() { beforeTaskStateRemove = old }
}

// CopyRegularForTesting drives copyRegular.
func CopyRegularForTesting(root *os.Root, src, rel string, perm os.FileMode) error {
	return copyRegular(root, src, rel, perm)
}

// LiteralExcludeForTesting drives literalExclude.
var LiteralExcludeForTesting = literalExclude

// SetWriteExcludesForTesting swaps the .worktreeinclude exclude writer; restore undoes it.
func SetWriteExcludesForTesting(f func(context.Context, string, []string) error) (restore func()) {
	old := writeExcludes
	writeExcludes = f
	return func() { writeExcludes = old }
}

// SetGitWorktreeAddForTesting swaps create's `git worktree add` runner; restore undoes it.
func SetGitWorktreeAddForTesting(f func(context.Context, []string) ([]byte, error)) (restore func()) {
	old := gitWorktreeAdd
	gitWorktreeAdd = f
	return func() { gitWorktreeAdd = old }
}

// SetGitWorktreeRemoveForTesting swaps remove's `git worktree remove --force` runner; restore undoes it.
func SetGitWorktreeRemoveForTesting(f func(ctx context.Context, root, path string) ([]byte, error)) (restore func()) {
	old := gitWorktreeRemove
	gitWorktreeRemove = f
	return func() { gitWorktreeRemove = old }
}

// SetSettingsStage replaces the EnsureLetsAdditionalDir test seam; restore undoes it.
func SetSettingsStage(f func(string)) (restore func()) {
	old := settingsStage
	settingsStage = f
	return func() { settingsStage = old }
}
