//go:build !unix

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// NewHandoffCmd returns the handoff stub on non-unix platforms: every subcommand
// emits a parseable not_supported envelope and exits 0, so the command's JSON
// reading still works.
func NewHandoffCmd() *cobra.Command {
	root := &cobra.Command{Use: "handoff", Short: "Hand-off delivery (not supported on this platform)", SilenceUsage: true, SilenceErrors: true}
	subs := []struct {
		use, key string
		inner    map[string]any
	}{
		{"targets", "targets", map[string]any{"available": false, "terminals": []any{}, "reason": "not_supported"}},
		{"send", "send", map[string]any{"delivery": "skipped", "agent": "", "created": false, "reason": "not_supported"}},
		{"codex", "run", map[string]any{"provider": "codex", "ran": false, "complete": false, "reason": "not_supported"}},
		{"await", "run", map[string]any{"provider": "", "ran": false, "complete": false, "reason": "not_supported"}},
	}
	for _, sub := range subs {
		sub := sub
		c := &cobra.Command{
			Use: sub.use, Short: "handoff " + sub.use + " (no-op on this platform)", SilenceUsage: true, SilenceErrors: true,
			RunE: func(cmd *cobra.Command, _ []string) error {
				b, _ := json.MarshalIndent(map[string]any{"schema_version": 1, "ok": true, "subcommand": sub.use, "steps": []any{}, sub.key: sub.inner}, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			},
		}
		for _, name := range []string{"brief", "terminal", "new", "match", "agent", "since", "fingerprint", "timeout"} {
			c.Flags().String(name, "", "")
		}
		c.Flags().Bool("json", false, "")
		root.AddCommand(c)
	}
	return root
}
