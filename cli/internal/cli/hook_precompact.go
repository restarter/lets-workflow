package cli

import (
	"github.com/spf13/cobra"
)

// NewHookPreCompactCmd builds `lets hook precompact` - DEPRECATED, removed in
// the next release (lets-v59n8). The plugin no longer wires a PreCompact hook:
// on an auto-compact Claude Code sends its stdout only to the debug log, and on
// a manual /compact it echoes it into the /compact output - a second copy of
// the LETS Config that SessionStart(compact) re-injects anyway. Kept for one
// release as a silent no-op, so an older plugin's hooks.json run against this
// binary neither prints that copy nor fails a compaction: --rules is accepted
// and ignored, stdin is not read, nothing is printed, exit 0.
func NewHookPreCompactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "precompact",
		Short:  "Deprecated: prints nothing (the plugin no longer wires PreCompact)",
		Hidden: true,
		RunE:   func(*cobra.Command, []string) error { return nil },
	}
	cmd.Flags().String("rules", "", "Ignored - kept so an older plugin's hooks.json still runs")
	return cmd
}
