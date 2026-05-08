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

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

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
	for _, key := range letsconfig.Names() {
		if val := env[key]; val != "" {
			if _, err := fmt.Fprintf(w, "%s=%s\n", key, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// driftCheck returns a "## LETS Notice" block when the installed rules differ
// from the plugin's. Empty string means no notice (versions match or plugin
// unreadable). Wraps drift.Check + drift.Message — single source of truth for
// drift wording shared with `lets init --json` output.
func driftCheck(pluginRulesPath, projectRoot string) string {
	installedPath := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	r := drift.Check(pluginRulesPath, installedPath)
	msg := drift.Message(r)
	if msg == "" {
		return ""
	}
	return "## LETS Notice\n\n" + msg
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
