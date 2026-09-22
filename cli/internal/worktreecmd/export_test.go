//go:build unix

package worktreecmd

import (
	"context"

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
