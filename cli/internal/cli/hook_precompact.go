package cli

import (
	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

// NewHookPreCompactCmd builds `lets hook precompact --rules=<path>`.
//
// PreCompact fires just before Claude Code compacts conversation history.
// We re-inject the workflow rules + LETS Config so the compaction summary
// retains them - prevents workflow drift in long sessions.
//
// Currently shares its implementation with `lets hook session-start`
// (same output, same effect). Kept as a distinct subcommand so future
// PreCompact-specific behavior (e.g. context snapshotting) lives here
// without touching the SessionStart codepath.
func NewHookPreCompactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "precompact",
		Short: "Re-inject workflow rules and LETS Config (PreCompact hook target)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rulesPath, _ := cmd.Flags().GetString("rules")
			return sessionstart.Run(cmd.OutOrStdout(), rulesPath, sessionstart.DetectProjectRoot())
		},
	}
	cmd.Flags().String("rules", "", "Path to rules-context.md (required, supplied by plugin)")
	_ = cmd.MarkFlagRequired("rules")
	return cmd
}
