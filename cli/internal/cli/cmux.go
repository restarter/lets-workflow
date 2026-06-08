//go:build unix

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/cmuxcmd"
)

// NewCmuxCmd builds `lets cmux` with its `open` subcommand.
func NewCmuxCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "cmux",
		Short:         "Launch LETS worktrees in cmux workspaces (macOS, optional)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCmuxOpenCmd())
	root.AddCommand(newCmuxRenameCmd())
	root.AddCommand(newCmuxNotifyCmd())
	return root
}

func newCmuxOpenCmd() *cobra.Command {
	var (
		name, description, command string
		force, jsonOut, quiet      bool
	)
	cmd := &cobra.Command{
		Use:           "open <path>",
		Short:         "Open a directory in a cmux workspace (falls back to a manual command when cmux is absent)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, runErr := cmuxcmd.Open(cmd.Context(), cmuxcmd.OpenOptions{
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
				// skip the renderer to avoid a duplicate line (mirrors worktree.go info).
				cmuxcmd.RenderOpen(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Workspace label (short readable slug)")
	cmd.Flags().StringVar(&description, "description", "", "Workspace description (e.g. \"<task-id> · <title>\")")
	cmd.Flags().StringVar(&command, "command", "", "Command to run in the workspace (e.g. claude '/lets:start <id>')")
	cmd.Flags().BoolVar(&force, "force", false, "Open even if a cmux workspace already targets this path (skip the duplicate-session guard)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newCmuxRenameCmd() *cobra.Command {
	var (
		title, ref, cwd string
		jsonOut, quiet  bool
	)
	cmd := &cobra.Command{
		Use:           "rename --title <new> [--ref <ref> | --cwd <path>]",
		Short:         "Rename a cmux workspace tab label (resolves the active workspace, or by --cwd / --ref)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := cmuxcmd.Rename(cmd.Context(), cmuxcmd.RenameOptions{
				Title: title,
				Ref:   ref,
				Cwd:   cwd,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet && res.OK {
				// Skip the renderer on hard error (main.go prints to stderr) to
				// avoid a duplicate line (mirrors worktree.go info).
				cmuxcmd.RenderRename(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "New workspace title (required)")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit workspace ref/uuid/index (else resolve by --cwd or the active workspace)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the workspace by current_directory (else use the active workspace)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newCmuxNotifyCmd() *cobra.Command {
	var (
		title, subtitle, body, ref, cwd string
		jsonOut, quiet                  bool
	)
	cmd := &cobra.Command{
		Use:           "notify --title <text> [--body <text>] [--ref <ref> | --cwd <path>]",
		Short:         "Send a notification to a cmux workspace (resolves the active workspace, or by --cwd / --ref)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := cmuxcmd.Notify(cmd.Context(), cmuxcmd.NotifyOptions{
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
				// Skip the renderer on hard error (main.go prints to stderr) to
				// avoid a duplicate line (mirrors worktree.go info).
				cmuxcmd.RenderNotify(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Notification title (required)")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Notification subtitle")
	cmd.Flags().StringVar(&body, "body", "", "Notification body")
	cmd.Flags().StringVar(&ref, "ref", "", "Explicit workspace ref/uuid/index (else resolve by --cwd or the active workspace)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Resolve the workspace by current_directory (else use the active workspace)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
