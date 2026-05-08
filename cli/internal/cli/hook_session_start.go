package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

// Hook output size limits enforced by Claude Code (lets-q9bx7). The hook stdout
// is capped at 10K - anything beyond is silently truncated to a 2K preview.
// We warn approaching the cap and error past it (both stderr) so any future
// regression of the cap surfaces loudly instead of silently truncating context.
const (
	hookSizeWarn  = 9000
	hookSizeError = 10000
)

// NewHookSessionStartCmd builds `lets hook session-start --rules=<path>`.
// Output is the LETS Config block + optional drift notice (rules emission was
// removed in Phase 4b - rules now live in <project>/.claude/rules/lets-rules.md).
//
// Invoked by Claude Code via plugins/lets/hooks/hooks.json on SessionStart.
func NewHookSessionStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Emit LETS Config + drift check (SessionStart hook target)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rulesPath, _ := cmd.Flags().GetString("rules")
			var buf bytes.Buffer
			if err := sessionstart.Run(&buf, rulesPath, sessionstart.DetectProjectRoot()); err != nil {
				return err
			}
			size := buf.Len()
			if _, err := cmd.OutOrStdout().Write(buf.Bytes()); err != nil {
				return err
			}
			if size >= hookSizeError {
				fmt.Fprintf(os.Stderr, "error: hook output %d bytes exceeds 10K cap - rules will be truncated by Claude Code\n", size)
			} else if size >= hookSizeWarn {
				fmt.Fprintf(os.Stderr, "warning: hook output %d bytes approaching 10K cap\n", size)
			}
			return nil
		},
	}
	cmd.Flags().String("rules", "", "Path to plugin's rules/lets-rules.md (for drift check)")
	_ = cmd.MarkFlagRequired("rules")
	return cmd
}
