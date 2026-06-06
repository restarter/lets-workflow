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
	return root
}

func newCmuxOpenCmd() *cobra.Command {
	var (
		name, command         string
		focus, jsonOut, quiet bool
	)
	cmd := &cobra.Command{
		Use:           "open <path>",
		Short:         "Open a directory in a cmux workspace (falls back to a manual command when cmux is absent)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, runErr := cmuxcmd.Open(cmd.Context(), cmuxcmd.OpenOptions{
				Path:    args[0],
				Name:    name,
				Command: command,
				Focus:   focus,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet {
				cmuxcmd.RenderOpen(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Workspace label (short readable slug)")
	cmd.Flags().StringVar(&command, "command", "", "Command to run in the workspace (e.g. claude '/lets:start <id>')")
	cmd.Flags().BoolVar(&focus, "focus", false, "Focus the new workspace (OFF by default - `cmux workspace create` may not accept --focus; verify before enabling)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
