package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/updatecmd"
)

// NewUpdateCmd builds `lets update`.
//
// Internal command - invoked by the `/lets:update` slash command with
// --plugin-root=${CLAUDE_PLUGIN_ROOT}. Syncs the version-pinned artifacts
// (.lets/.env header, .claude/rules/lets-rules.md) and reports version status
// for the lets binary and the Claude Code plugin (which it cannot self-update).
//
// Distinct from `lets init`: update never prompts for config, touches
// settings.json, or runs beads - it only syncs what a new release changes.
func NewUpdateCmd() *cobra.Command {
	var (
		flagPluginRoot   string
		flagJSON         bool
		flagOffline      bool
		flagRefreshCache bool
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Sync project with the current LETS release (internal - invoke via /lets:update)",
		Long: `Checks the drift-able LETS artifacts (four core + two optional):

  .lets/.env                    regenerated if LETS_ENV_VERSION is stale (user values preserved)
  .claude/rules/lets-rules.md   re-copied from the plugin if outdated/missing
  lets binary                   version compared to the latest GitHub release (reports only)
  Claude Code plugin            version compared to the latest release (reports only)
  ~/.claude/rules/lets-rules.md user-scope global rules - row appears only when the file
                                exists; synced like project rules EXCEPT a newer/customized
                                (ahead) copy is reported, never overwritten
  .claude/rules/tracker-<n>.md  the active tracker adapter (row appears when LETS_TRACKER
                                names a shipped adapter, or a user-authored one with an
                                installed copy - reported delegated) - synced like project
                                rules; on a tracker switch (edited .env) the deactivated
                                adapter file is removed and the board profile scaffolded

This is an internal subcommand. The supported entry point is the /lets:update
slash command, which shells out with --plugin-root=${CLAUDE_PLUGIN_ROOT}.`,
		// SilenceUsage stops cobra appending the Usage block after a RunE error
		// (would corrupt the JSON envelope). SilenceErrors stops the "Error: ..."
		// prefix on stderr - we carry the error in the envelope; main.go prints once.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			emit := func(result updatecmd.Result, err error) error {
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
						updatecmd.PrintReport(cmd.OutOrStdout(), result)
					}
					return err
				}
				updatecmd.PrintReport(cmd.OutOrStdout(), result)
				return err
			}

			projectRoot := initcmd.DetectProjectRoot()
			if projectRoot == "" {
				return emit(updatecmd.NewResult("", ""), fmt.Errorf("not in a git repository"))
			}
			if initcmd.DetectInsideWorktree() {
				return emit(updatecmd.NewResult(projectRoot, ""), fmt.Errorf("'lets update' must run from the main repo, not a worktree (.claude/ isn't shared into worktrees)"))
			}
			pluginRoot, err := initcmd.DetectPluginRoot(flagPluginRoot)
			if err != nil {
				return emit(updatecmd.NewResult(projectRoot, ""), fmt.Errorf("%w\n\nRun /lets:update from inside Claude Code, or pass --plugin-root=<path-to-plugins/lets>", err))
			}

			if flagRefreshCache && flagOffline {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --refresh-cache has no effect with --offline")
			}
			home, _ := os.UserHomeDir() // "" on failure -> user-rules artifact skipped
			opts := updatecmd.Options{HomeDir: home}
			if !flagOffline {
				cacheDir := filepath.Join(projectRoot, ".lets", "cache")
				refresh := flagRefreshCache
				opts.LatestFn = func(c context.Context) (updatecmd.LatestInfo, error) {
					return updatecmd.FetchLatest(c, cacheDir, refresh)
				}
			}
			result, runErr := updatecmd.Run(ctx, opts, projectRoot, pluginRoot)
			return emit(result, runErr)
		},
	}
	cmd.Flags().StringVar(&flagPluginRoot, "plugin-root", "", "Plugin install dir (else $CLAUDE_PLUGIN_ROOT, required)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Output machine-readable JSON to stdout (single object, schema_version=1)")
	cmd.Flags().BoolVar(&flagOffline, "offline", false, "Skip the GitHub latest-release check (binary/plugin reported as unknown)")
	cmd.Flags().BoolVar(&flagRefreshCache, "refresh-cache", false, "Bypass the cached latest-release lookup and hit GitHub now")
	return cmd
}
