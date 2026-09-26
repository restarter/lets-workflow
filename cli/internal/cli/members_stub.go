//go:build !unix

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errMembersUnsupported: the members registry judges liveness from Claude Code's
// session registry and flock, both unix-only here.
var errMembersUnsupported = errors.New("lets members is not supported on this platform (unix only)")

// NewMembersCmd returns the members stub on non-unix platforms: every subcommand
// fails with the same error, so a caller never mistakes it for an empty registry.
func NewMembersCmd() *cobra.Command {
	root := &cobra.Command{Use: "members", Short: "Members registry (not supported on this platform)", SilenceUsage: true, SilenceErrors: true}
	for _, use := range []string{"add", "dismiss", "status", "lead"} {
		c := &cobra.Command{
			Use: use, Short: "members " + use + " (not supported on this platform)", SilenceUsage: true, SilenceErrors: true,
			RunE: func(*cobra.Command, []string) error { return errMembersUnsupported },
		}
		for _, name := range []string{"scope", "name", "role", "model", "isolation", "worktree-path", "worktree-branch", "link", "cwd", "agent-id"} {
			c.Flags().String(name, "", "")
		}
		for _, name := range []string{"json", "all", "claim"} {
			c.Flags().Bool(name, false, "")
		}
		root.AddCommand(c)
	}
	return root
}
