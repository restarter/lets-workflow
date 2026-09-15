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
