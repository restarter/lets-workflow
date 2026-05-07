package cli

import (
	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

// NewHookSessionStartCmd builds `lets hook session-start --rules=<path>`.
// Output is the rules file contents followed by the LETS Config block.
//
// Invoked by Claude Code via plugins/lets/hooks/hooks.json on SessionStart.
func NewHookSessionStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Inject workflow rules and LETS Config (SessionStart hook target)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rulesPath, _ := cmd.Flags().GetString("rules")
			return sessionstart.Run(cmd.OutOrStdout(), rulesPath, sessionstart.DetectProjectRoot())
		},
	}
	cmd.Flags().String("rules", "", "Path to rules-context.md (required, supplied by plugin)")
	_ = cmd.MarkFlagRequired("rules")
	return cmd
}
