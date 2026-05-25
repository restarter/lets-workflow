//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// NewWorktreeCmd builds `lets worktree` with its 4 subcommand factories.
// Subcommands inherit SilenceUsage + SilenceErrors so cobra doesn't double-
// print after a JSON envelope is emitted. Stream contract per subcommand:
//
//   --print-cd : stdout = path (success), stderr = JSON when --json, else
//                human-readable prose suppressed unless --verbose.
//   --json     : stdout = JSON envelope; human prose suppressed.
//   default    : stdout = human-readable rendering.
//   --quiet    : suppresses human prose entirely (JSON/path paths unaffected).
func NewWorktreeCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "worktree",
		Short:         "Manage interactive git worktrees with LETS-managed symlinks",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	for _, sub := range []*cobra.Command{
		newWorktreeCreateCmd(),
		newWorktreeRemoveCmd(),
		newWorktreeListCmd(),
		newWorktreeInfoCmd(),
	} {
		sub.SilenceUsage = true
		sub.SilenceErrors = true
		root.AddCommand(sub)
	}
	return root
}

func newWorktreeCreateCmd() *cobra.Command {
	var (
		attach, newBranch, noSymLets, noSymBeads bool
		printCD, switchMain, jsonOut, quiet      bool
		verbose                                  bool
		base                                     string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree or attach an existing branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if attach && newBranch {
				return &worktreecmd.Error{
					Code:    worktreecmd.ExitUsage,
					Kind:    "flag_conflict",
					Message: "--attach and --new-branch are mutually exclusive",
				}
			}
			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return &worktreecmd.Error{
					Code:    worktreecmd.ExitNotInRepo,
					Kind:    "not_in_repo",
					Message: "not inside a git repository",
				}
			}
			mode := worktreecmd.BranchAuto
			if attach {
				mode = worktreecmd.BranchAttach
			}
			if newBranch {
				mode = worktreecmd.BranchNewBranch
			}
			res, runErr := worktreecmd.Create(cmd.Context(), projectRoot, worktreecmd.CreateOptions{
				Name:               args[0],
				Mode:               mode,
				Base:               base,
				NoSymlinkLets:      noSymLets,
				NoSymlinkBeads:     noSymBeads,
				SwitchMainIfNeeded: switchMain,
			})

			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			switch {
			case printCD:
				if res.OK && res.Worktree != nil {
					fmt.Fprintln(cmd.OutOrStdout(), res.Worktree.Path)
				}
				if jsonOut {
					fmt.Fprintln(cmd.ErrOrStderr(), string(jsonBytes))
				} else if verbose && !quiet {
					worktreecmd.RenderCreate(cmd.ErrOrStderr(), res)
				}
			case jsonOut:
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			default:
				if !quiet {
					worktreecmd.RenderCreate(cmd.OutOrStdout(), res)
				}
			}
			return runErr
		},
	}
	cmd.Flags().BoolVar(&attach, "attach", false, "Force attach to an existing branch")
	cmd.Flags().BoolVar(&newBranch, "new-branch", false, "Force create new branch (refuse if exists)")
	cmd.Flags().StringVar(&base, "base", "", "Base ref for new branch (default: LETS_MERGE_BRANCH or main)")
	cmd.Flags().BoolVar(&noSymLets, "no-symlink-lets", false, "Skip .lets/ symlink")
	cmd.Flags().BoolVar(&noSymBeads, "no-symlink-beads", false, "Skip .beads/.env symlink")
	cmd.Flags().BoolVar(&printCD, "print-cd", false, "Print worktree path to stdout (JSON to stderr); for $(...) substitution")
	cmd.Flags().BoolVar(&switchMain, "switch-main-if-needed", false, "Auto-switch main repo if attaching its current branch (requires clean tree)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "In --print-cd mode, also write human prose to stderr")
	return cmd
}

func newWorktreeRemoveCmd() *cobra.Command {
	var (
		force, deleteBranch, forceBranch, branchOnly bool
		jsonOut, quiet                               bool
		branch                                       string
	)
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a worktree and optionally delete its branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return &worktreecmd.Error{
					Code: worktreecmd.ExitNotInRepo,
					Kind: "not_in_repo",
				}
			}
			res, runErr := worktreecmd.Remove(cmd.Context(), projectRoot, worktreecmd.RemoveOptions{
				Name:         args[0],
				Force:        force,
				DeleteBranch: deleteBranch,
				ForceBranch:  forceBranch,
				BranchOnly:   branchOnly,
				Branch:       branch,
			})
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet {
				worktreecmd.RenderRemove(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Skip uncommitted-changes guard")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Also delete the branch after removing worktree")
	cmd.Flags().BoolVar(&forceBranch, "force-branch", false, "Use -D (force delete unmerged) instead of -d")
	cmd.Flags().BoolVar(&branchOnly, "branch-only", false, "Skip worktree removal; only delete the branch (use after a prior remove). Requires --branch and --delete-branch")
	cmd.Flags().StringVar(&branch, "branch", "", "Explicit branch name (used with --branch-only)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newWorktreeListCmd() *cobra.Command {
	var jsonOut, quiet bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List worktrees with LETS annotations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return &worktreecmd.Error{
					Code: worktreecmd.ExitNotInRepo,
					Kind: "not_in_repo",
				}
			}
			res, runErr := worktreecmd.List(cmd.Context(), projectRoot)
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet {
				worktreecmd.RenderList(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newWorktreeInfoCmd() *cobra.Command {
	var jsonOut, quiet bool
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show worktree status for the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return &worktreecmd.Error{
					Code:    worktreecmd.ExitFilesystem,
					Kind:    "getwd_failed",
					Message: err.Error(),
					Cause:   err,
				}
			}
			res, runErr := worktreecmd.Info(cmd.Context(), cwd)
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet {
				worktreecmd.RenderInfo(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

// Human-readable rendering lives in cli/internal/worktreecmd/render.go
// (review S-8: domain package owns presentation alongside the envelope
// shape; mirrors updatecmd.PrintReport precedent).
