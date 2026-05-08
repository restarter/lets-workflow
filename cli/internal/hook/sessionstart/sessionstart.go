// Package sessionstart implements the SessionStart + PreCompact hook output:
// it writes ONLY the LETS Config block (and an optional drift notice) to the
// provided writer. Workflow rules themselves live in the project's
// .claude/rules/lets-rules.md (uncapped Claude Code project-instructions
// channel) - they are NOT emitted by the hook (Phase 4b: lets-q9bx7 fix for
// the 10K hook output cap that silently truncated the 17KB rules-context.md).
//
// This package owns no I/O policy: callers supply the (plugin) rules file
// path - used for drift comparison vs the installed rules - the project root,
// and the output writer. Detection helpers (DetectProjectRoot) are exposed
// for cobra wiring.
package sessionstart

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// configKeys is the whitelist of LETS_* keys read from .env, in the fixed
// order they're emitted in the LETS Config block.
var configKeys = []string{
	"LETS_LANGUAGE",
	"LETS_MERGE_BRANCH",
	"LETS_PR_FLOW",
	"LETS_TRACKER",
}

// Run writes the SessionStart hook output to w:
//  1. Optional ## LETS Notice block (drift check: rules missing or outdated)
//  2. Blank line
//  3. ## LETS Config block (LETS_PROJECT_ROOT + .env whitelisted keys)
//
// rulesPath is the plugin's rules/lets-rules.md (for version compare against
// the installed copy at <projectRoot>/.claude/rules/lets-rules.md).
//
// projectRoot empty -> emit nothing (matches bash behavior when git rev-parse
// returns nothing).
func Run(w io.Writer, rulesPath, projectRoot string) error {
	if projectRoot == "" {
		return nil
	}

	if notice := driftCheck(rulesPath, projectRoot); notice != "" {
		if _, err := fmt.Fprintln(w, notice); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	env, _ := readEnvFile(filepath.Join(projectRoot, ".lets", ".env"))

	if _, err := fmt.Fprintln(w, "## LETS Config"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "LETS_PROJECT_ROOT=%s\n", projectRoot); err != nil {
		return err
	}
	for _, key := range configKeys {
		if val := env[key]; val != "" {
			if _, err := fmt.Fprintf(w, "%s=%s\n", key, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// driftCheck returns a "## LETS Notice" block if installed rules are missing,
// outdated, or AHEAD of plugin (which would indicate tampering or a stale
// binary). Empty string otherwise.
//
// Cases:
//   - plugin rules path empty / unreadable / no version → silent (nothing to compare)
//   - installed rules file absent → "rules not installed"
//   - installed file present but no parseable version → "rules version unknown"
//   - installed.version < plugin.version → "rules outdated"
//   - installed.version > plugin.version → "rules ahead of plugin" (tampering or stale binary signal)
//   - installed.version == plugin.version → silent
func driftCheck(pluginRulesPath, projectRoot string) string {
	pluginVer := frontmatter.ReadVersion(pluginRulesPath)
	if pluginVer == "" {
		return ""
	}
	installedPath := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		return "## LETS Notice\n\nWorkflow rules not installed in `.claude/rules/lets-rules.md`. Run `/lets:init` to install."
	}
	installedVer := frontmatter.ReadVersion(installedPath)
	if installedVer == "" {
		return "## LETS Notice\n\nWorkflow rules version unknown - rules may be outdated. Run `/lets:init` to refresh."
	}
	switch semver.Compare("v"+installedVer, "v"+pluginVer) {
	case -1:
		return fmt.Sprintf("## LETS Notice\n\nWorkflow rules outdated (installed v%s < plugin v%s). Run `/lets:init` to update.", installedVer, pluginVer)
	case 1:
		// Installed > plugin: either someone hand-edited the rules file
		// (possibly to bypass the drift check while neutering rules content)
		// or the lets binary is older than the rules. Either way, surface it
		// instead of silently honoring the installed version.
		return fmt.Sprintf("## LETS Notice\n\nWorkflow rules AHEAD of plugin (installed v%s > plugin v%s). Verify the rules file integrity (rules tampering signal) or upgrade the lets binary. Run `/lets:init` to reset to plugin version.", installedVer, pluginVer)
	}
	return ""
}

// DetectProjectRoot returns the git toplevel for the current working
// directory, or empty string if git is unavailable or cwd is not in a repo.
//
// Bash parity (matches old session-start.sh `git rev-parse --show-toplevel
// 2>/dev/null` semantics): no os.Getwd() fallback. Empty result triggers
// Run() to emit nothing, which is the correct behavior for "user opened
// Claude Code outside any project" - downstream commands assume the value
// is a real project root and would otherwise mutate the user's $HOME or cwd.
//
// 2-second timeout because the hook fires on every SessionStart and a
// hanging git would noticeably delay Claude Code startup.
func DetectProjectRoot() string {
	return gitutil.ProjectRoot("", 2*time.Second)
}

// readEnvFile parses the .env file at path. A missing file is not an error -
// returns empty map.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return map[string]string{}, err
	}
	defer f.Close()
	return envfile.Parse(f)
}
