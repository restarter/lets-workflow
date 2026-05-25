//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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
					renderHumanCreate(cmd.ErrOrStderr(), res)
				}
			case jsonOut:
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			default:
				if !quiet {
					renderHumanCreate(cmd.OutOrStdout(), res)
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
				renderHumanRemove(cmd.OutOrStdout(), res)
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
				renderHumanList(cmd.OutOrStdout(), res)
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
				renderHumanInfo(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

// renderHumanCreate writes a short, one-screen summary.
func renderHumanCreate(w io.Writer, res *worktreecmd.CreateResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		if res.Error.Remediation != "" {
			fmt.Fprintf(w, "Hint: %s\n", res.Error.Remediation)
		}
		if res.Rollback != nil && len(res.Rollback.Residual) > 0 {
			fmt.Fprintf(w, "Residual paths (clean up manually): %s\n", strings.Join(res.Rollback.Residual, ", "))
		}
		return
	}
	if res.Worktree == nil {
		return
	}
	fmt.Fprintf(w, "Worktree created: %s\n", res.Worktree.Path)
	fmt.Fprintf(w, "Branch: %s (%s)\n", res.Worktree.Branch, res.Worktree.BranchMode)
	fmt.Fprintf(w, "Symlinks: lets=%v beads=%v\n", res.Worktree.LetsSymlinked, res.Worktree.BeadsSymlinked)
	fmt.Fprintf(w, "Next: cd %s && claude\n", res.Worktree.Path)
}

func renderHumanRemove(w io.Writer, res *worktreecmd.RemoveResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		if res.Error.Remediation != "" {
			fmt.Fprintf(w, "Hint: %s\n", res.Error.Remediation)
		}
		return
	}
	if res.Removed == nil {
		return
	}
	fmt.Fprintf(w, "Worktree removed: %s\n", res.Removed.Path)
	branchStatus := "kept"
	if res.Removed.BranchDeleted {
		branchStatus = "deleted"
	}
	fmt.Fprintf(w, "Branch: %s (%s)\n", res.Removed.Branch, branchStatus)
	if res.Removed.Forced {
		fmt.Fprintln(w, "Forced: true")
	}
}

func renderHumanList(w io.Writer, res *worktreecmd.ListResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		return
	}
	fmt.Fprintf(w, "%-12s %-22s %-12s %-7s %-7s %-12s %s\n",
		"NAME", "BRANCH", "KIND", "LETS", "BEADS", "CHANGES", "PATH")
	for _, wt := range res.Worktrees {
		changes := "clean"
		if !wt.ChangesClean {
			changes = fmt.Sprintf("%dm/%du", wt.ChangesModified, wt.ChangesUntracked)
		}
		fmt.Fprintf(w, "%-12s %-22s %-12s %-7v %-7v %-12s %s\n",
			wt.Name, wt.Branch, wt.Kind, wt.LetsSymlinked, wt.BeadsSymlinked, changes, wt.Path)
	}
	mainBranch := ""
	if res.Main != nil {
		mainBranch = res.Main.Branch
	}
	fmt.Fprintf(w, "\n%d worktrees (main: %s)\n", len(res.Worktrees), mainBranch)
}

func renderHumanInfo(w io.Writer, res *worktreecmd.InfoResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		return
	}
	fmt.Fprintf(w, "In worktree: %v\n", res.InWorktree)
	if res.Worktree != nil {
		fmt.Fprintf(w, "Path:        %s\n", res.Worktree.Path)
		if res.InWorktree {
			fmt.Fprintf(w, "Main repo:   %s\n", res.MainRoot)
		}
		fmt.Fprintf(w, "Branch:      %s\n", res.Worktree.Branch)
		if res.InWorktree {
			lets := "local"
			if res.Worktree.LetsSymlinked {
				lets = "symlinked"
			}
			beads := "local"
			if res.Worktree.BeadsSymlinked {
				beads = "shared"
			}
			fmt.Fprintf(w, "LETS:        %s\n", lets)
			fmt.Fprintf(w, "Beads:       %s\n", beads)
		}
		changes := "clean"
		if !res.Worktree.ChangesClean {
			changes = fmt.Sprintf("%d modified, %d untracked",
				res.Worktree.ChangesModified, res.Worktree.ChangesUntracked)
		}
		fmt.Fprintf(w, "Changes:     %s\n", changes)
	}
}
