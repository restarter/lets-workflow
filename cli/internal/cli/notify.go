//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/notifycmd"
)

// NewNotifyCmd builds `lets notify` - the launcher-neutral gate sink. It
// dispatches on LETS_LAUNCHER (resolved from .lets/.env) to the active launcher.
func NewNotifyCmd() *cobra.Command {
	var (
		title, subtitle, body, ref, cwd string
		jsonOut, quiet                  bool
	)
	cmd := &cobra.Command{
		Use:           "notify --title <text> [--body <text>] [--ref <ref> | --cwd <path>]",
		Short:         "Send a gate notification through the active LETS_LAUNCHER (cmux | tmux; no-op on terminal)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _ := os.UserHomeDir()
			root := cwd
			if root == "" {
				root = "."
			}
			res, runErr := notifycmd.Notify(cmd.Context(), notifycmd.Options{
				Title: title, Subtitle: subtitle, Body: body, Ref: ref, Cwd: cwd,
				ProjectRoot: gitutil.ProjectRoot(root, 2*time.Second),
				HomeDir:     home,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet && res.OK {
				fmt.Fprintln(cmd.OutOrStdout(), renderNotify(res))
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Notification title (required)")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Notification subtitle")
	cmd.Flags().StringVar(&body, "body", "", "Notification body")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit launcher target (cmux ref / tmux session:window)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the target by directory (also locates .lets/.env)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func renderNotify(res *notifycmd.Result) string {
	if res.Notify == nil {
		return "notify: no result"
	}
	if res.Notify.Notified {
		return fmt.Sprintf("notified via %s: %q", res.Notify.Launcher, res.Notify.Title)
	}
	return fmt.Sprintf("not notified (%s / %s)", res.Notify.Launcher, res.Notify.Reason)
}
