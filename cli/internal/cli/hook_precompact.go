package cli

import (
	"github.com/spf13/cobra"
)

// NewHookPreCompactCmd builds `lets hook precompact --rules=<path>`.
//
// PreCompact fires just before Claude Code compacts conversation history.
// We re-emit the LETS Config block + drift check so the compaction summary
// retains them - prevents workflow drift in long sessions.
//
// Currently shares its implementation with `lets hook session-start` via
// runHookSessionPipeline (same output, same effect). Kept as a distinct
// subcommand so future PreCompact-specific behavior (e.g. context
// snapshotting) lives here without touching the SessionStart codepath.
func NewHookPreCompactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "precompact",
		Short: "Re-emit LETS Config + drift check (PreCompact hook target)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rulesPath, _ := cmd.Flags().GetString("rules")
			return runHookSessionPipeline(cmd, rulesPath)
		},
	}
	cmd.Flags().String("rules", "", "Path to plugin's rules/lets-rules.md (for drift check)")
	// MarkFlagRequired returns an error only if the flag name is wrong (typo);
	// the flag IS defined immediately above, so any error is a programmer bug
	// and would surface during dev. Intentional swallow.
	_ = cmd.MarkFlagRequired("rules")
	return cmd
}
