package cli

import (
	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// NewRootCmd builds the root cobra command for the lets CLI.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "lets",
		Short:   "LETS workflow CLI - companion binary for the Claude Code plugin",
		Version: version.Version,
		// Silence both: SilenceUsage prevents cobra from dumping the usage block
		// after an error (we already showed help on demand). SilenceErrors stops
		// cobra from printing the error itself - main.go prints it once via
		// fmt.Fprintln(os.Stderr). Without SilenceErrors, errors show up TWICE
		// (cobra "Error: ..." + main.go's print). Bug had been discovered + fixed
		// during Phase 4b smoke testing then lost in the revert+stash drop;
		// re-applied here as a post-cleanup hotfix (sibling of the
		// DetectInsideWorktree filepath.Abs fix revived in Cleanup 5).
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewVersionCmd())
	cmd.AddCommand(NewHookCmd())
	cmd.AddCommand(NewStatuslineCmd())
	cmd.AddCommand(NewInitCmd())
	cmd.AddCommand(NewUpdateCmd())
	cmd.AddCommand(NewWorktreeCmd())
	return cmd
}
