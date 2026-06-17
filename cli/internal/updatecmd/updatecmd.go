package updatecmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

const (
	// installScriptCmd is the BARE installer command (no prose, no backticks).
	// It is the single source for both binaryUpdateAction (prose) and
	// next_action.Command (execution-bound). SECURITY: only ever a compile-time
	// const - never fmt.Sprintf'd with a version or any dynamic/untrusted data.
	installScriptCmd   = "curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash"
	binaryUpdateAction = "Update the lets binary: `" + installScriptCmd + "`"
	pluginUpdateAction = "Update the plugin: `/plugin marketplace update lets-workflow`, then `/reload-plugins` (or restart Claude Code: `/exit`, reopen) - all in Claude Code, no terminal. Do it once and skip this in future: enable auto-update in `/plugin` -> Marketplaces -> lets-workflow."
)

// Options carries injectable dependencies. LatestFn resolves the latest stable
// release version; nil means "skip network checks" (--offline). The cobra
// wrapper sets it to a closure over FetchLatest; tests pass a stub.
type Options struct {
	LatestFn func(context.Context) (LatestInfo, error)
	// HomeDir is the user's home for the optional user-rules artifact
	// (~/.claude/rules/lets-rules.md). Empty = skip the artifact entirely
	// (resolution failed, or tests opting out via the zero value).
	HomeDir string
}

// Run checks the four core drift-able artifacts plus the optional user-scope
// rules copy: auto-syncs .env (header refresh when LETS_ENV_VERSION is stale),
// the project rules file (re-copy when drift is detected) and the user-level
// ~/.claude/rules/lets-rules.md when it exists (omitted otherwise), and
// reports version status for the lets binary and the Claude Code plugin
// (which it cannot self-update). Returns a fully populated Result; error is
// reserved for hard failures (write errors).
//
// Run does NOT prompt for config, touch settings.json, or run beads - that's
// `lets init`'s job. `lets update` only syncs version-pinned artifacts.
func Run(ctx context.Context, opts Options, projectRoot, pluginRoot string) (Result, error) {
	result := NewResult(projectRoot, pluginRoot)

	// Resolve "latest" once, shared by the binary and plugin checks.
	offline := opts.LatestFn == nil
	var latest LatestInfo
	var latestErr error
	if !offline {
		latest, latestErr = opts.LatestFn(ctx)
	}

	// --- Artifact 1: .lets/.env ---
	envPath := filepath.Join(projectRoot, ".lets", ".env")
	_, envStatErr := os.Stat(envPath)
	switch {
	case os.IsNotExist(envStatErr):
		result.Add(Artifact{Name: ".env", Status: StatusNotInitialized, Action: "Run /lets:init"})
	case version.IsDev():
		// A dev binary has no real version to stamp; RegenerateEnv would rewrite
		// LETS_ENV_VERSION to "dev" (a confusing downgrade). Skip it - same
		// short-circuit the binary artifact below uses.
		result.Add(Artifact{Name: ".env", Status: StatusDev, Detail: "untagged dev build - .env regen skipped"})
	default:
		action, err := initcmd.RegenerateEnv(envPath, initcmd.Prefs{Tracker: letsconfig.Defaults()["LETS_TRACKER"]})
		if err != nil {
			return result, fmt.Errorf("regenerate .env: %w", err)
		}
		switch action.Kind {
		case initcmd.EnvSkip:
			result.Add(Artifact{Name: ".env", Status: StatusInSync, CurrentVersion: action.PrevVersion, Detail: "tracks the lets binary"})
		case initcmd.EnvRegenerated:
			a := Artifact{Name: ".env", Status: StatusUpdated, CurrentVersion: action.NewVersion}
			if len(action.ChangedKeys) > 0 {
				a.Detail = fmt.Sprintf("was %s; %d key(s) changed; backup %s", version.Format(action.PrevVersion), len(action.ChangedKeys), action.BackupPath)
			} else {
				a.Detail = fmt.Sprintf("was %s; header refreshed; backup %s", version.Format(action.PrevVersion), action.BackupPath)
			}
			result.Add(a)
		case initcmd.EnvCreated:
			// Unreachable: the os.Stat above already routed a missing file to
			// StatusNotInitialized. Kept for switch completeness.
			result.Add(Artifact{Name: ".env", Status: StatusUpdated, CurrentVersion: action.NewVersion, Detail: "created"})
		}
	}

	// --- Artifact 2: lets binary --- (emitted before rules so the deferral gate
	// below can read the plugin's status off the artifact list, not re-derive it)
	result.Add(versionArtifact("binary", version.Version, latest, latestErr, offline, binaryUpdateAction))

	// --- Artifact 3: Claude Code plugin ---
	pluginVer := ReadPluginVersion(pluginRoot)
	result.Add(versionArtifact("plugin", pluginVer, latest, latestErr, offline, pluginUpdateAction))

	// Order-aware gate (lets-rlue4): the plugin is "behind" when it is outdated
	// vs the latest release (read straight off the plugin artifact just added -
	// no duplicated latest-compare, no latestErr divergence) OR behind the binary
	// locally (offline-safe; versionArtifact never compares plugin-vs-binary).
	// When behind, do NOT advance the rules file to the stale plugin's version -
	// that's the half-step. Consumed by the rules + user-rules blocks below.
	pluginBehind := false
	for _, a := range result.Artifacts {
		if a.Name == "plugin" && a.Status == StatusOutdated {
			pluginBehind = true
			break
		}
	}
	if pluginVer != "" && !version.IsDevString(pluginVer) {
		if bv := version.Version; bv != "" && !version.IsDevString(bv) &&
			semver.Compare("v"+pluginVer, "v"+bv) < 0 {
			pluginBehind = true
		}
	}

	// Rules scope + user-scope presence, resolved once (consumed by Artifact 4
	// and Artifact 5). Mirror the hook's MERGED read - project value, else the
	// user-level ~/.lets/.env value - so a hand-added scope in ~/.lets/.env
	// can't make hook and update contradict each other (the boomerang would
	// return through that side door). Anything other than "user" degrades to
	// project semantics - same fail-safe contract as initcmd.effectiveRulesScope.
	envScopeValue := func(path string) string {
		if data, err := os.ReadFile(path); err == nil {
			if vals, perr := envfile.Parse(bytes.NewReader(data)); perr == nil {
				return vals["LETS_RULES_SCOPE"]
			}
		}
		return ""
	}
	rulesScope := envScopeValue(filepath.Join(projectRoot, ".lets", ".env"))
	if rulesScope == "" && opts.HomeDir != "" {
		rulesScope = envScopeValue(filepath.Join(opts.HomeDir, ".lets", ".env"))
	}
	userRulesDst := ""
	globalPresent := false
	if opts.HomeDir != "" {
		userRulesDst = filepath.Join(opts.HomeDir, ".claude", "rules", "lets-rules.md")
		if _, err := os.Stat(userRulesDst); err == nil {
			globalPresent = true
		}
	}

	// --- Artifact 4: .claude/rules/lets-rules.md ---
	rulesSrc := filepath.Join(pluginRoot, "rules", "lets-rules.md")
	rulesDst := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	if rulesData, readErr := os.ReadFile(rulesSrc); readErr != nil {
		result.Add(Artifact{Name: "rules", Status: StatusUnknown, Detail: fmt.Sprintf("plugin rules unreadable: %s", rulesSrc)})
	} else {
		dr := drift.Check(rulesSrc, rulesDst)
		// dupHint annotates the synced/in-sync paths when scope=user yet a
		// project copy exists - the user said "rely on global" but both load
		// (duplication). Report-only; never auto-delete (git rm is a team action).
		dupHint := ""
		if rulesScope == "user" {
			if globalPresent {
				dupHint = " (duplication: scope=user but a project copy exists - remove it or set LETS_RULES_SCOPE=project)"
			} else {
				dupHint = " (scope=user but only the project copy exists - run lets init --user or set scope=project)"
			}
		}
		switch {
		case dr.State == drift.StatePluginUnreadable:
			// rulesSrc exists and is readable (the os.ReadFile above succeeded)
			// but has no parseable `version:` frontmatter - "couldn't check",
			// not "up to date".
			result.Add(Artifact{Name: "rules", Status: StatusUnknown, Detail: "plugin rules version unparseable (no `version:` frontmatter)"})
		case dr.State == drift.StateMissing && rulesScope == "user" && globalPresent:
			// Delegated: the project copy is deliberately absent; rules come from
			// the global ~/.claude/rules copy. Target state, not drift - never
			// re-create it (that was the boomerang bug).
			result.Add(Artifact{Name: "rules", Status: StatusDelegated, Detail: "scope=user - rules come from the global copy (~/.claude/rules)"})
		case dr.State == drift.StateMissing && rulesScope == "user":
			// scope=user but the global copy is missing too - nothing covers the
			// project. Surface an actionable fix instead of silently re-copying.
			result.Add(Artifact{Name: "rules", Status: StatusNotInitialized, Action: "Run `lets init --user` to restore the global rules (or set LETS_RULES_SCOPE=project)", Detail: "scope=user but no rules anywhere - global copy missing"})
		case dr.State == drift.StateOutdated && pluginBehind:
			// Half-step guard (lets-rlue4): installed rules are behind the plugin,
			// but the plugin itself is behind - writing now would advance to a
			// stale lower version. Hold until the plugin is updated. ONLY
			// StateOutdated defers: StateMissing must still write below (behind
			// rules beat NO rules), StateAhead/StateUnknown keep their behavior.
			result.Add(Artifact{Name: "rules", Status: StatusDeferred, Detail: fmt.Sprintf("plugin behind (plugin v%s); rules sync deferred until the plugin is updated", pluginVer)})
		case dr.Detected():
			if err := os.MkdirAll(filepath.Dir(rulesDst), 0o755); err != nil {
				return result, err
			}
			if err := initcmd.AtomicWriteBytes(rulesDst, rulesData, 0o644); err != nil {
				return result, fmt.Errorf("write rules: %w", err)
			}
			drPost := drift.Check(rulesSrc, rulesDst)
			result.Add(Artifact{Name: "rules", Status: StatusUpdated, CurrentVersion: drPost.InstalledVersion, Detail: rulesUpdatedDetail(dr) + dupHint})
		default:
			result.Add(Artifact{Name: "rules", Status: StatusInSync, CurrentVersion: dr.InstalledVersion, Detail: "tracks the plugin" + dupHint})
		}
	}

	// --- Artifact 5: ~/.claude/rules/lets-rules.md (user scope, optional) ---
	// Row appears ONLY when the file exists: absence means "user-scope install
	// not in use" - the normal state for project-scope users, not a status
	// worth a row (and keeps project-only output identical to the 4-artifact
	// era). update never bootstraps user scope; that's `lets init --user`.
	//
	// ahead = no-clobber (unlike the project rules artifact): a global file
	// newer than the plugin is a user customization (the only per-project
	// opt-out, GH anthropics/claude-code#8395) or a newer release's copy -
	// never silently reset.
	if globalPresent {
		if rulesData, readErr := os.ReadFile(rulesSrc); readErr != nil {
			result.Add(Artifact{Name: "user-rules", Status: StatusUnknown, Detail: fmt.Sprintf("plugin rules unreadable: %s", rulesSrc)})
		} else {
			dr := drift.Check(rulesSrc, userRulesDst)
			switch {
			// StatePluginUnreadable = the SOURCE (plugin payload) has no
			// parseable version. An INSTALLED global file with broken
			// frontmatter is StateUnknown instead -> Detected() -> rewritten
			// below (lets-* files are plugin-owned by convention).
			case dr.State == drift.StatePluginUnreadable:
				result.Add(Artifact{Name: "user-rules", Status: StatusUnknown, Detail: "plugin rules version unparseable (no `version:` frontmatter)"})
			case dr.State == drift.StateAhead:
				result.Add(Artifact{Name: "user-rules", Status: StatusAhead, CurrentVersion: dr.InstalledVersion, Detail: "global rules newer than the plugin - customized or newer release; not overwritten"})
			case dr.State == drift.StateOutdated && pluginBehind:
				// Half-step guard (lets-rlue4): mirrors the project-rules block -
				// hold the global rules sync while the plugin is behind.
				result.Add(Artifact{Name: "user-rules", Status: StatusDeferred, Detail: fmt.Sprintf("plugin behind (plugin v%s); global rules sync deferred until the plugin is updated", pluginVer)})
			case dr.Detected():
				if err := initcmd.AtomicWriteBytes(userRulesDst, rulesData, 0o644); err != nil {
					return result, fmt.Errorf("write user rules: %w", err)
				}
				drPost := drift.Check(rulesSrc, userRulesDst)
				result.Add(Artifact{Name: "user-rules", Status: StatusUpdated, CurrentVersion: drPost.InstalledVersion, Detail: rulesUpdatedDetail(dr)})
			default:
				result.Add(Artifact{Name: "user-rules", Status: StatusInSync, CurrentVersion: dr.InstalledVersion, Detail: "tracks the plugin"})
			}
		}
	}

	// --- Artifact 6: .claude/rules/tracker-<name>.md (project scope, optional) ---
	// Row appears ONLY when LETS_TRACKER names an adapter the plugin ships
	// (tracker-<name>.md present in the plugin payload). Absence - unset tracker,
	// a typo, or a plugin without the file - means NO row, so a project that
	// predates the tracker platform keeps the pre-tracker artifact set unchanged.
	// Plugin-version-locked like user-rules: excluded from Consistent (it always
	// equals the plugin after a sync). Tracker rules are always project-local (no
	// user scope). Mirrors Artifact 4's drift/deferral/write pattern.
	trackerName := ""
	if data, err := os.ReadFile(filepath.Join(projectRoot, ".lets", ".env")); err == nil {
		if vals, perr := envfile.Parse(bytes.NewReader(data)); perr == nil {
			trackerName = vals["LETS_TRACKER"]
		}
	}
	if trackerName != "" && initcmd.ValidTrackerName(trackerName) {
		trackerSrc := filepath.Join(pluginRoot, "rules", "tracker-"+trackerName+".md")
		if trackerData, readErr := os.ReadFile(trackerSrc); readErr == nil {
			trackerDst := filepath.Join(projectRoot, ".claude", "rules", "tracker-"+trackerName+".md")
			dr := drift.Check(trackerSrc, trackerDst)
			switch {
			case dr.State == drift.StatePluginUnreadable:
				result.Add(Artifact{Name: "tracker-rules", Status: StatusUnknown, Detail: "plugin tracker rules version unparseable (no `version:` frontmatter)"})
			case dr.State == drift.StateOutdated && pluginBehind:
				result.Add(Artifact{Name: "tracker-rules", Status: StatusDeferred, Detail: fmt.Sprintf("plugin behind (plugin v%s); tracker rules sync deferred until the plugin is updated", pluginVer)})
			case dr.Detected():
				if err := os.MkdirAll(filepath.Dir(trackerDst), 0o755); err != nil {
					return result, err
				}
				if err := initcmd.AtomicWriteBytes(trackerDst, trackerData, 0o644); err != nil {
					return result, fmt.Errorf("write tracker rules: %w", err)
				}
				drPost := drift.Check(trackerSrc, trackerDst)
				result.Add(Artifact{Name: "tracker-rules", Status: StatusUpdated, CurrentVersion: drPost.InstalledVersion, Detail: rulesUpdatedDetail(dr) + fmt.Sprintf(" (tracker-%s.md)", trackerName)})
			default:
				result.Add(Artifact{Name: "tracker-rules", Status: StatusInSync, CurrentVersion: dr.InstalledVersion, Detail: fmt.Sprintf("tracks the plugin (tracker-%s.md)", trackerName)})
			}
		}
		// source absent -> no row (init's Step 8b warns; update stays quiet about a
		// missing adapter to keep the artifact set stable).
	}

	// Cross-reference: when an in-sync artifact's local source (the binary for
	// .env, the plugin for rules) is itself behind the latest release, say so on
	// the row. Runs after all four artifacts exist so per-artifact computation
	// stays independent (lets-kaw72).
	annotateInSyncBehind(&result)

	// --- internal consistency (binary == plugin == installed-rules frontmatter) ---
	// user-rules is DELIBERATELY excluded: after a sync it always equals the
	// plugin (zero signal), and the one divergent case - `ahead` - is a
	// deliberate customization that must not permanently trip the
	// "inconsistent install" warning (it would contradict the artifact row's
	// own "customized, not an error" detail).
	result.Consistent = consistentVersions(version.Version, pluginVer, frontmatter.ReadVersion(rulesDst))

	// The single ordered next step (lets-rlue4) - derived purely from the
	// artifact statuses computed above, so it can never diverge from the rows.
	computeNextAction(&result, version.Version, latest)

	return result, nil
}

// rulesUpdatedDetail renders a past-tense summary of what the rules file was
// before `lets update` re-copied it. The row is already `updated`, so it must
// NOT carry the pre-install imperative message ("Run /lets:init") - that
// contradicts the status (lets-kaw72). Mirrors the .env "was v…" style.
func rulesUpdatedDetail(pre drift.Result) string {
	switch pre.State {
	case drift.StateMissing:
		return "was missing"
	case drift.StateOutdated:
		return fmt.Sprintf("was outdated (v%s)", pre.InstalledVersion)
	case drift.StateAhead:
		return fmt.Sprintf("was ahead (v%s)", pre.InstalledVersion)
	case drift.StateUnknown:
		return "was unparseable"
	default:
		return ""
	}
}

// annotateInSyncBehind appends "(itself behind latest v…)" to an in-sync row
// whose tracked upstream is itself outdated, so two in-sync rows at different
// versions read as explained rather than contradictory.
func annotateInSyncBehind(r *Result) {
	upstreamOf := map[string]string{".env": "binary", "rules": "plugin", "user-rules": "plugin", "tracker-rules": "plugin"}
	latestBehind := map[string]string{} // outdated upstream name -> its latest version
	for _, a := range r.Artifacts {
		if (a.Name == "binary" || a.Name == "plugin") && a.Status == StatusOutdated {
			latestBehind[a.Name] = a.LatestVersion
		}
	}
	for i := range r.Artifacts {
		a := &r.Artifacts[i]
		if a.Status != StatusInSync {
			continue
		}
		lv, ok := latestBehind[upstreamOf[a.Name]]
		if !ok {
			continue
		}
		hint := fmt.Sprintf("itself behind latest v%s", lv)
		if a.Detail == "" {
			a.Detail = hint
		} else {
			a.Detail += " (" + hint + ")"
		}
	}
}

// computeNextAction sets result.NextAction to the single ordered step the user
// should take this run. Order: init -> binary -> plugin -> reload -> done. The
// loop is idempotent: do the one step, rerun `lets update`, repeat until done.
// It reads only the already-computed artifact statuses (no re-deriving version
// comparisons); binaryVer is used only for the `done` Version fallback.
func computeNextAction(r *Result, binaryVer string, latest LatestInfo) {
	status := func(name string) (ArtifactStatus, Artifact) {
		for _, a := range r.Artifacts {
			if a.Name == name {
				return a.Status, a
			}
		}
		return "", Artifact{}
	}

	// 1. Anything not initialized - the project `.env` (run /lets:init), or a
	//    rules row under scope=user with no global copy (run lets init --user).
	//    Scan ALL artifacts so a not-initialized rules row can't sit in
	//    ActionNeeded while next_action says "done" (the single-source invariant).
	for _, a := range r.Artifacts {
		if a.Status == StatusNotInitialized {
			msg := a.Action
			if msg == "" {
				msg = "This project isn't set up yet - run /lets:init."
			}
			r.NextAction = &NextAction{Kind: "init", Message: msg}
			return
		}
	}
	// 2. Binary behind latest - do this FIRST so rules later sync to the right
	//    version (and so a fresh install doesn't chase an outdated plugin).
	if s, a := status("binary"); s == StatusOutdated {
		r.NextAction = &NextAction{
			Kind:    "binary",
			Message: fmt.Sprintf("Update the lets binary (v%s -> v%s).", a.CurrentVersion, a.LatestVersion),
			Command: installScriptCmd,
		}
		return
	}
	// 3. Plugin behind - outdated vs latest, OR behind the binary (surfaced as a
	//    deferred rules row, which is the only behind signal available offline).
	pluginOutdated := func() bool { s, _ := status("plugin"); return s == StatusOutdated }
	rulesDeferred := func() bool {
		for _, a := range r.Artifacts {
			if (a.Name == "rules" || a.Name == "user-rules" || a.Name == "tracker-rules") && a.Status == StatusDeferred {
				return true
			}
		}
		return false
	}
	if pluginOutdated() || rulesDeferred() {
		r.NextAction = &NextAction{
			Kind:    "plugin",
			Message: "Update the Claude Code plugin: /plugin marketplace update lets-workflow, then /reload-plugins.",
		}
		return
	}
	// 4. Rules were just synced - reload so the running session picks them up.
	for _, a := range r.Artifacts {
		if (a.Name == "rules" || a.Name == "user-rules" || a.Name == "tracker-rules") && a.Status == StatusUpdated {
			r.NextAction = &NextAction{Kind: "reload", Message: "Restart Claude Code so the updated rules load - /exit, then reopen."}
			return
		}
	}
	// 5. Nothing pending. Only claim "latest release" when we actually checked it
	//    (latest.Version set = online); offline we report a known version without
	//    asserting it's latest. Version falls back to the binary when offline.
	ver := latest.Version
	msg := "Everything is on the latest release."
	if latest.Version == "" {
		msg = "Nothing pending locally - couldn't verify the latest release."
		if !version.IsDevString(binaryVer) {
			ver = binaryVer
		}
	}
	r.NextAction = &NextAction{Kind: "done", Message: msg, Version: ver}
}

// versionArtifact builds an Artifact for a version-only artifact (binary, plugin).
func versionArtifact(name, current string, latest LatestInfo, latestErr error, offline bool, action string) Artifact {
	a := Artifact{Name: name, CurrentVersion: current}
	if current == "" {
		a.Status = StatusUnknown
		a.Detail = "could not determine installed version"
		return a
	}
	if version.IsDevString(current) {
		a.Status = StatusDev
		a.Detail = "untagged dev build - no release comparison"
		return a
	}
	if offline {
		a.Status = StatusUnknown
		a.Detail = "skipped (--offline)"
		return a
	}
	if latestErr != nil || latest.Version == "" {
		a.Status = StatusUnknown
		a.Detail = "could not check the latest release on github.com (offline, rate-limited, or no published release)"
		return a
	}
	a.LatestVersion = latest.Version
	if latest.Source == "cache" {
		a.Detail = fmt.Sprintf("latest checked %s ago", durationApprox(time.Since(latest.CheckedAt)))
	}
	switch semver.Compare("v"+current, "v"+latest.Version) {
	case 0:
		a.Status = StatusUpToDate
	case -1:
		a.Status = StatusOutdated
		a.Action = action
	case 1:
		a.Status = StatusAhead
		if a.Detail != "" {
			a.Detail += "; "
		}
		a.Detail += "newer than the latest stable release (prerelease or local checkout)"
	}
	return a
}

// consistentVersions reports whether all non-empty, non-dev version strings
// among the args are mutually equal. Dev sentinels (incl. dev-<metadata>) are
// excluded from the comparison set - they have no semver to compare against.
func consistentVersions(vs ...string) bool {
	var seen string
	for _, v := range vs {
		if v == "" || version.IsDevString(v) {
			continue
		}
		if seen == "" {
			seen = v
			continue
		}
		if v != seen {
			return false
		}
	}
	return true
}

// durationApprox renders a coarse human duration ("3m", "2h", "5d"). Sub-minute
// and negative inputs (clock skew) collapse to "<1m".
func durationApprox(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// PrintReport writes a human-readable status table to w.
func PrintReport(w io.Writer, r Result) {
	fmt.Fprintln(w, "LETS Update Status")
	fmt.Fprintln(w, "==================")
	for _, a := range r.Artifacts {
		ver := "?"
		if a.CurrentVersion != "" {
			ver = version.Format(a.CurrentVersion)
		}
		line := fmt.Sprintf("%-10s %-9s %s", a.Name, ver, a.Status)
		if a.LatestVersion != "" {
			line += fmt.Sprintf(" (latest %s)", version.Format(a.LatestVersion))
		}
		if a.Detail != "" {
			line += " - " + a.Detail
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w)
	na := r.NextAction
	// The inconsistent-install warning is redundant when the single next action
	// already explains a partial state (a deferred-rules run trips !Consistent by
	// design); show it only for the reload/done tail.
	if !r.Consistent && (na == nil || (na.Kind != "binary" && na.Kind != "plugin")) {
		fmt.Fprintln(w, "warning: local install is inconsistent (binary / plugin / rules versions differ) - likely a partial upgrade")
	}
	if na == nil {
		return
	}
	// Single ordered next action (lets-rlue4) - one step per run, rerun until done.
	switch na.Kind {
	case "done":
		if na.Version != "" {
			fmt.Fprintf(w, "Everything on v%s.\n", na.Version)
		} else {
			fmt.Fprintln(w, "Nothing to do (couldn't verify the latest release - rerun with network).")
		}
	default:
		fmt.Fprintf(w, "Next: %s\n", na.Message)
		if na.Command != "" {
			fmt.Fprintf(w, "  %s\n", na.Command)
		}
	}
}
