//go:build !unix

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errCmuxUnsupported is the canonical "not on this platform" error for
// `lets cmux`. cmux is a macOS terminal; the launcher only makes sense there.
// The subcommand still exists in --help so `lets --help` stays informative.
var errCmuxUnsupported = errors.New("lets cmux is macOS-only (cmux terminal); use the manual new-terminal flow on this platform")

// NewCmuxCmd returns the cmux subcommand stub on non-unix platforms.
func NewCmuxCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "cmux",
		Short:         "Launch worktrees in cmux (macOS only)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(&cobra.Command{
		Use:           "open <path>",
		Short:         "Open in cmux (not supported on this platform)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(_ *cobra.Command, _ []string) error { return errCmuxUnsupported },
	})
	return root
}
