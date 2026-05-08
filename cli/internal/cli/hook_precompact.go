package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

// NewHookPreCompactCmd builds `lets hook precompact --rules=<path>`.
//
// PreCompact fires just before Claude Code compacts conversation history.
// We re-emit the LETS Config block + drift check so the compaction summary
// retains them - prevents workflow drift in long sessions.
//
// Currently shares its implementation with `lets hook session-start`
// (same output, same effect). Kept as a distinct subcommand so future
// PreCompact-specific behavior (e.g. context snapshotting) lives here
// without touching the SessionStart codepath.
func NewHookPreCompactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "precompact",
		Short: "Re-emit LETS Config + drift check (PreCompact hook target)",
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
