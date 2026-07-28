//go:build !unix

package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// errTmuxUnsupported is the canonical "not on this platform" error for
// `lets tmux`. tmux has no Windows build; the launcher only makes sense on unix.
// The subcommand still exists in --help so `lets --help` stays informative, and
// so TestShippedLaunchers_MatchSubcommands passes on every GOOS.
var errTmuxUnsupported = errors.New("lets tmux is unix-only (Linux/macOS); use the manual new-terminal flow on this platform")

// NewTmuxCmd returns the tmux subcommand stub on non-unix platforms.
func NewTmuxCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tmux",
		Short:         "Launch worktrees in tmux (Linux/macOS only)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// open/rename are interactive-only: a hard error is the right non-unix
	// signal (no JSON caller depends on them degrading).
	for _, use := range []struct{ name, short string }{
		{"open <path>", "Open in tmux (not supported on this platform)"},
		{"rename", "Rename a tmux window (not supported on this platform)"},
	} {
		root.AddCommand(&cobra.Command{
			Use:           use.name,
			Short:         use.short,
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE:          func(_ *cobra.Command, _ []string) error { return errTmuxUnsupported },
		})
	}
	// notify is fired from gate snippets (often with --json) - so unlike
	// open/rename it must NOT hard-fail here. It emits a graceful envelope
	// (ok=true, notified=false, reason=not_supported) and exits 0, so a --json
	// caller on a non-unix host gets a parseable result instead of a bare
	// non-zero exit.
	root.AddCommand(newTmuxNotifyStub())
	return root
}

func newTmuxNotifyStub() *cobra.Command {
	var (
		title, subtitle, body, ref, cwd string
		jsonOut, quiet                  bool
	)
	cmd := &cobra.Command{
		Use:           "notify --title <text> [--body <text>] [--ref <ref> | --cwd <path>]",
		Short:         "Display a tmux gate message (no-op on this platform)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Inline envelope: tmuxcmd is unix-only, so its types aren't
			// available here. Mirror tmuxcmd's NotifyResult shape exactly.
			env := map[string]any{
				"schema_version": 1,
				"ok":             true,
				"subcommand":     "notify",
				"steps": []map[string]string{
					{"status": "skip", "message": "tmux is unix-only; notification skipped"},
				},
				"notify": map[string]any{
					"notified": false,
					"reason":   "not_supported",
				},
			}
			if jsonOut {
				b, _ := json.MarshalIndent(env, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if !quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "tmux notification not sent (not_supported)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Notification title (required)")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Notification subtitle")
	cmd.Flags().StringVar(&body, "body", "", "Notification body")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit tmux target")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the window by pane_current_path")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
