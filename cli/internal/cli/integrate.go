//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/integratecmd"
)

// NewIntegrateCmd builds `lets integrate`: land since..from of an isolated
// implementer's branch in this tree as a staged patch, through a temporary
// worktree, never a merge and never `reset --hard`. The lead commits.
func NewIntegrateCmd() *cobra.Command {
	var (
		o       integratecmd.Options
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:           "integrate",
		Short:         "Stage an isolated implementer's commits in this tree as one verified patch",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, _ := os.Getwd()
			root := gitutil.ProjectRoot(cwd, 2*time.Second)
			if root == "" {
				e := &integratecmd.Error{Code: integratecmd.ExitGeneric, Kind: "not_in_repo", Message: "not inside a git repository"}
				return printIntegrate(cmd, jsonOut, integratecmd.NewErrorResult("integrate", e.Kind, e.Message), e)
			}
			res, err := integratecmd.Run(cmd.Context(), root, o)
			return printIntegrate(cmd, jsonOut, res, err)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.From, "from", "", "Commit-ish to integrate up to (an agent branch or a chunk sha)")
	f.StringVar(&o.Since, "since", "", "Exclusive start of the range: the group's integrated source, or BASE")
	f.StringVar(&o.Run, "run", "", "Run id ([a-z0-9-]{1,40})")
	f.StringVar(&o.Chunk, "chunk", "", "Chunk id ([a-z0-9-]{1,40})")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

// printIntegrate writes res as JSON, or human for a terminal, and returns runErr.
func printIntegrate(cmd *cobra.Command, jsonOut bool, res *integratecmd.Result, runErr error) error {
	w := cmd.OutOrStdout()
	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(w, string(b))
		return runErr
	}
	if runErr == nil {
		if len(res.Picked) == 0 {
			fmt.Fprintln(w, "nothing to integrate")
		} else {
			fmt.Fprintf(w, "staged %d commit(s) from a temp worktree: %s\npatch: %s\n", len(res.Picked), strings.Join(res.Files, ", "), res.PatchPath)
		}
	}
	return runErr
}
