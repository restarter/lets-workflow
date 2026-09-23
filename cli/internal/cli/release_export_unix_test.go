//go:build unix

package cli

import (
	"context"

	"github.com/restarter/lets-workflow/cli/internal/notifycmd"
)

// SetReleaseNotify swaps the release notification sink for a test and returns the restorer.
func SetReleaseNotify(f func(context.Context, notifycmd.Options) (*notifycmd.Result, error)) (restore func()) {
	old := releaseNotify
	releaseNotify = f
	return func() { releaseNotify = old }
}
