package cli

import (
	"github.com/spf13/cobra"
)

// NewHookCmd builds the parent `hook` command. Subcommands invoke logic
// for individual Claude Code hook events (SessionStart, PreCompact, etc.).
//
// Direct invocation of `lets hook` (no subcommand) shows help.
func NewHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Hook subcommands invoked by Claude Code (SessionStart, PreCompact, ...)",
	}
	cmd.AddCommand(NewHookSessionStartCmd())
	return cmd
}
