package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/statusline"
)

// NewStatuslineCmd builds `lets statusline` and `lets statusline --fetch-usage-only`.
//
// Default mode: reads JSON from stdin, writes the 2-line formatted statusline to
// stdout. Used by per-project .lets/statusline.sh wrapper.
//
// --fetch-usage-only mode: fetches the Anthropic usage API and writes the
// result to <cache-dir>/usage. Used by the default mode to refresh stale cache
// via background subprocess. No stdin/stdout interaction.
func NewStatuslineCmd() *cobra.Command {
	var (
		fetchOnly bool
		cacheDir  string
	)

	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "Render LETS-branded statusline (or background-refresh usage cache)",
		Long: `Reads JSON context from Claude Code via stdin and writes a two-line
formatted statusline to stdout. Used by per-project .lets/statusline.sh wrapper.

With --fetch-usage-only: runs as a background process that fetches the
Anthropic usage API and writes the result to <cache-dir>/usage. No
stdin or stdout interaction in this mode.`,
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			// Statusline runs frequently and stderr is invisible to users in
			// the bottom bar. A panic in render/fetch would otherwise leave
			// a blank statusline with no diagnostic. Recover, write a minimal
			// fallback line so the user sees SOMETHING, log full panic to
			// stderr (visible via `lets statusline 2>&1`), and exit 0 so
			// Claude Code uses our fallback instead of dropping the bar.
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintln(cmd.OutOrStdout(), "🌱 LETS Workflow [statusline error - run `lets statusline 2>&1` for details]")
					fmt.Fprintf(cmd.ErrOrStderr(), "statusline panic: %v\n", r)
					err = nil
				}
			}()
			if fetchOnly {
				return statusline.RunFetchOnly(cacheDir)
			}
			return statusline.Render(cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&fetchOnly, "fetch-usage-only", false,
		"Internal: refresh usage cache and exit (used by background subprocess)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "",
		"Cache directory (required when --fetch-usage-only is set)")
	return cmd
}
