package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return fmt.Errorf("not in a git repository - run `git init` first")
			}
			if initcmd.DetectInsideWorktree() {
				return fmt.Errorf("'lets init' must run from main repo, not a worktree")
			}

			pluginRoot, err := initcmd.DetectPluginRoot(flagPluginRoot)
			if err != nil {
				return fmt.Errorf("%w\n\nRun /lets:init from inside Claude Code (after `/plugin install lets@lets-workflow`).\nFor advanced use, pass --plugin-root=<path-to-plugins/lets>", err)
			}

			// Deprecation: --github maps to --pr-flow=github
			if flagGithub {
				if flagPRFlow != "" && flagPRFlow != "github" {
					return fmt.Errorf("--github conflicts with --pr-flow=%s", flagPRFlow)
				}
				flagPRFlow = "github"
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --github is deprecated, use --pr-flow=github")
			}

			prefs := initcmd.Prefs{
				Language:    flagLanguage,
				MergeBranch: flagMergeBranch,
				PRFlow:      flagPRFlow,
				SkipBeads:   flagSkipBeads,
			}
			// Defaults if caller omitted them (slash command should always pass,
			// but be lenient for direct shell invocation).
			if prefs.Language == "" {
				prefs.Language = "English"
			}
			if prefs.MergeBranch == "" {
				prefs.MergeBranch = "main"
			}
			if prefs.PRFlow == "" {
				prefs.PRFlow = "local"
			}

			steps, err := initcmd.Run(ctx, prefs, projectRoot, pluginRoot)
			initcmd.PrintSteps(cmd.OutOrStdout(), steps) // print even on partial failure
			return err
		},
	}
	cmd.Flags().StringVar(&flagLanguage, "language", "", "Response language (English/Ukrainian/Italian/etc)")
	cmd.Flags().StringVar(&flagMergeBranch, "merge-branch", "", "Target branch for merges (default: main)")
	cmd.Flags().StringVar(&flagPRFlow, "pr-flow", "", "PR flow: local | github | bitbucket")
	cmd.Flags().BoolVar(&flagGithub, "github", false, "(deprecated) alias for --pr-flow=github")
	cmd.Flags().BoolVar(&flagSkipBeads, "skip-beads", false, "Skip beads initialization")
	cmd.Flags().StringVar(&flagPluginRoot, "plugin-root", "", "Plugin install dir (else $CLAUDE_PLUGIN_ROOT, required)")
	return cmd
}
