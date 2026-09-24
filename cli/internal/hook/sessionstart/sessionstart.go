// Package sessionstart implements the SessionStart + PreCompact hook output:
// it writes the LETS Config block (and an optional drift notice) to the
// provided writer. Workflow rules themselves live in the project's
// .claude/rules/lets-rules.md (uncapped Claude Code project-instructions
// channel) - they are NOT emitted by the hook (Phase 4b: lets-q9bx7 fix for
// the 10K hook output cap that silently truncated the 17KB rules-context.md).
//
// Per-key usage docs (the "Local Config" explainer) are embedded from
// local_config_explainer.md and emitted right after the values block, so
// the values arrive self-documenting and the explainer survives compaction
// the same way the values do (lets-q9bx7 scope extension 2026-05-10).
//
// This package owns no I/O policy: callers supply the (plugin) rules file
// path - used for drift comparison vs the installed rules - the project root,
// the user home dir (user-scope rules presence + ~/.lets/.env defaults; lets-wug9k),
// and the output writer. Detection helpers (DetectProjectRoot) are exposed
// for cobra wiring.
package sessionstart

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/rulescache"
)

//go:embed local_config_explainer.md
var localConfigExplainer string

// Run writes the SessionStart hook output to w:
//  1. Optional ## LETS Notice block (project rules drift, see driftCheck,
//     followed by every non-empty extraNotices entry - the cli layer's self-heal
//     and global rules cache sync)
//  2. Blank line
//  3. ## LETS Config block (LETS_PROJECT_ROOT + whitelisted keys from the
//     merged project-over-user env, see mergedEnv)
//  4. Blank line
//  5. ### About these values explainer (embedded from local_config_explainer.md)
//
// rulesPath is the plugin's rules/lets-rules.md (for version compare against
// the installed project copy).
//
// homeDir is the user's home directory for user-scope lookups
// (~/.claude/rules/lets-rules.md presence, ~/.lets/.env config defaults);
// empty string = "no user scope" (resolution failed or tests opting out) and
// degrades to the project-only behavior.
//
// projectRoot empty -> only a non-empty Notice (the global rules cache);
// nothing else.
//
// extraNotices are messages another layer needs surfaced in the same Notice block
// (the SessionStart self-heal's adopt outcome, the global rules cache sync);
// PreCompact passes nil.
func Run(w io.Writer, rulesPath, projectRoot, homeDir string, extraNotices []string) error {
	if projectRoot == "" {
		return writeNotice(w, extraNotices)
	}

	// Compute the merged env BEFORE the notice so driftCheck can read the
	// resolved LETS_RULES_SCOPE. Output order is unchanged - the notice is still
	// emitted first, the Config block second.
	env := letsconfig.ResolvedEnv(projectRoot, homeDir, func(r string) string { return gitutil.DefaultBranch(r, time.Second) })

	if err := writeNotice(w, append([]string{driftCheck(rulesPath, projectRoot, homeDir, env["LETS_RULES_SCOPE"])}, extraNotices...)); err != nil {
		return err
	}

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
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, localConfigExplainer); err != nil {
		return err
	}
	return nil
}

// writeNotice writes the ## LETS Notice block for every non-empty msg; nothing
// when all are empty. The trailing surface-this line is hook-only (it tells the
// orchestrator to relay the notice even when a big slash command like
// /lets:start is running); it is NOT part of drift.Message, so `lets init
// --json` output stays clean.
func writeNotice(w io.Writer, msgs []string) error {
	var keep []string
	for _, m := range msgs {
		if m != "" {
			keep = append(keep, m)
		}
	}
	if len(keep) == 0 {
		return nil
	}
	notice := "## LETS Notice\n\n" + strings.Join(keep, "\n\n") + "\n\n→ Surface this to the user at the start of your next response (one line), then continue - do not skip it."
	if _, err := fmt.Fprintln(w, notice); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

// driftCheck returns the project rules drift message. The global
// ~/.claude/rules copy is not drift-checked here: rulescache owns it and
// reports through its own Notice (lets-tg008).
func driftCheck(pluginRulesPath, projectRoot, homeDir, rulesScope string) string {
	r := drift.Check(pluginRulesPath, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"))
	if r.State == drift.StateMissing && homeDir != "" {
		if _, err := os.Stat(rulescache.DstPath(homeDir)); err == nil || rulesScope == "user" {
			return "" // user scope covers this project; the cache sync speaks for that file
		}
	}
	return drift.Message(r)
}

// DetectProjectRoot returns the git toplevel for the current working
// directory, or empty string if git is unavailable or cwd is not in a repo.
//
// Bash parity (matches old session-start.sh `git rev-parse --show-toplevel
// 2>/dev/null` semantics): no os.Getwd() fallback. Empty result triggers
// Run() to emit no Config (only a non-empty Notice), which is the correct
// behavior for "user opened Claude Code outside any project" - downstream
// commands assume the value is a real project root and would otherwise mutate
// the user's $HOME or cwd.
//
// 2-second timeout because the hook fires on every SessionStart and a
// hanging git would noticeably delay Claude Code startup.
func DetectProjectRoot() string {
	return gitutil.ProjectRoot("", 2*time.Second)
}
