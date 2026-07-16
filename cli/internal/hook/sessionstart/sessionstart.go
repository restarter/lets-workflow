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
// the user home dir (user-scope rules + ~/.lets/.env defaults; lets-wug9k),
// and the output writer. Detection helpers (DetectProjectRoot) are exposed
// for cobra wiring.
package sessionstart

import (
	_ "embed"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

//go:embed local_config_explainer.md
var localConfigExplainer string

// Run writes the SessionStart hook output to w:
//  1. Optional ## LETS Notice block (scope-aware drift check, see driftCheck)
//  2. Blank line
//  3. ## LETS Config block (LETS_PROJECT_ROOT + whitelisted keys from the
//     merged project-over-user env, see mergedEnv)
//  4. Blank line
//  5. ### About these values explainer (embedded from local_config_explainer.md)
//
// rulesPath is the plugin's rules/lets-rules.md (for version compare against
// the installed copies).
//
// homeDir is the user's home directory for user-scope lookups
// (~/.claude/rules/lets-rules.md drift check, ~/.lets/.env config defaults);
// empty string = "no user scope" (resolution failed or tests opting out) and
// degrades to the project-only behavior.
//
// projectRoot empty -> emit nothing (matches bash behavior when git rev-parse
// returns nothing). User scope alone does not create output in non-git dirs.
func Run(w io.Writer, rulesPath, projectRoot, homeDir string) error {
	if projectRoot == "" {
		return nil
	}

	// Compute the merged env BEFORE the notice so driftCheck can read the
	// resolved LETS_RULES_SCOPE. Output order is unchanged - the notice is still
	// emitted first, the Config block second.
	env := mergedEnv(projectRoot, homeDir)

	if notice := driftCheck(rulesPath, projectRoot, homeDir, env["LETS_RULES_SCOPE"]); notice != "" {
		if _, err := fmt.Fprintln(w, notice); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
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

// driftCheck returns a "## LETS Notice" block when rules drift requires user
// action, considering BOTH installed scopes:
//
//   - project rules present (any non-missing state) -> existing single-scope
//     behavior verbatim: project drift wins, global state is irrelevant
//     (project copy overrides the global one in Claude Code's loading order).
//   - project rules MISSING + global present and current -> no notice (the
//     user-scope install covers this project; nagging /lets:init here is the
//     exact noise lets-wug9k removes).
//   - project rules MISSING + global present but drifted -> user-scope notice
//     (MessageUser wording names the global path + remediation).
//   - both missing + LETS_RULES_SCOPE=user -> the project deliberately relies
//     on the global copy which is now gone; point at `lets init --user` (the
//     generic /lets:init nag would install a project copy against the choice).
//   - both missing (scope unset/project) -> the existing /lets:init nag.
//
// rulesScope comes from the MERGED env, so a hand-added LETS_RULES_SCOPE in
// ~/.lets/.env also engages the guard (documented side effect). homeDir == ""
// (no user scope / resolution failed) preserves pre-user-scope behavior exactly.
//
// Wraps drift.Check + drift.Message/MessageUser — single source of truth for
// drift wording shared with `lets init --json` output. The trailing
// surface-this line is hook-only (it tells the orchestrator to relay the
// notice even when a big slash command like /lets:start is running); it is NOT
// part of drift.Message, so `lets init --json` output stays clean.
func driftCheck(pluginRulesPath, projectRoot, homeDir, rulesScope string) string {
	installedPath := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	r := drift.Check(pluginRulesPath, installedPath)

	msg := ""
	if r.State == drift.StateMissing && homeDir != "" {
		userPath := filepath.Join(homeDir, ".claude", "rules", "lets-rules.md")
		ur := drift.Check(pluginRulesPath, userPath)
		switch {
		case ur.State == drift.StateMissing && rulesScope == "user":
			// The project opted into the global copy (scope=user) but it's gone.
			// Point at the global installer, not /lets:init (which would install
			// a project copy against the persisted choice).
			msg = "LETS_RULES_SCOPE=user but the global rules copy `~/.claude/rules/lets-rules.md` is missing. Run `lets init --user` to restore it (or set LETS_RULES_SCOPE=project and run `/lets:init`)."
		case ur.State == drift.StateMissing:
			msg = drift.Message(r) // both missing, no scope opt-in: classic nag
		case ur.Detected():
			msg = drift.MessageUser(ur) // global present but drifted
		default:
			// global equal (or plugin unreadable): covered, stay quiet
		}
	} else {
		msg = drift.Message(r)
	}

	if msg == "" {
		return ""
	}
	return "## LETS Notice\n\n" + msg + "\n\n→ Surface this to the user at the start of your next response (one line), then continue - do not skip it."
}

// mergedEnv resolves LETS_* config for injection: letsconfig.MergedEnv does the
// project-over-user overlay; this adds the hook-only LETS_MERGE_BRANCH fallback.
// Whitelist filtering stays at emit time in Run, so foreign keys in either
// file are consistently dropped from injection.
//
// LETS_MERGE_BRANCH git-fallback: when neither file supplies it, derive from
// the repo's origin default branch (single git spawn, 1s timeout), else the
// literal "main" - matching the model-side fallback in the explainer. Only
// fires when the key is absent, so initialized projects (whose .env always
// carries the key - RegenerateEnv restores hand-deleted keys) pay no extra
// git call. Uninitialized repos DO pay one spawn per hook fire (SessionStart
// AND PreCompact), bounded by the 1s timeout.
func mergedEnv(projectRoot, homeDir string) map[string]string {
	merged := letsconfig.MergedEnv(projectRoot, homeDir)
	if merged["LETS_MERGE_BRANCH"] == "" {
		if b := gitutil.DefaultBranch(projectRoot, time.Second); b != "" {
			// Branch names are attacker-influenced in cloned repos, and this is
			// the ONE .env-class value that does not pass through envfile.Parse -
			// apply the same length cap before injection. Newlines/spaces are
			// impossible in ref names (git rejects them); bloat is the residual
			// vector.
			if len(b) > envfile.MaxValueLen {
				b = b[:envfile.MaxValueLen]
			}
			merged["LETS_MERGE_BRANCH"] = b
		} else {
			merged["LETS_MERGE_BRANCH"] = "main"
		}
	}
	return merged
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
