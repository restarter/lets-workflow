package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// NewVersionCmd returns a cobra command that prints the CLI version.
// Output format matches cobra's default --version template ("lets version X.Y.Z")
// so both `lets version` and `lets --version` produce identical strings.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the lets CLI version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "lets version %s\n", version.Version)
		},
	}
}
