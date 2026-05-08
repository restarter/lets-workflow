package cli

import (
	"context"
	"encoding/json"
	"fmt"

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
		flagGithub      bool
		flagSkipBeads   bool
		flagPluginRoot  string
		flagJSON        bool
		flagForceEnv    bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Apply LETS project setup (internal - invoke via /lets:init)",
		Long: `Sets up .lets/ structure, configures .claude/settings.json (statusLine
provenance markers), copies plugin rules to .claude/rules/lets-rules.md, and
runs bd init.

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

			// Defaults flow from letsconfig.Defaults() — single source of truth.
			// Direct shell invocation may omit flags; slash command always passes them.
			defaults := letsconfig.Defaults()
			prefs := initcmd.Prefs{
				Language:    flagOrDefault(flagLanguage, defaults["LETS_LANGUAGE"]),
				MergeBranch: flagOrDefault(flagMergeBranch, defaults["LETS_MERGE_BRANCH"]),
				PRFlow:      flagOrDefault(flagPRFlow, defaults["LETS_PR_FLOW"]),
				Tracker:     defaults["LETS_TRACKER"], // no --tracker flag yet; always default
				SkipBeads:   flagSkipBeads,
				ForceEnv:    flagForceEnv,
			}

			result, runErr := initcmd.Run(ctx, prefs, projectRoot, pluginRoot)
			return emit(result, runErr)
		},
	}
	cmd.Flags().StringVar(&flagLanguage, "language", "", "Response language (English/Ukrainian/Italian/etc)")
	cmd.Flags().StringVar(&flagMergeBranch, "merge-branch", "", "Target branch for merges (default: main)")
	cmd.Flags().StringVar(&flagPRFlow, "pr-flow", "", "PR flow: local | github | bitbucket")
	cmd.Flags().BoolVar(&flagGithub, "github", false, "(deprecated) alias for --pr-flow=github")
	cmd.Flags().BoolVar(&flagSkipBeads, "skip-beads", false, "Skip beads initialization")
	cmd.Flags().StringVar(&flagPluginRoot, "plugin-root", "", "Plugin install dir (else $CLAUDE_PLUGIN_ROOT, required)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Output machine-readable JSON to stdout (single object, schema_version=1)")
	cmd.Flags().BoolVar(&flagForceEnv, "force-env", false, "Surgically update existing .env (LETS_* keys only, preserves comments/foreign keys, writes single .env.bak)")
	return cmd
}

// flagOrDefault returns flagVal if non-empty, else def. Used to layer
// letsconfig.Defaults() under cobra string flags.
func flagOrDefault(flagVal, def string) string {
	if flagVal == "" {
		return def
	}
	return flagVal
}
