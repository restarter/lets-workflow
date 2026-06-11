// Package drift provides the rules-version drift state machine used by
// `lets init` (Step 8) and the SessionStart/PreCompact hooks. State machine
// is parsing-domain-aware (uses frontmatter.ReadVersion + semver.Compare).
// Owns canonical user-facing messages so multiple consumers (hook notice +
// init JSON output) speak identical words.
package drift

import (
	"fmt"
	"os"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
)

// State enumerates the drift relationships between plugin and installed rules.
//
// Note: initcmd mints an out-of-enum value `initcmd.DriftStateDelegated`
// ("delegated") carried in `initcmd.DriftReport.State` for the JSON channel
// only (scope=user, project copy deliberately absent). drift-package functions
// are never expected to receive it - it never comes from a drift.Check result.
type State string

const (
	StateEqual            State = "equal"             // versions match
	StateMissing          State = "missing"           // installed file absent
	StateUnknown          State = "unknown"           // installed present, version unparseable
	StateOutdated         State = "outdated"          // installed < plugin
	StateAhead            State = "ahead"             // installed > plugin (tampering / stale binary)
	StatePluginUnreadable State = "plugin-unreadable" // can't read plugin version; callers stay silent
)

// Result carries the outcome of a drift check.
type Result struct {
	State            State
	InstalledVersion string // "" when missing/unreadable
	PluginVersion    string // "" when plugin unreadable
}

// Check compares the plugin's rules version with the installed one. Returns
// state + parsed versions. Use Message(r) for the canonical user-facing text.
//
// installedRulesPath is the absolute path to <projectRoot>/.claude/rules/lets-rules.md.
// Pass it explicitly so this function stays I/O-policy-free.
func Check(pluginRulesPath, installedRulesPath string) Result {
	pluginVer := frontmatter.ReadVersion(pluginRulesPath)
	if pluginVer == "" {
		return Result{State: StatePluginUnreadable}
	}
	if _, err := os.Stat(installedRulesPath); os.IsNotExist(err) {
		return Result{State: StateMissing, PluginVersion: pluginVer}
	}
	installedVer := frontmatter.ReadVersion(installedRulesPath)
	if installedVer == "" {
		return Result{State: StateUnknown, PluginVersion: pluginVer}
	}
	switch semver.Compare("v"+installedVer, "v"+pluginVer) {
	case -1:
		return Result{State: StateOutdated, InstalledVersion: installedVer, PluginVersion: pluginVer}
	case 1:
		return Result{State: StateAhead, InstalledVersion: installedVer, PluginVersion: pluginVer}
	}
	return Result{State: StateEqual, InstalledVersion: installedVer, PluginVersion: pluginVer}
}

// Detected reports whether the state requires user action.
func (r Result) Detected() bool {
	return r.State != StateEqual && r.State != StatePluginUnreadable
}

// Message returns the canonical human-readable drift message for the 4 actionable
// states. Returns "" for StateEqual and StatePluginUnreadable.
//
// This is the single source of truth — both the SessionStart hook notice and
// the `lets init --json` output use this. Wording changes here propagate
// automatically to all consumers.
//
// StateMissing points at /lets:init (first-time install — no rules file yet).
// The other three point at /lets:update (sync an already-installed project with
// a fresh release): `lets update` re-copies the plugin rules on any Detected()
// state, including StateAhead, so no --force flag is needed.
func Message(r Result) string {
	switch r.State {
	case StateMissing:
		return "Workflow rules not installed in `.claude/rules/lets-rules.md`. Run `/lets:init` to install."
	case StateUnknown:
		return "Workflow rules version unknown - rules may be outdated. Run `/lets:update` to refresh."
	case StateOutdated:
		return fmt.Sprintf("Workflow rules outdated (installed v%s < plugin v%s). Run `/lets:update` to update.", r.InstalledVersion, r.PluginVersion)
	case StateAhead:
		return fmt.Sprintf("Workflow rules AHEAD of plugin (installed v%s > plugin v%s). Verify the rules file integrity (rules tampering signal) or upgrade the lets binary. Run `/lets:update` to reset to plugin version.", r.InstalledVersion, r.PluginVersion)
	}
	return ""
}

// MessageUser returns the canonical drift message for the USER-SCOPE installed
// copy at ~/.claude/rules/lets-rules.md. Same single-source-of-truth contract
// as Message; wording names the global path explicitly so a user with a synced
// project copy but drifted global copy knows which file the notice targets.
//
// StateMissing points at `lets init --user` (the global installer; /lets:init
// offers it when the plugin is user-scoped). The drifted states point at
// /lets:update (its user-rules artifact re-syncs the global copy) with
// `lets init --user` as the project-independent alternative.
//
// StateAhead deliberately does NOT promise a reset: a newer/customized global
// file is the documented opt-out mechanism and is never overwritten silently
// (unlike the project copy).
func MessageUser(r Result) string {
	switch r.State {
	case StateMissing:
		return "Global workflow rules not installed in `~/.claude/rules/lets-rules.md`. Run `lets init --user` (offered by `/lets:init`) to install."
	case StateUnknown:
		return "Global workflow rules version unknown - `~/.claude/rules/lets-rules.md` may be outdated. Run `/lets:update` (or `lets init --user`) to refresh."
	case StateOutdated:
		return fmt.Sprintf("Global workflow rules outdated (installed v%s < plugin v%s in `~/.claude/rules/lets-rules.md`). Run `/lets:update` (or `lets init --user`) to update.", r.InstalledVersion, r.PluginVersion)
	case StateAhead:
		return fmt.Sprintf("Global workflow rules AHEAD of plugin (installed v%s > plugin v%s in `~/.claude/rules/lets-rules.md`). If customized deliberately, ignore this; otherwise upgrade the lets binary + plugin.", r.InstalledVersion, r.PluginVersion)
	}
	return ""
}
