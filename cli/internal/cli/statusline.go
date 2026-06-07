package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/statusline"
)

// interactiveTTY reports whether r is an interactive terminal (a character
// device) rather than a pipe or file. statusline.Render blocks reading the
// Claude Code JSON context from stdin; on a TTY nothing arrives and nothing
// closes the stream, so a direct `lets statusline` run would hang forever.
// Type-asserting to *os.File means test readers (strings.Reader) never trip
// the guard, so the render tests are unaffected.
func interactiveTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

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
		fetchOnly     bool
		fetchTaskOnly bool
		taskID        string
		cacheDir      string
		light         bool
		rich          bool // deprecated no-op: rich is the default now
		compact       bool
		noTip         bool
		noDir         bool
		noTask        bool
	)

	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "Render the LETS rich statusline (or background-refresh a cache)",
		Long: `Reads JSON context from Claude Code via stdin and writes the rich
multi-line statusline to stdout. Wire it into .claude/settings.json:
"statusLine": {"type":"command","command":"lets statusline"}.

  --light     light-terminal palette (default dark)
  --no-tip    hide the bottom tip line (also: env LETS_STATUSLINE_TIP=off)
  --no-dir    hide the location pill (also: env LETS_STATUSLINE_DIR=off)
  --no-task   hide the task line (also: env LETS_STATUSLINE_TASK=off)
  --compact   render the legacy 2-line statusline instead of the rich box

With --fetch-usage-only / --fetch-task-only: runs as a background process
that refreshes <cache-dir>/usage or <cache-dir>/task-status and exits. No
stdin or stdout interaction in those modes.`,
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
			if fetchTaskOnly {
				return statusline.RunFetchTaskOnly(cacheDir, taskID)
			}
			// Interactive guard (after the fetch-only branches, which don't touch
			// stdin): a direct `lets statusline` in a terminal would block on the
			// stdin read forever. Print a one-shot wiring hint to stderr and exit 0
			// so Claude Code never treats this as a crashed bar.
			if interactiveTTY(cmd.InOrStdin()) {
				fmt.Fprintln(cmd.ErrOrStderr(), "lets statusline expects Claude Code's JSON context on stdin; run by hand it just waits for input.")
				fmt.Fprintln(cmd.ErrOrStderr(), "  Preview:  echo '{}' | lets statusline [--light] [--compact] [--no-tip]")
				fmt.Fprintln(cmd.ErrOrStderr(), "  Install:  lets init   (wires \"lets statusline\" into .claude/settings.json)")
				return nil
			}
			_ = rich // accepted for back-compat; rich is the default
			return statusline.Render(cmd.InOrStdin(), cmd.OutOrStdout(), light, compact, !noTip, !noDir, !noTask)
		},
	}
	cmd.Flags().BoolVar(&fetchOnly, "fetch-usage-only", false,
		"Internal: refresh usage cache and exit (used by background subprocess)")
	cmd.Flags().BoolVar(&fetchTaskOnly, "fetch-task-only", false,
		"Internal: refresh task-status cache and exit (used by background subprocess)")
	cmd.Flags().StringVar(&taskID, "task-id", "",
		"Task id to query (required when --fetch-task-only is set)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "",
		"Cache directory (required when --fetch-usage-only / --fetch-task-only is set)")
	cmd.Flags().BoolVar(&light, "light", false,
		"Use the light-terminal palette (default dark)")
	cmd.Flags().BoolVar(&noTip, "no-tip", false,
		"Hide the bottom tip line (also: env LETS_STATUSLINE_TIP=off)")
	cmd.Flags().BoolVar(&noDir, "no-dir", false,
		"Hide the location pill (also: env LETS_STATUSLINE_DIR=off)")
	cmd.Flags().BoolVar(&noTask, "no-task", false,
		"Hide the task line (also: env LETS_STATUSLINE_TASK=off)")
	cmd.Flags().BoolVar(&compact, "compact", false,
		"Render the legacy 2-line statusline instead of the rich box")
	cmd.Flags().BoolVar(&rich, "rich", false,
		"Accepted no-op: the rich statusline is the default")
	_ = cmd.Flags().MarkHidden("rich") // accepted for back-compat, hidden from help, no warning
	return cmd
}
