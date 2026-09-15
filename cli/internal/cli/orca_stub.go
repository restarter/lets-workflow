//go:build !unix

package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var errOrcaUnsupported = errors.New("lets orca is unix-only on this build; open the worktree with the manual flow")

// NewOrcaCmd returns the orca stub on non-unix platforms: open hard-errors (no JSON
// caller depends on it degrading); notify and status emit a graceful not_supported
// envelope and exit 0, so gate snippets and init's probe stay parseable.
func NewOrcaCmd() *cobra.Command {
	root := &cobra.Command{Use: "orca", Short: "Orca launcher (not supported on this platform)", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{
		Use: "open", Short: "Open in Orca (not supported on this platform)", SilenceUsage: true, SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error { return errOrcaUnsupported },
	})
	for _, sub := range []struct{ use, key string }{{"notify", "notify"}, {"status", "status"}} {
		sub := sub
		var jsonOut, quiet bool
		var title, body, cwd string
		c := &cobra.Command{
			Use: sub.use, Short: "Orca " + sub.use + " (no-op on this platform)", SilenceUsage: true, SilenceErrors: true,
			RunE: func(cmd *cobra.Command, _ []string) error {
				inner := map[string]any{"reason": "not_supported"}
				if sub.key == "notify" {
					inner["notified"] = false
				} else {
					inner["running"] = false
				}
				env := map[string]any{"schema_version": 1, "ok": true, "subcommand": sub.use,
					"steps": []map[string]string{{"status": "skip", "message": "orca is unix-only on this build"}}, sub.key: inner}
				if jsonOut {
					b, _ := json.MarshalIndent(env, "", "  ")
					fmt.Fprintln(cmd.OutOrStdout(), string(b))
				} else if !quiet {
					fmt.Fprintln(cmd.OutOrStdout(), "orca "+sub.use+": not_supported")
				}
				return nil
			},
		}
		c.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
		c.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
		if sub.key == "notify" {
			c.Flags().StringVar(&title, "title", "", "Note title")
			c.Flags().StringVar(&body, "body", "", "Note body")
			c.Flags().StringVar(&cwd, "cwd", "", "Worktree path")
		}
		root.AddCommand(c)
	}
	return root
}
