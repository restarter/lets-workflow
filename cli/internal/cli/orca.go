//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// NewOrcaCmd builds `lets orca` (open / notify / status). Orca is an opt-in addon:
// these commands run only when invoked explicitly or when LETS_LAUNCHER=orca routes
// a flow here.
func NewOrcaCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "orca",
		Short:         "Open LETS worktrees through the Orca app and write gate notes to its card (optional addon)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newOrcaOpenCmd(), newOrcaNotifyCmd(), newOrcaStatusCmd(), newOrcaCardCmd(), newOrcaReposCmd(), newOrcaWakeCmd())
	return root
}

func printOrca(cmd *cobra.Command, jsonOut, quiet, ok bool, res any, render func()) {
	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
	} else if !quiet && ok {
		render()
	}
}

func newOrcaOpenCmd() *cobra.Command {
	var (
		repo, name, prompt    string
		force, jsonOut, quiet bool
	)
	cmd := &cobra.Command{
		Use:           "open",
		Short:         "Create an Orca worktree running Claude (falls back when Orca is unavailable)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := orcacmd.Open(cmd.Context(), orcacmd.OpenOptions{Repo: repo, Name: name, Prompt: prompt, Force: force})
			printOrca(cmd, jsonOut, quiet, res.OK, res, func() { orcacmd.RenderOpen(cmd.OutOrStdout(), res) })
			return runErr
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Main checkout of the repository")
	cmd.Flags().StringVar(&name, "name", "", "Worktree name (Orca turns `/` into `-`; pass a `/`-free name)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "First message for the agent (e.g. /lets:start <id>)")
	cmd.Flags().BoolVar(&force, "force", false, "Open even when a worktree with this name is already open")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newOrcaNotifyCmd() *cobra.Command {
	var (
		title, body, cwd string
		jsonOut, quiet   bool
	)
	cmd := &cobra.Command{
		Use:           "notify",
		Short:         "Write a gate note as this worktree's Orca card comment",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := orcacmd.Notify(cmd.Context(), orcacmd.NotifyOptions{Title: title, Body: body, Cwd: cwd})
			printOrca(cmd, jsonOut, quiet, res.OK, res, func() { orcacmd.RenderNotify(cmd.OutOrStdout(), res) })
			return runErr
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Note title")
	cmd.Flags().StringVar(&body, "body", "", "Note body")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Worktree path to match when ORCA_WORKTREE_ID does not identify this checkout")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newOrcaCardCmd() *cobra.Command {
	var (
		phase, comment string
		jsonOut, quiet bool
	)
	cmd := &cobra.Command{
		Use:           "card",
		Short:         "Mirror a LETS phase onto this worktree's Orca card (only with LETS_LAUNCHER=orca)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, _ := os.Getwd()
			home, _ := os.UserHomeDir()
			root := gitutil.ProjectRoot(cwd, 2*time.Second)
			launcher := ""
			if root != "" {
				launcher = letsconfig.ResolvedEnv(root, home, nil)["LETS_LAUNCHER"]
			}
			res, runErr := orcacmd.Card(cmd.Context(), orcacmd.CardOptions{Phase: phase, Comment: comment, Launcher: launcher})
			printOrca(cmd, jsonOut, quiet, res.OK, res, func() {
				if res.Card != nil && res.Card.Updated {
					fmt.Fprintf(cmd.OutOrStdout(), "orca card: %s\n", res.Card.Phase)
				} else if res.Card != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "orca card skipped: %s\n", res.Card.Reason)
				}
			})
			return runErr
		},
	}
	cmd.Flags().StringVar(&phase, "phase", "", "start | pr | closed | end | blocked | gate")
	cmd.Flags().StringVar(&comment, "comment", "", "Card comment (redacted, at most 280 characters)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newOrcaReposCmd() *cobra.Command {
	var jsonOut, quiet bool
	cmd := &cobra.Command{
		Use: "repos", Short: "List the repos Orca knows that are main checkouts (by index)", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := orcacmd.Repos(cmd.Context())
			printOrca(cmd, jsonOut, quiet, res.OK, res, func() {
				for _, r := range res.Repos.Repos {
					fmt.Fprintf(cmd.OutOrStdout(), "%d  %s\n", r.Index, r.Name)
				}
			})
			return runErr
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newOrcaWakeCmd() *cobra.Command {
	var (
		o              orcacmd.WakeOptions
		repoIndex      int
		jsonOut, quiet bool
	)
	cmd := &cobra.Command{
		Use: "wake", Short: "Resume a stopped orchestrator in a visible Orca terminal (refuses a live or unknown one)", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.Repo == "" && repoIndex >= 0 {
				p, f := orcacmd.RepoByIndex(cmd.Context(), repoIndex)
				if f != nil {
					res := &orcacmd.WakeResult{Envelope: orcacmd.Envelope{SchemaVersion: orcacmd.SchemaVersion, OK: true, Subcommand: "wake", Steps: []orcacmd.Step{}}, Wake: &orcacmd.WakeInfo{Reason: f.Reason}}
					printOrca(cmd, jsonOut, quiet, true, res, func() { fmt.Fprintln(cmd.OutOrStdout(), "orca wake: "+f.Reason) })
					return nil
				}
				o.Repo = p
			}
			res, runErr := orcacmd.Wake(cmd.Context(), o)
			printOrca(cmd, jsonOut, quiet, res.OK, res, func() {
				if res.Wake != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "orca wake: woken=%v %s\n", res.Wake.Woken, res.Wake.Reason)
				}
			})
			return runErr
		},
	}
	cmd.Flags().StringVar(&o.Repo, "repo", "", "The project's main checkout")
	cmd.Flags().IntVar(&repoIndex, "repo-index", -1, "An index from lets orca repos")
	cmd.Flags().StringVar(&o.Session, "session", "", "The orchestrator's last-seen session id")
	cmd.Flags().IntVar(&o.Pid, "pid", 0, "Its recorded pid")
	cmd.Flags().StringVar(&o.Title, "title", "", "The orchestrator name (terminal title)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newOrcaStatusCmd() *cobra.Command {
	var jsonOut, quiet bool
	cmd := &cobra.Command{
		Use:           "status",
		Short:         "Report whether the Orca CLI resolves and the app is running",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := orcacmd.Status(cmd.Context())
			printOrca(cmd, jsonOut, quiet, res.OK, res, func() { orcacmd.RenderStatus(cmd.OutOrStdout(), res) })
			return runErr
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}
