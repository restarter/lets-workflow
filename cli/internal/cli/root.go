package cli

import (
	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// NewRootCmd builds the root cobra command for the lets CLI.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "lets",
		Short:        "LETS workflow CLI - companion binary for the Claude Code plugin",
		Version:      version.Version,
		SilenceUsage: true,
	}
	cmd.AddCommand(NewVersionCmd())
	cmd.AddCommand(NewHookCmd())
	return cmd
}
