package updatecmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/rulescache"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

const (
	// installScriptCmd is the BARE installer command (no prose, no backticks).
	// It is the single source for both binaryUpdateAction (prose) and
	// next_action.Command (execution-bound). SECURITY: only ever a compile-time
	// const - never fmt.Sprintf'd with a version or any dynamic/untrusted data.
	installScriptCmd   = "curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash"
	binaryUpdateAction = "Update the lets binary: `" + installScriptCmd + "`"
	pluginUpdateAction = "Update the plugin: `/plugin marketplace update lets-workflow`, then `/reload-plugins` (or start a new session) - all in Claude Code, no terminal."
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
	// MainCheckout is set when update runs inside a linked worktree: the
	// project rows (.env, rules, tracker-rules) are skipped and name it, since
	// .claude/ is not shared into worktrees (lets-tg008).
	MainCheckout string
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
	result.MainCheckout = opts.MainCheckout
	skipped := func(name string) Artifact {
		return Artifact{Name: name, Status: StatusSkipped, Detail: "worktree - project files are synced from the main checkout " + opts.MainCheckout}
	}
	loadedRoot := pluginRoot
	pluginRoot, rootVerified, resolveNote := ResolveInstalledRoot(pluginRoot, opts.HomeDir)

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
	case opts.MainCheckout != "":
		result.Add(skipped(".env"))
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
	if filepath.Clean(pluginRoot) != filepath.Clean(loadedRoot) {
		last := &result.Artifacts[len(result.Artifacts)-1]
		last.Detail = strings.TrimPrefix(last.Detail+fmt.Sprintf("; v%s is installed - this session still runs v%s", pluginVer, ReadPluginVersion(loadedRoot)), "; ")
		result.LoadedPluginVersion = ReadPluginVersion(loadedRoot)
	} else if !rootVerified && resolveNote != "" {
		last := &result.Artifacts[len(result.Artifacts)-1]
		last.Detail = strings.TrimPrefix(last.Detail+"; "+resolveNote, "; ")
	}

	// Order-aware gate (lets-rlue4): the plugin is "behind" when it is outdated
	// vs the latest release (read straight off the plugin artifact just added -
	// no duplicated latest-compare, no latestErr divergence) OR behind the binary
	// locally (offline-safe; versionArtifact never compares plugin-vs-binary).
	// When behind, do NOT advance the rules file to the stale plugin's version -
	// that's the half-step. Consumed by the rules + tracker-rules blocks below.
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
	if opts.MainCheckout != "" {
		result.Add(skipped("rules"))
	} else if rulesData, readErr := os.ReadFile(rulesSrc); readErr != nil {
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

	// --- Artifact 5: ~/.claude/rules/lets-rules.md - read-only (lets-tg008) ---
	// The session hook owns this file (rulescache): update never writes it and
	// never calls it `in-sync` - the row names what the copy is keyed to, and is
	// healthy (delegated) only when that is verified against the plugin.
	if globalPresent {
		result.Add(userRulesInfo(opts.HomeDir, userRulesDst, pluginRoot, pluginVer, rootVerified))
	}

	// --- Artifact 6: .claude/rules/tracker-<name>.md (project scope, optional) ---
	// Row appears when LETS_TRACKER names an adapter the plugin ships
	// (tracker-<name>.md in the payload), OR a user-authored/unshipped adapter with
	// an installed .claude/rules/tracker-<name>.md copy (reported delegated, "left
	// as-is"). Only a name with NEITHER a shipped source NOR an installed copy -
	// unset, a typo, or a pre-platform project - yields NO row, so such a project
	// keeps the pre-tracker artifact set unchanged.
	// Plugin-version-locked like user-rules: excluded from Consistent (it always
	// equals the plugin after a sync). Tracker rules are always project-local (no
	// user scope). Mirrors Artifact 4's drift/deferral/write pattern.
	trackerName := ""
	if data, err := os.ReadFile(filepath.Join(projectRoot, ".lets", ".env")); err == nil {
		if vals, perr := envfile.Parse(bytes.NewReader(data)); perr == nil {
			trackerName = vals["LETS_TRACKER"]
		}
	}
	if opts.MainCheckout != "" {
		result.Add(skipped("tracker-rules"))
	} else if trackerName != "" && initcmd.ValidTrackerName(trackerName) {
		trackerSrc := filepath.Join(pluginRoot, "rules", "tracker-"+trackerName+".md")
		if trackerData, readErr := os.ReadFile(trackerSrc); readErr == nil {
			trackerDst := filepath.Join(projectRoot, ".claude", "rules", "tracker-"+trackerName+".md")
			rulesDir := filepath.Dir(trackerDst)
			// The documented switch path is "edit LETS_TRACKER in .lets/.env, then
			// /lets:update" (init.md 2c-quater, docs/trackers.md) - so update must apply
			// the same switch semantics as init Step 8b: drop the deactivated shipped
			// adapter (never two adapter files loaded at once) and scaffold the
			// create-once board profile. Both are independent of the version-sync
			// deferral below: the switch is user intent read from .env, not a
			// plugin-content sync. Notes land on the tracker-rules row's Detail.
			var switchNotes []string
			removed, removeFailed := initcmd.CleanupShippedTrackerFiles(rulesDir, trackerName)
			for _, r := range removed {
				switchNotes = append(switchNotes, r+" removed (tracker switched)")
			}
			for _, f := range removeFailed {
				switchNotes = append(switchNotes, f+" is stale but could not be removed - two adapters loaded, remove it manually")
			}
			if msg, berr := initcmd.ScaffoldBoardOnce(pluginRoot, rulesDir, trackerName); berr != nil {
				switchNotes = append(switchNotes, berr.Error())
			} else if msg != "" {
				switchNotes = append(switchNotes, fmt.Sprintf("tracker-%s.board.md scaffolded (user-owned)", trackerName))
			}
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
			// Attach the switch notes to the tracker-rules row just added.
			if len(switchNotes) > 0 {
				last := &result.Artifacts[len(result.Artifacts)-1]
				last.Detail = strings.TrimPrefix(last.Detail+"; "+strings.Join(switchNotes, "; "), "; ")
			}
		} else if os.IsNotExist(readErr) {
			// No plugin-shipped source: a user-authored (unshipped) adapter. If an
			// installed copy exists in .claude/rules/, report it left-as-is in the
			// green group (parity with init Step 8b's gentle skip), never an error.
			// Nothing installed either -> no row (typo / pre-tracker project). A
			// non-IsNotExist read error stays silently ignored, as before.
			trackerDst := filepath.Join(projectRoot, ".claude", "rules", "tracker-"+trackerName+".md")
			if _, derr := os.Stat(trackerDst); derr == nil {
				result.Add(Artifact{Name: "tracker-rules", Status: StatusDelegated, Detail: fmt.Sprintf("user-authored adapter (tracker-%s.md not shipped) - left as-is", trackerName)})
			}
		}
	}

	// Cross-reference: when an in-sync artifact's local source (the binary for
	// .env, the plugin for rules) is itself behind the latest release, say so on
	// the row. Runs after all four artifacts exist so per-artifact computation
	// stays independent (lets-kaw72).
	annotateInSyncBehind(&result)

	// --- internal consistency (binary == plugin == installed-rules frontmatter) ---
	// user-rules is DELIBERATELY excluded: the row is informational - the
	// session hook owns that file (rulescache, lets-tg008) and the row reports
	// what it is keyed to, never a version update would reconcile here.
	if opts.MainCheckout != "" {
		// A worktree run skipped the project rules - never judge consistency by a
		// file this run did not check.
		result.Consistent = consistentVersions(version.Version, pluginVer)
	} else {
		result.Consistent = consistentVersions(version.Version, pluginVer, frontmatter.ReadVersion(rulesDst))
	}

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

// userRulesInfo describes the hook-maintained global rules. `delegated`
// (counted healthy) needs proof: the plugin root is VERIFIED as the installed one
// (ResolveInstalledRoot), its version is known, and the key, the file and that
// plugin's rules share one hash while the key names that version. Anything else
// is `unknown` with the reason. HookPending is set only when rulescache.Plan -
// the hook's own decision, computed without writing - says the next session
// start will change the file, so next_action never promises more than that.
func userRulesInfo(home, dst, pluginRoot, pluginVer string, verified bool) Artifact {
	a := Artifact{Name: "user-rules", Status: StatusUnknown}
	k, haveKey := rulescache.ReadKey(home)
	if haveKey {
		a.CurrentVersion = k.Version
		a.Detail = fmt.Sprintf("maintained by the session hook - cache of plugin v%s (sha256 %s)", k.Version, k.Hash[:12])
	} else {
		a.Detail = "maintained by the session hook - not recorded yet"
	}
	dstData, derr := os.ReadFile(dst)
	srcData, serr := os.ReadFile(filepath.Join(pluginRoot, "rules", "lets-rules.md"))
	if !verified || serr != nil || pluginVer == "" {
		a.Detail += "; not verified against the installed plugin (see the plugin row)"
		return a
	}
	if haveKey && derr == nil && rulescache.Sum(dstData) == k.Hash && rulescache.Sum(srcData) == k.Hash && k.Version == pluginVer {
		a.Status = StatusDelegated
		return a
	}
	var why string
	switch {
	case !haveKey:
		why = "; the next Claude Code session start records it"
	case derr != nil || rulescache.Sum(dstData) != k.Hash:
		why = "; edited since the last sync - the next session start saves the edit to a .bak and restores the plugin copy"
	default:
		why = fmt.Sprintf("; the installed plugin v%s carries different rules - the next session start refreshes them", pluginVer)
	}
	switch p := rulescache.Plan(rulescache.Options{PluginRoot: pluginRoot, HomeDir: home}); p.Outcome {
	case rulescache.OutcomeNoop, rulescache.OutcomeCreated, rulescache.OutcomeWritten:
		a.Detail += why
		a.HookPending = true
	default:
		a.Detail += "; the next session start will not change it - " + p.Notice()
	}
	return a
}

// annotateInSyncBehind appends "(itself behind latest v…)" to an in-sync row
// whose tracked upstream is itself outdated, so two in-sync rows at different
// versions read as explained rather than contradictory.
func annotateInSyncBehind(r *Result) {
	upstreamOf := map[string]string{".env": "binary", "rules": "plugin", "tracker-rules": "plugin"}
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
// should take this run. Order: init -> binary -> plugin | reload (a newer
// plugin installed, not loaded) -> reload (rules synced) -> main-checkout (a
// worktree run) -> new-session | user-rules (the global rules row) -> done. The
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
			if (a.Name == "rules" || a.Name == "tracker-rules") && a.Status == StatusDeferred {
				return true
			}
		}
		return false
	}
	//    A newer plugin already installed but not loaded by this session needs a
	//    reload, not an update: a re-run before it reports the same (lets-tg008).
	if r.LoadedPluginVersion != "" {
		_, p := status("plugin")
		r.NextAction = &NextAction{
			Kind:    "reload",
			Message: fmt.Sprintf("v%s is already installed; this session still runs v%s. Run /reload-plugins or start a new session - re-running /lets:update before that reports the same.", p.CurrentVersion, r.LoadedPluginVersion),
		}
		return
	}
	if pluginOutdated() || rulesDeferred() {
		r.NextAction = &NextAction{
			Kind:    "plugin",
			Message: "Update the plugin: /plugin marketplace update lets-workflow, then /reload-plugins (or start a new session).",
		}
		return
	}
	// 4. Rules were just synced - reload so the running session picks them up.
	for _, a := range r.Artifacts {
		if (a.Name == "rules" || a.Name == "tracker-rules") && a.Status == StatusUpdated {
			r.NextAction = &NextAction{Kind: "reload", Message: "Restart Claude Code so the updated rules load - /exit, then reopen."}
			return
		}
	}
	// 4a. Project rows skipped from a worktree - they sync in the main checkout.
	for _, a := range r.Artifacts {
		if a.Status == StatusSkipped {
			r.NextAction = &NextAction{Kind: "main-checkout", Message: "Project files were not synced from this worktree - run /lets:update in the main checkout: " + r.MainCheckout}
			return
		}
	}
	// 4b. The global rules row is not verified healthy (lets-tg008). Say what
	// actually happens next: the session hook fixes only HookPending states;
	// anything else is a diagnostic, never a promise and never "re-run".
	for _, a := range r.Artifacts {
		if a.Name != "user-rules" || a.Status != StatusUnknown {
			continue
		}
		if a.HookPending {
			r.NextAction = &NextAction{Kind: "new-session", Message: "The global rules (~/.claude/rules/lets-rules.md) refresh at the next Claude Code session start - re-running /lets:update will not change them."}
		} else {
			r.NextAction = &NextAction{Kind: "user-rules", Message: "Global rules not verified: " + a.Detail}
		}
		return
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
