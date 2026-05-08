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
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
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

// driftCheck returns a "## LETS Notice" block if installed rules are missing
// or outdated vs plugin. Empty string otherwise.
//
// Cases:
//   - plugin rules path empty / unreadable / no version → silent (nothing to compare)
//   - installed rules file absent → "rules not installed"
//   - installed file present but no parseable version → "rules version unknown"
//   - installed.version < plugin.version → "rules outdated"
//   - installed.version >= plugin.version (incl. future drift) → silent
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
	if semver.Compare("v"+installedVer, "v"+pluginVer) < 0 {
		return fmt.Sprintf("## LETS Notice\n\nWorkflow rules outdated (installed v%s < plugin v%s). Run `/lets:init` to update.", installedVer, pluginVer)
	}
	return ""
}

// DetectProjectRoot returns the git toplevel for the current working
// directory. Falls back to os.Getwd() if git is unavailable or the cwd is
// not in a git repo. Returns empty string if both fail.
func DetectProjectRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
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
