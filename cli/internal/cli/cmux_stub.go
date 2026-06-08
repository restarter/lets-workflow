//go:build !unix

package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// errCmuxUnsupported is the canonical "not on this platform" error for
// `lets cmux`. cmux is a macOS terminal; the launcher only makes sense there.
// The subcommand still exists in --help so `lets --help` stays informative.
var errCmuxUnsupported = errors.New("lets cmux is macOS-only (cmux terminal); use the manual new-terminal flow on this platform")

// NewCmuxCmd returns the cmux subcommand stub on non-unix platforms.
func NewCmuxCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "cmux",
		Short:         "Launch worktrees in cmux (macOS only)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// open/rename are interactive-only: a hard error is the right non-unix
	// signal (no JSON caller depends on them degrading).
	for _, use := range []struct{ name, short string }{
		{"open <path>", "Open in cmux (not supported on this platform)"},
		{"rename", "Rename a cmux workspace (not supported on this platform)"},
	} {
		root.AddCommand(&cobra.Command{
			Use:           use.name,
			Short:         use.short,
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE:          func(_ *cobra.Command, _ []string) error { return errCmuxUnsupported },
		})
	}
	// notify is fired from gate snippets (often with --json) - so unlike
	// open/rename it must NOT hard-fail here. It emits the SAME graceful
	// envelope the unix darwin-less path returns (ok=true, notified=false,
	// reason=not_macos) and exits 0, so a --json caller on a non-unix host gets
	// a parseable result instead of a bare non-zero exit.
	root.AddCommand(newCmuxNotifyStub())
	return root
}

func newCmuxNotifyStub() *cobra.Command {
	var (
		title, subtitle, body, ref, cwd string
		jsonOut, quiet                  bool
	)
	cmd := &cobra.Command{
		Use:           "notify --title <text> [--body <text>] [--ref <ref> | --cwd <path>]",
		Short:         "Send a cmux notification (no-op on this platform)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Inline envelope: cmuxcmd is unix-only, so its types aren't
			// available here. Mirror cmuxcmd's NotifyResult shape exactly.
			env := map[string]any{
				"schema_version": 1,
				"ok":             true,
				"subcommand":     "notify",
				"steps": []map[string]string{
					{"status": "skip", "message": "cmux is macOS-only; notification skipped"},
				},
				"notify": map[string]any{
					"notified": false,
					"reason":   "not_macos",
				},
			}
			if jsonOut {
				b, _ := json.MarshalIndent(env, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if !quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "cmux notification not sent (not_macos)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Notification title (required)")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Notification subtitle")
	cmd.Flags().StringVar(&body, "body", "", "Notification body")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit workspace ref/uuid/index")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the workspace by current_directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
