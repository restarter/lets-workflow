package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

// NewInitCmd builds `lets init`.
//
// Default: TUI flow (welcome -> form-with-confirm -> apply with spinner -> completion + step list).
// --non-interactive: skip TUI, use flags directly, plain text output.
// --plugin-root or $CLAUDE_PLUGIN_ROOT must point to plugin install dir.
func NewInitCmd() *cobra.Command {
	var (
		flagLanguage    string
		flagMergeBranch string
		flagPRFlow      string
		flagGithub      bool
		flagSkipBeads   bool
		flagQuiet       bool
		flagNoTUI       bool
		flagPluginRoot  string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize LETS in current project (interactive TUI by default)",
		Long: `Sets up .lets/ structure, configures .claude/settings.json, copies plugin rules
to .claude/rules/lets-rules.md, and initializes beads.

Interactive: launches a TUI form (huh) for language/merge-branch/pr-flow.
Non-interactive: pass --language --merge-branch --pr-flow + --non-interactive.

Migrates existing projects (legacy .lets/statusline.sh, bash-wrapped
settings.json statusLine, .lets/config.yaml -> .lets/.env).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return fmt.Errorf("not in a git repository")
			}
			if initcmd.DetectInsideWorktree() {
				return fmt.Errorf("'lets init' must run from main repo, not a worktree.\nSwitch: cd $(git rev-parse --git-common-dir | xargs dirname)")
			}

			pluginRoot, err := initcmd.DetectPluginRoot(flagPluginRoot)
			if err != nil {
				return err
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
			opts := initcmd.RunOptions{
				Quiet:          flagQuiet,
				NonInteractive: flagNoTUI,
			}

			isTTY := term.IsTerminal(int(os.Stdin.Fd()))
			interactive := !flagNoTUI && isTTY && !flagQuiet

			if interactive {
				fmt.Fprintln(cmd.OutOrStdout(), initcmd.RenderWelcome(projectRoot, pluginRoot, version.Version))
				fmt.Fprintln(cmd.OutOrStdout())

				// PromptPrefs uses huh: collects lang / merge-branch / pr-flow
				// AND ends with a "Confirm: Apply / Cancel" field. If user picks
				// Cancel, returns ErrUserAborted. (No separate ConfirmApply step,
				// no separate plan preview - completion screen shows what was done.)
				prefs, err = initcmd.PromptPrefs(prefs)
				if errors.Is(err, initcmd.ErrUserAborted) {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
				if err != nil {
					return err
				}

				steps, err := initcmd.RunWithSpinner(ctx, prefs, opts, projectRoot, pluginRoot)
				if err != nil {
					return err
				}

				beadsOK := false
				for _, s := range steps {
					if s.Status == initcmd.StepOK && s.Message == "beads initialized" {
						beadsOK = true
					}
				}
				fmt.Fprintln(cmd.OutOrStdout())
				initcmd.PrintSteps(cmd.OutOrStdout(), steps)
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), initcmd.RenderCompletion(projectRoot, prefs, beadsOK))
				return nil
			}

			// Non-interactive (no TTY, --non-interactive, or --quiet)
			if prefs.Language == "" {
				prefs.Language = "English"
			}
			if prefs.MergeBranch == "" {
				prefs.MergeBranch = "main"
			}
			if prefs.PRFlow == "" {
				prefs.PRFlow = "local"
			}
			steps, err := initcmd.Run(ctx, prefs, opts, projectRoot, pluginRoot)
			if err != nil {
				return err
			}
			initcmd.PrintSteps(cmd.OutOrStdout(), steps)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagLanguage, "language", "", "Response language (English/Ukrainian/Italian/etc)")
	cmd.Flags().StringVar(&flagMergeBranch, "merge-branch", "", "Target branch for merges (default: main)")
	cmd.Flags().StringVar(&flagPRFlow, "pr-flow", "", "PR flow: local | github | bitbucket")
	cmd.Flags().BoolVar(&flagGithub, "github", false, "(deprecated) alias for --pr-flow=github")
	cmd.Flags().BoolVar(&flagSkipBeads, "skip-beads", false, "Skip beads initialization")
	cmd.Flags().BoolVar(&flagQuiet, "quiet", false, "Suppress informational output")
	cmd.Flags().BoolVar(&flagNoTUI, "non-interactive", false, "Skip TUI, use flags only")
	cmd.Flags().StringVar(&flagPluginRoot, "plugin-root", "", "Plugin install dir (else $CLAUDE_PLUGIN_ROOT)")
	return cmd
}
