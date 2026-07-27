//go:build unix

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/tmuxcmd"
)

// NewTmuxCmd builds `lets tmux` with its open/rename/notify subcommands.
func NewTmuxCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tmux",
		Short:         "Launch LETS worktrees in tmux windows/sessions (Linux + macOS, optional)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newTmuxOpenCmd())
	root.AddCommand(newTmuxRenameCmd())
	root.AddCommand(newTmuxNotifyCmd())
	return root
}

func newTmuxOpenCmd() *cobra.Command {
	var (
		name, description, command string
		force, jsonOut, quiet      bool
	)
	cmd := &cobra.Command{
		Use:           "open <path>",
		Short:         "Open a directory in a tmux window/session (falls back to a manual command when tmux is absent)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, runErr := tmuxcmd.Open(cmd.Context(), tmuxcmd.OpenOptions{
				Path:        args[0],
				Name:        name,
				Description: description,
				Command:     command,
				Force:       force,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet && res.OK {
				// On a hard error main.go prints the typed error to stderr;
				// skip the renderer to avoid a duplicate line (mirrors cmux.go).
				tmuxcmd.RenderOpen(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Session/window label (short readable slug)")
	cmd.Flags().StringVar(&description, "description", "", "Task identity stamped into the @lets_task window option (e.g. \"<task-id> · <title>\")")
	cmd.Flags().StringVar(&command, "command", "", "Command to run in the pane (e.g. claude '/lets:start <id>')")
	cmd.Flags().BoolVar(&force, "force", false, "Open even if a tmux pane already lives at this path (skip the duplicate guard)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newTmuxRenameCmd() *cobra.Command {
	var (
		title, ref, cwd string
		jsonOut, quiet  bool
	)
	cmd := &cobra.Command{
		Use:           "rename --title <new> [--ref <ref> | --cwd <path>]",
		Short:         "Rename a tmux window (resolves the active window, or by --cwd / --ref)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := tmuxcmd.Rename(cmd.Context(), tmuxcmd.RenameOptions{
				Title: title,
				Ref:   ref,
				Cwd:   cwd,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet && res.OK {
				tmuxcmd.RenderRename(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "New window name (required)")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit tmux target session:window (else resolve by --cwd or the active window)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the window by pane_current_path (else use the active window)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newTmuxNotifyCmd() *cobra.Command {
	var (
		title, subtitle, body, ref, cwd string
		jsonOut, quiet                  bool
	)
	cmd := &cobra.Command{
		Use:           "notify --title <text> [--body <text>] [--ref <ref> | --cwd <path>]",
		Short:         "Display a gate message on every attached tmux client (resolves by --cwd / --ref)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := tmuxcmd.Notify(cmd.Context(), tmuxcmd.NotifyOptions{
				Title:    title,
				Subtitle: subtitle,
				Body:     body,
				Ref:      ref,
				Cwd:      cwd,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet && res.OK {
				tmuxcmd.RenderNotify(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Notification title (required)")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Notification subtitle")
	cmd.Flags().StringVar(&body, "body", "", "Notification body")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit tmux target (else resolve by --cwd or the active window)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the window by pane_current_path (else use the active window)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
