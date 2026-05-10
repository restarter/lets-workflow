package cli

import (
	"github.com/spf13/cobra"
)

// NewHookSessionStartCmd builds `lets hook session-start --rules=<path>`.
// Output is the LETS Config block + optional drift notice (rules emission was
// removed in Phase 4b - rules now live in <project>/.claude/rules/lets-rules.md).
//
// Invoked by Claude Code via plugins/lets-workflow/hooks/hooks.json on SessionStart.
// Body shared with `lets hook precompact` via runHookSessionPipeline.
func NewHookSessionStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Emit LETS Config + drift check (SessionStart hook target)",
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
