//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/notifycmd"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// emitErrorEnvelope is invoked from RunE early-returns that short-circuit
// before worktreecmd builds an envelope (flag conflicts, not_in_repo,
// getwd_failed). When jsonOut is set, writes a minimal envelope to w so
// --json consumers parse valid JSON; otherwise it is a no-op. The error
// is returned unmodified for `return emitErrorEnvelope(...)`-style use.
func emitErrorEnvelope(w io.Writer, jsonOut bool, subcommand string, e *worktreecmd.Error) error {
	if jsonOut {
		env := worktreecmd.NewErrorEnvelope(subcommand, e.Kind, e.Message)
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Fprintln(w, string(b))
	}
	return e
}

// NewWorktreeCmd builds `lets worktree` with its subcommand factories.
// Subcommands inherit SilenceUsage + SilenceErrors so cobra doesn't double-
// print after a JSON envelope is emitted. Stream contract per subcommand:
//
//	--print-cd : stdout = path (success), stderr = JSON when --json, else
//	             human-readable prose suppressed unless --verbose.
//	--json     : stdout = JSON envelope; human prose suppressed.
//	default    : stdout = human-readable rendering.
//	--quiet    : suppresses human prose entirely (JSON/path paths unaffected).
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
		newWorktreeAdoptCmd(),
		newWorktreeReleaseCmd(),
		newWorktreeRecordCmd(),
		newWorktreeTaskStateCmd(),
		newWorktreeBranchNameCmd(),
		newWorktreeSweepCmd(),
		newWorktreePushedCmd(),
		newWorktreeTeamInitCmd(),
		newWorktreeSwitchCmd(),
		newWorktreeParkedCmd(),
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
		base, branch, pluginRoot                 string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree or attach an existing branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// For create's early-returns, --json envelope goes to stderr when
			// --print-cd is set (matching the success-path printCD+json branch
			// below), otherwise stdout.
			errOut := cmd.OutOrStdout()
			if printCD {
				errOut = cmd.ErrOrStderr()
			}
			if attach && newBranch {
				return emitErrorEnvelope(errOut, jsonOut, "create", &worktreecmd.Error{
					Code:    worktreecmd.ExitUsage,
					Kind:    "flag_conflict",
					Message: "--attach and --new-branch are mutually exclusive",
				})
			}
			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return emitErrorEnvelope(errOut, jsonOut, "create", &worktreecmd.Error{
					Code:    worktreecmd.ExitNotInRepo,
					Kind:    "not_in_repo",
					Message: "not inside a git repository",
				})
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
				Branch:             branch,
				Mode:               mode,
				Base:               base,
				NoSymlinkLets:      noSymLets,
				NoStoreLinks:       noSymBeads,
				PluginRoot:         pluginRoot,
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
	cmd.Flags().StringVar(&branch, "branch", "", "Branch ref to attach/create, decoupled from <name> (allows '/', e.g. feature/x)")
	cmd.Flags().BoolVar(&noSymLets, "no-symlink-lets", false, "Skip .lets/ symlink")
	cmd.Flags().BoolVar(&noSymBeads, "no-store-links", false, "Skip the tracker adapter's declared store links")
	cmd.Flags().BoolVar(&noSymBeads, "no-symlink-beads", false, "Deprecated alias of --no-store-links")
	_ = cmd.Flags().MarkHidden("no-symlink-beads")
	cmd.Flags().StringVar(&pluginRoot, "plugin-root", "", "Plugin root for the tracker adapter fallback (default: $CLAUDE_PLUGIN_ROOT)")
	cmd.Flags().BoolVar(&printCD, "print-cd", false, "Print worktree path to stdout (pair with --json or --verbose to also emit stderr); for $(...) substitution")
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
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "remove", &worktreecmd.Error{
					Code:    worktreecmd.ExitNotInRepo,
					Kind:    "not_in_repo",
					Message: "not inside a git repository",
				})
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
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "list", &worktreecmd.Error{
					Code:    worktreecmd.ExitNotInRepo,
					Kind:    "not_in_repo",
					Message: "not inside a git repository",
				})
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
	var jsonOut, quiet, taskCandidate bool
	var dir, refFile, pluginRoot string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show worktree status for the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := dirOrCwd(dir)
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "info", &worktreecmd.Error{
					Code:    worktreecmd.ExitFilesystem,
					Kind:    "getwd_failed",
					Message: err.Error(),
					Cause:   err,
				})
			}
			if refFile != "" && !taskCandidate {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "info", &worktreecmd.Error{
					Code: worktreecmd.ExitUsage, Kind: "flag_conflict", Message: "--ref-file requires --task-candidate",
				})
			}
			var res *worktreecmd.InfoResult
			var runErr error
			if taskCandidate {
				res, runErr = worktreecmd.TaskCandidateFor(cmd.Context(), target, refFile, pluginRoot)
			} else {
				res, runErr = worktreecmd.Info(cmd.Context(), target)
			}
			jsonBytes, _ := json.MarshalIndent(res, "", "  ")
			if jsonOut {
				fmt.Fprintln(cmd.OutOrStdout(), string(jsonBytes))
			} else if !quiet && res.OK {
				// On error, skip the human renderer — main.go prints the
				// error to stderr via err.Error(). Without this guard,
				// RenderInfo's "Error: ..." line duplicates main.go's
				// "kind: message" line.
				worktreecmd.RenderInfo(cmd.OutOrStdout(), res)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to inspect (default: current directory)")
	cmd.Flags().BoolVar(&taskCandidate, "task-candidate", false, "Return only the task id the active convention reads off the branch name (created shapes)")
	cmd.Flags().StringVar(&refFile, "ref-file", "", "With --task-candidate: read the branch ref from this file instead of HEAD (an untrusted ref is never typed into a shell)")
	cmd.Flags().StringVar(&pluginRoot, "plugin-root", "", "With --task-candidate: plugin root for the tracker adapter fallback (default: $CLAUDE_PLUGIN_ROOT)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

// dirOrCwd returns dir, or the current working directory when dir is empty.
func dirOrCwd(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	return os.Getwd()
}

// emitJSONOrRender prints res as JSON (--json) or through render, and returns runErr.
func emitJSONOrRender(cmd *cobra.Command, jsonOut, quiet bool, res any, render func(), runErr error) error {
	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
	} else if !quiet && render != nil {
		render()
	}
	return runErr
}

func newWorktreeAdoptCmd() *cobra.Command {
	var jsonOut, quiet, replaceTask, linksOnly bool
	var dir, task, pluginRoot string
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Make a worktree created elsewhere (Orca, git worktree add) a LETS worktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := dirOrCwd(dir)
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "adopt", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.Adopt(cmd.Context(), target, worktreecmd.AdoptOptions{
				Task: task, ReplaceTask: replaceTask, LinksOnly: linksOnly, PluginRoot: pluginRoot,
			})
			return emitJSONOrRender(cmd, jsonOut, quiet, res, func() { worktreecmd.RenderSteps(cmd.OutOrStdout(), res.Envelope) }, runErr)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Worktree directory (default: current directory)")
	cmd.Flags().StringVar(&task, "task", "", "Task id to record (supersedes an id derived from the branch name)")
	cmd.Flags().BoolVar(&replaceTask, "replace-task", false, "Replace a task-state file that names a different task (for a human at a terminal)")
	cmd.Flags().BoolVar(&linksOnly, "links-only", false, "Link .lets and the store; skip the task step")
	cmd.Flags().StringVar(&pluginRoot, "plugin-root", "", "Plugin root for the tracker adapter fallback (default: $CLAUDE_PLUGIN_ROOT)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

// releaseNotify is the notification sink of a release that lost a record (a seam).
var releaseNotify = notifycmd.Notify

func newWorktreeReleaseCmd() *cobra.Command {
	var jsonOut, quiet bool
	var dir string
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Record a worktree's state before it goes away (never refuses for dirty or unpushed work)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := dirOrCwd(dir)
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "release", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.Release(cmd.Context(), target, worktreecmd.ReleaseOptions{})
			if alarm := res.Alarm(); alarm != "" {
				// Loud even under --quiet: a hook's stdout is not a signal.
				fmt.Fprintln(cmd.ErrOrStderr(), "lets: "+alarm)
				home, _ := os.UserHomeDir()
				_, _ = releaseNotify(cmd.Context(), notifycmd.Options{
					Title: "LETS: a worktree left without a record", Body: alarm,
					Cwd: res.ProjectRoot, ProjectRoot: res.ProjectRoot, HomeDir: home,
				})
			}
			return emitJSONOrRender(cmd, jsonOut, quiet, res, func() { worktreecmd.RenderSteps(cmd.OutOrStdout(), res.Envelope) }, runErr)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Worktree directory (default: current directory)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newWorktreeRecordCmd() *cobra.Command {
	var jsonOut bool
	var o worktreecmd.RecordOptions
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Report whether each task left a session record, and whether it is orphaned in this checkout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "record", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.TaskRecord(cmd.Context(), cwd, o)
			return emitJSONOrRender(cmd, jsonOut, false, res, func() { worktreecmd.RenderRecord(cmd.OutOrStdout(), res) }, runErr)
		},
	}
	cmd.Flags().StringArrayVar(&o.Tasks, "task", nil, "Task id (repeatable)")
	cmd.Flags().StringVar(&o.Ref, "ref", "", "Commit the record must cover (one --task only; default: the task's worktree HEAD, else its branch tip)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newWorktreeTaskStateCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "task-state",
		Short: "Read or write this branch's task-state file (.lets/sessions/.task-<branch-slug>)",
	}
	var o worktreecmd.TaskStateOptions
	var setJSON, showJSON bool
	set := &cobra.Command{
		Use:   "set",
		Short: "Change only the given keys of the current branch's task-state file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), setJSON, "task-state", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.TaskStateSet(cmd.Context(), cwd, o)
			return emitJSONOrRender(cmd, setJSON, false, res, func() { worktreecmd.RenderTaskState(cmd.OutOrStdout(), res) }, runErr)
		},
	}
	set.Flags().StringVar(&o.Task, "task", "", "Task id")
	set.Flags().StringVar(&o.Start, "start", "", "Task boundary commit (7-64 hex)")
	set.Flags().StringVar(&o.SessionSHA, "session-sha", "", "Session boundary commit (with --session-id)")
	set.Flags().StringVar(&o.SessionID, "session-id", "", "Claude Code session id (with --session-sha)")
	set.Flags().StringVar(&o.Orc, "orc", "", "Bind this branch to an orchestrator name (never on the merge-branch)")
	set.Flags().BoolVar(&o.ClearOrigin, "clear-origin", false, "Remove origin: (the id is confirmed)")
	set.Flags().BoolVar(&o.ClearOrc, "clear-orc", false, "Remove orc:")
	set.Flags().BoolVar(&o.ClearTask, "clear-task", false, "Remove task:, start: and origin: (keeps session: and every other line)")
	set.Flags().BoolVar(&o.Create, "create", false, "Create the file when it is missing")
	set.Flags().DurationVar(&o.Wait, "wait", 5*time.Second, "Lock deadline")
	set.Flags().BoolVar(&setJSON, "json", false, "Emit JSON envelope")
	show := &cobra.Command{
		Use:   "show",
		Short: "Print the current branch's task-state file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), showJSON, "task-state", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.TaskStateShow(cmd.Context(), cwd)
			return emitJSONOrRender(cmd, showJSON, false, res, func() { worktreecmd.RenderTaskState(cmd.OutOrStdout(), res) }, runErr)
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, "Emit JSON envelope")
	for _, c := range []*cobra.Command{set, show} {
		c.SilenceUsage, c.SilenceErrors = true, true
		root.AddCommand(c)
	}
	return root
}

func newWorktreeSweepCmd() *cobra.Command {
	var jsonOut, quiet, apply bool
	cmd := &cobra.Command{
		Use:   "sweep",
		Short: "List (or with --apply delete) local task branches already merged upstream",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "sweep", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.Sweep(cmd.Context(), cwd, apply)
			return emitJSONOrRender(cmd, jsonOut, quiet, res, func() { worktreecmd.RenderSweep(cmd.OutOrStdout(), res) }, runErr)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Delete the merged branches (default: dry run)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress human-readable output")
	return cmd
}

func newWorktreeBranchNameCmd() *cobra.Command {
	var jsonOut, worktree bool
	var task, titleFile, pluginRoot string
	cmd := &cobra.Command{
		Use:   "branch-name",
		Short: "Render the branch LETS creates for a task under the tracker's naming convention",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "branch-name", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.BranchName(cmd.Context(), cwd, worktreecmd.BranchNameOptions{Task: task, TitleFile: titleFile, Worktree: worktree, PluginRoot: pluginRoot})
			return emitJSONOrRender(cmd, jsonOut, false, res, func() {
				if res.OK {
					fmt.Fprintln(cmd.OutOrStdout(), res.Branch)
				}
			}, runErr)
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "Task id")
	cmd.Flags().StringVar(&titleFile, "title-file", "", "File holding the task title (the slug is derived from it)")
	cmd.Flags().BoolVar(&worktree, "worktree", false, "Render the worktree-branch: template")
	cmd.Flags().StringVar(&pluginRoot, "plugin-root", "", "Plugin root for the tracker adapter fallback (default: $CLAUDE_PLUGIN_ROOT)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newWorktreePushedCmd() *cobra.Command {
	var jsonOut bool
	var o worktreecmd.PushedOptions
	cmd := &cobra.Command{
		Use:   "pushed",
		Short: "Report whether a commit is on any branch of the remote (pushed | not_pushed | unverified)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "pushed", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.Pushed(cmd.Context(), cwd, o)
			return emitJSONOrRender(cmd, jsonOut, false, res, func() { worktreecmd.RenderPushed(cmd.OutOrStdout(), res) }, runErr)
		},
	}
	cmd.Flags().StringVar(&o.Commit, "commit", "", "Commit sha to look for on the remote")
	cmd.Flags().StringVar(&o.Branch, "branch", "", "Branch the commit belongs to (named in the message; its upstream remote is used)")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 0, "Bound for each network step (default 20s)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newWorktreeTeamInitCmd() *cobra.Command {
	var jsonOut bool
	var o worktreecmd.TeamInitOptions
	cmd := &cobra.Command{
		Use:   "team-init",
		Short: "Write a standing team's file .lets/teams/<callsign>.md (never overwritten), or propose a free callsign",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "team-init", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.TeamInit(cmd.Context(), cwd, o)
			return emitJSONOrRender(cmd, jsonOut, false, res, func() {
				switch {
				case res.Suggested:
					fmt.Fprintln(cmd.OutOrStdout(), res.Callsign)
				case res.OK:
					fmt.Fprintln(cmd.OutOrStdout(), res.TeamFile)
				default:
					worktreecmd.RenderSteps(cmd.OutOrStdout(), res.Envelope)
				}
			}, runErr)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Callsign, "callsign", "", "Team callsign (snake, frog, ...)")
	f.StringVar(&o.Area, "area", "", "What the team owns, in one line")
	f.StringVar(&o.Worktree, "worktree", "", "Absolute path of the team's worktree")
	f.StringVar(&o.PluginRoot, "plugin-root", "", "Plugin root holding templates/team.md (default: $CLAUDE_PLUGIN_ROOT)")
	f.StringVar(&o.AgentCommand, "agent-command", "", "How the lead is launched (default claude)")
	f.StringVar(&o.OrcaAgent, "orca-agent", "", "Shell command an Orca lead terminal runs (default claude)")
	f.BoolVar(&o.Suggest, "suggest-callsign", false, "Only propose a free callsign; write nothing")
	f.BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newWorktreeSwitchCmd() *cobra.Command {
	var jsonOut bool
	var o worktreecmd.SwitchOptions
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Move a team worktree to another task's branch (--park commits the old one as wip; never stashes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "switch", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			// The session: line goes through the SessionStart hook's own guard: a
			// teammate pane in this worktree never takes the lead's boundary.
			o.Session = os.Getenv("CLAUDE_CODE_SESSION_ID")
			if o.Session != "" {
				o.Guard, _ = sessionGuardFn(gitutil.ProjectRoot(cwd, 2*time.Second), o.Session)
			}
			res, runErr := worktreecmd.Switch(cmd.Context(), cwd, o)
			return emitJSONOrRender(cmd, jsonOut, false, res, func() { worktreecmd.RenderSteps(cmd.OutOrStdout(), res.Envelope) }, runErr)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Task, "task", "", "Task to switch to")
	f.StringVar(&o.Branch, "branch", "", "Its branch (default: the task's local branch, else a new created-shape one)")
	f.StringVar(&o.TitleFile, "title-file", "", "File holding the task title (the new branch's slug)")
	f.BoolVar(&o.Park, "park", false, "Commit the old branch's changes as wip(<id>): park")
	f.StringArrayVar(&o.Include, "include", nil, "A new path to park (repeatable; every untracked or staged-new path needs one)")
	f.DurationVar(&o.FetchTimeout, "fetch-timeout", 0, "Bound for fetching origin/<merge> (default 20s)")
	f.BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newWorktreeParkedCmd() *cobra.Command {
	var jsonOut bool
	var team string
	cmd := &cobra.Command{
		Use:   "parked",
		Short: "List the branches a team parked (owned) and parks that name no team (unowned)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return emitErrorEnvelope(cmd.OutOrStdout(), jsonOut, "parked", &worktreecmd.Error{Code: worktreecmd.ExitFilesystem, Kind: "getwd_failed", Message: err.Error(), Cause: err})
			}
			res, runErr := worktreecmd.Parked(cmd.Context(), cwd, team)
			return emitJSONOrRender(cmd, jsonOut, false, res, func() {
				for _, e := range res.Owned {
					fmt.Fprintf(cmd.OutOrStdout(), "owned   %s %s %s\n", e.Task, e.Branch, e.ParkSha)
				}
				for _, e := range res.Unowned {
					fmt.Fprintf(cmd.OutOrStdout(), "unowned %s %s %s\n", e.Task, e.Branch, e.ParkSha)
				}
			}, runErr)
		},
	}
	cmd.Flags().StringVar(&team, "team", "", "Team callsign")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

// Human-readable rendering lives in cli/internal/worktreecmd/render.go
// (review S-8: domain package owns presentation alongside the envelope
// shape; mirrors updatecmd.PrintReport precedent).
