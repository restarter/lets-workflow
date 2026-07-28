//go:build !unix

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// NewNotifyCmd returns the launcher-neutral notify stub on non-unix platforms.
// Like the tmux/cmux notify stubs, it NEVER hard-fails: gate snippets fire it
// with --json, so it emits a parseable graceful envelope and exits 0.
func NewNotifyCmd() *cobra.Command {
	var (
		title, subtitle, body, ref, cwd string
		jsonOut, quiet                  bool
	)
	cmd := &cobra.Command{
		Use:           "notify --title <text> [--body <text>] [--ref <ref> | --cwd <path>]",
		Short:         "Send a gate notification through the active LETS_LAUNCHER (no-op on this platform)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := map[string]any{
				"schema_version": 1,
				"ok":             true,
				"subcommand":     "notify",
				"steps": []map[string]string{
					{"status": "skip", "message": "launcher notifications are unix-only; skipped"},
				},
				"notify": map[string]any{
					"notified": false,
					"launcher": "",
					"reason":   "not_supported",
				},
			}
			if jsonOut {
				b, _ := json.MarshalIndent(env, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if !quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "notification not sent (not_supported)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Notification title (required)")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Notification subtitle")
	cmd.Flags().StringVar(&body, "body", "", "Notification body")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit launcher target")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the target by directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
