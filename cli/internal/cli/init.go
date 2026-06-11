package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

// NewInitCmd builds `lets init`.
//
// Internal command - intended to be invoked by the `/lets:init` slash command
// (which captures user prefs via AskUserQuestion and passes them as flags
// alongside --plugin-root=${CLAUDE_PLUGIN_ROOT}). Not designed for direct
// terminal use: there is no TUI prompt and required flags must be provided.
//
// Direct shell invocation works for advanced use (CI / dev override) when
// the caller knows where the plugin source lives.
func NewInitCmd() *cobra.Command {
	var (
		flagLanguage    string
		flagMergeBranch string
		flagPRFlow      string
		flagLauncher    string
		flagGithub      bool
		flagSkipBeads   bool
		flagRulesScope  string
		flagUser        bool
		flagPluginRoot  string
		flagJSON        bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Apply LETS project setup (internal - invoke via /lets:init)",
		Long: `Sets up .lets/ structure, configures .claude/settings.json (statusLine
provenance markers), copies plugin rules to .claude/rules/lets-rules.md, and
runs bd init.

With --user: user-scope install instead - copies plugin rules to
~/.claude/rules/lets-rules.md (global, all projects) and writes user-level
defaults (LETS_LANGUAGE, LETS_LAUNCHER) to ~/.lets/.env. No project changes,
works from any directory. A customized/newer global rules file is never
overwritten (re-run reports it instead).

Migrates existing projects (legacy .lets/statusline.sh, bash-wrapped
settings.json statusLine, .lets/config.yaml -> .lets/.env).

This is an internal subcommand. The supported entry point is the /lets:init
slash command, which captures user preferences in Claude Code and shells
out with --plugin-root=${CLAUDE_PLUGIN_ROOT} plus the chosen flags.`,
		// SilenceUsage prevents cobra from appending the Usage block to stdout
		// after RunE error — would corrupt the JSON envelope. SilenceErrors
		// prevents cobra from prefixing "Error: ..." to stderr (we set
		// result.Error in the envelope; main.go can still print to stderr).
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// emit always returns the cmd Run error (preserves exit code) but
			// in --json mode writes a JSON envelope to stdout regardless of
			// success/failure. In text mode, prints steps via PrintSteps.
			emit := func(result initcmd.Result, err error) error {
				if err != nil {
					result.OK = false
					result.Error = err.Error()
				} else {
					result.OK = true
				}
				if flagJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					if encErr := enc.Encode(result); encErr != nil {
						// Almost impossible (struct is JSON-safe). Fall through to
						// text path so user sees something.
						initcmd.PrintSteps(cmd.OutOrStdout(), result.Steps)
					}
					return err
				}
				initcmd.PrintSteps(cmd.OutOrStdout(), result.Steps)
				return err
			}

			// --user: user-scope install (global rules + ~/.lets/.env). Works
			// from ANY directory - no git project, no worktree guard (those are
			// project-scope concerns).
			if flagUser {
				if flagMergeBranch != "" || flagPRFlow != "" || flagGithub || flagSkipBeads || flagRulesScope != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: --merge-branch/--pr-flow/--github/--skip-beads/--rules-scope are project-scope flags - ignored with --user")
				}
				home, herr := os.UserHomeDir()
				if herr != nil {
					return emit(initcmd.NewResult("", ""), fmt.Errorf("cannot resolve home directory: %w", herr))
				}
				pluginRoot, perr := initcmd.DetectPluginRoot(flagPluginRoot)
				if perr != nil {
					return emit(initcmd.NewResult(home, ""), fmt.Errorf("%w\n\nRun /lets:init from inside Claude Code, or pass --plugin-root=<path-to-plugins/lets>", perr))
				}
				// Language/Launcher pass through raw: empty means "preserve
				// existing ~/.lets/.env value or canonical default" (handled
				// inside RegenerateUserEnv) - do NOT apply the project path's
				// default-fill below.
				result, runErr := initcmd.RunUser(initcmd.UserOptions{
					Language:   flagLanguage,
					Launcher:   flagLauncher,
					HomeDir:    home,
					PluginRoot: pluginRoot,
				})
				return emit(result, runErr)
			}

			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return emit(initcmd.NewResult("", ""), fmt.Errorf("not in a git repository - run `git init` first"))
			}
			if initcmd.DetectInsideWorktree() {
				return emit(initcmd.NewResult(projectRoot, ""), fmt.Errorf("'lets init' must run from main repo, not a worktree"))
			}

			pluginRoot, err := initcmd.DetectPluginRoot(flagPluginRoot)
			if err != nil {
				return emit(initcmd.NewResult(projectRoot, ""), fmt.Errorf("%w\n\nRun /lets:init from inside Claude Code (after `/plugin install lets@lets-workflow`).\nFor advanced use, pass --plugin-root=<path-to-plugins/lets>", err))
			}

			// Deprecation: --github maps to --pr-flow=github
			if flagGithub {
				if flagPRFlow != "" && flagPRFlow != "github" {
					return emit(initcmd.NewResult(projectRoot, pluginRoot), fmt.Errorf("--github conflicts with --pr-flow=%s", flagPRFlow))
				}
				flagPRFlow = "github"
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --github is deprecated, use --pr-flow=github")
			}

			if flagRulesScope != "" && flagRulesScope != "project" && flagRulesScope != "user" {
				return emit(initcmd.NewResult(projectRoot, pluginRoot), fmt.Errorf("--rules-scope must be 'project' or 'user', got %q", flagRulesScope))
			}

			// Pass raw cobra flag values through to Prefs. Empty string means
			// "user did not pass --<key>" — initcmd.Run gates fresh-creation on
			// this (errors if .env doesn't exist and any of language/merge-branch/
			// pr-flow is empty). Existing-.env paths use empty as a signal to
			// preserve current values via RegenerateEnv's mergePrefs.
			//
			// Tracker has no CLI flag; always filled from canonical defaults.
			// mergePrefs preserves a user-customized LETS_TRACKER over this default.
			// Launcher/RulesScope pass through RAW (no default-fill): mergePrefs's
			// pick (flag > existing .env > default) both preserves a customized
			// value on regen AND honors an explicit flag - the LETS_LAUNCHER
			// preservation fix (lets-wug9k). They aren't in the fresh-init gate,
			// so empty is safe (mergePrefs falls back to the canonical default).
			prefs := initcmd.Prefs{
				Language:    flagLanguage,
				MergeBranch: flagMergeBranch,
				PRFlow:      flagPRFlow,
				Tracker:     letsconfig.Defaults()["LETS_TRACKER"],
				Launcher:    flagLauncher,
				RulesScope:  flagRulesScope,
				SkipBeads:   flagSkipBeads,
			}

			result, runErr := initcmd.Run(ctx, prefs, projectRoot, pluginRoot)
			return emit(result, runErr)
		},
	}
	cmd.Flags().StringVar(&flagLanguage, "language", "", "Response language (English/Ukrainian/Italian/etc)")
	cmd.Flags().StringVar(&flagMergeBranch, "merge-branch", "", "Target branch for merges (default: main)")
	cmd.Flags().StringVar(&flagPRFlow, "pr-flow", "", "PR flow: local | github | bitbucket")
	cmd.Flags().StringVar(&flagLauncher, "launcher", "", "Worktree launcher: terminal | cmux (default terminal)")
	cmd.Flags().BoolVar(&flagGithub, "github", false, "(deprecated) alias for --pr-flow=github")
	cmd.Flags().BoolVar(&flagSkipBeads, "skip-beads", false, "Skip beads initialization")
	cmd.Flags().StringVar(&flagRulesScope, "rules-scope", "", "Rules sourcing for this project: project (own copy) | user (rely on ~/.claude/rules)")
	cmd.Flags().BoolVar(&flagUser, "user", false, "User-scope install: global rules to ~/.claude/rules + defaults to ~/.lets/.env (no project changes)")
	cmd.Flags().StringVar(&flagPluginRoot, "plugin-root", "", "Plugin install dir (else $CLAUDE_PLUGIN_ROOT, required)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Output machine-readable JSON to stdout (single object, schema_version=1)")
	return cmd
}
