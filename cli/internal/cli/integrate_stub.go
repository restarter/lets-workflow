//go:build !unix

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errIntegrateUnsupported: integrate takes a flock and works through git
// worktrees in the common dir, unix-only here.
var errIntegrateUnsupported = errors.New("lets integrate is not supported on this platform (unix only)")

// NewIntegrateCmd returns the integrate stub on non-unix platforms: it fails, so a
// caller never mistakes it for an integrated chunk.
func NewIntegrateCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "integrate", Short: "Integrate an isolated implementer's commits (not supported on this platform)", SilenceUsage: true, SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error { return errIntegrateUnsupported },
	}
	for _, name := range []string{"from", "since", "run", "chunk"} {
		c.Flags().String(name, "", "")
	}
	c.Flags().Bool("json", false, "")
	return c
}
