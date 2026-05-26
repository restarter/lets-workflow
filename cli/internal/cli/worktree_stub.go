//go:build !unix

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errWorktreeUnsupported is the canonical "not on this platform" error for
// `lets worktree`. The Unix implementation lives in worktree.go (gated by
// //go:build unix) because it depends on syscall.Flock and other POSIX
// primitives. Windows support is tracked in lets-rqep4 backlog; until then
// the subcommand exists in --help so `lets --help` stays informative, but
// each subcommand exits with a clear remediation.
var errWorktreeUnsupported = errors.New("lets worktree is not yet supported on Windows (tracked in lets-rqep4 backlog); use the bash skill or run from a Unix host")

// NewWorktreeCmd returns the worktree subcommand stub on non-unix platforms.
// Each subcommand surfaces the same "not supported" error so scripts/agents
// see a consistent failure mode.
func NewWorktreeCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "worktree",
		Short:         "Manage interactive git worktrees (not yet supported on Windows)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	for _, use := range []struct{ name, short string }{
		{"create <name>", "Create a worktree (Windows: not supported)"},
		{"remove <name>", "Remove a worktree (Windows: not supported)"},
		{"list", "List worktrees (Windows: not supported)"},
		{"info", "Show worktree info (Windows: not supported)"},
	} {
		sub := &cobra.Command{
			Use:           use.name,
			Short:         use.short,
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE:          func(_ *cobra.Command, _ []string) error { return errWorktreeUnsupported },
		}
		root.AddCommand(sub)
	}
	return root
}
