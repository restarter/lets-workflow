package updatecmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// setVersion snapshots and restores version.Version per the package convention.
func setVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = orig })
}

// rulesFile writes a minimal rules markdown with the given frontmatter version.
func rulesFile(t *testing.T, path, ver string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("---\nname: lets-rules\nversion: %s\n---\n\n# rules\n", ver)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scaffold builds a fake project + plugin root. envVer/rulesVer/pluginVer/binVer
// pin each artifact's version ("" for envVer/rulesVer means that file is absent).
func scaffold(t *testing.T, envVer, rulesVer, pluginVer, binVer string) (projectRoot, pluginRoot string) {
	t.Helper()
	projectRoot = t.TempDir()
	pluginRoot = t.TempDir()
	setVersion(t, binVer)

	writePluginJSON(t, pluginRoot, fmt.Sprintf(`{"name":"lets","version":%q}`, pluginVer))
	rulesFile(t, filepath.Join(pluginRoot, "rules", "lets-rules.md"), pluginVer)

	if rulesVer != "" {
		rulesFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"), rulesVer)
	}
	if envVer != "" {
		envDir := filepath.Join(projectRoot, ".lets")
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Full canonical key set so Artifact 1's RegenerateEnv reports in-sync
		// (a missing key would diff → `updated`, not the `in-sync` these tests want).
		body := "# LETS plugin config\nLETS_ENV_VERSION=" + envVer + "\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\nLETS_LAUNCHER=terminal\nLETS_RULES_SCOPE=project\n"
		if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectRoot, pluginRoot
}

func stubLatest(v string) func(context.Context) (LatestInfo, error) {
	return func(context.Context) (LatestInfo, error) {
		return LatestInfo{Version: v, Source: "live", CheckedAt: time.Now()}, nil
	}
}

func stubCached(v string, age time.Duration) func(context.Context) (LatestInfo, error) {
	return func(context.Context) (LatestInfo, error) {
		return LatestInfo{Version: v, Source: "cache", CheckedAt: time.Now().Add(-age)}, nil
	}
}

func find(t *testing.T, r Result, name string) Artifact {
	t.Helper()
	for _, a := range r.Artifacts {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("artifact %q not found in %+v", name, r.Artifacts)
	return Artifact{}
}

func TestRun_AllInSync(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	// .env/rules track local sources -> in-sync; binary/plugin track the release -> up-to-date.
	for _, name := range []string{".env", "rules"} {
		if got := find(t, r, name).Status; got != StatusInSync {
			t.Errorf("%s status = %s, want in-sync", name, got)
		}
	}
	for _, name := range []string{"binary", "plugin"} {
		if got := find(t, r, name).Status; got != StatusUpToDate {
			t.Errorf("%s status = %s, want up-to-date", name, got)
		}
	}
	if !r.Consistent || r.Summary.ActionNeeded != 0 {
		t.Errorf("Consistent=%v ActionNeeded=%d, want true/0", r.Consistent, r.Summary.ActionNeeded)
	}
	// All four fold into the "in sync" bucket.
	if r.Summary.UpToDate != 4 {
		t.Errorf("Summary.UpToDate = %d, want 4", r.Summary.UpToDate)
	}
	// No in-sync row should carry a "behind latest" hint when nothing is outdated.
	for _, name := range []string{".env", "rules"} {
		if strings.Contains(find(t, r, name).Detail, "behind") {
			t.Errorf("%s Detail = %q, should not mention 'behind' when upstream is current", name, find(t, r, name).Detail)
		}
	}
}

// lets-kaw72: a partial upgrade (binary 0.5.2, plugin 0.5.3, both behind latest
// 0.5.4) must not produce two bare "up-to-date" rows at different versions. The
// in-sync rows declare what they track AND flag the behind-latest upstream.
func TestRun_PartialUpgrade_InSyncRowsExplained(t *testing.T) {
	pr, plug := scaffold(t, "0.5.2", "0.5.3", "0.5.3", "0.5.2") // env tracks binary 0.5.2; rules tracks plugin 0.5.3
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.5.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	env := find(t, r, ".env")
	if env.Status != StatusInSync || !strings.Contains(env.Detail, "lets binary") || !strings.Contains(env.Detail, "behind latest v0.5.4") {
		t.Fatalf(".env = %+v, want in-sync tracking the binary + behind-latest hint", env)
	}
	rules := find(t, r, "rules")
	if rules.Status != StatusInSync || !strings.Contains(rules.Detail, "plugin") || !strings.Contains(rules.Detail, "behind latest v0.5.4") {
		t.Fatalf("rules = %+v, want in-sync tracking the plugin + behind-latest hint", rules)
	}
	if find(t, r, "binary").Status != StatusOutdated || find(t, r, "plugin").Status != StatusOutdated {
		t.Fatalf("binary/plugin want outdated; got %s/%s", find(t, r, "binary").Status, find(t, r, "plugin").Status)
	}
	if r.Consistent {
		t.Errorf("Consistent = true, want false (binary 0.5.2 != plugin 0.5.3)")
	}
}

// lets-kaw72: when binary == plugin == rules == env but all are behind latest,
// consistent is true (no internal mismatch) yet binary/plugin read outdated. The
// in-sync rows must still flag the behind-latest upstream so the table coheres.
func TestRun_UniformlyBehindLatest(t *testing.T) {
	pr, plug := scaffold(t, "0.5.2", "0.5.2", "0.5.2", "0.5.2")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.5.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Consistent {
		t.Errorf("Consistent = false, want true (everything at 0.5.2)")
	}
	for _, name := range []string{".env", "rules"} {
		a := find(t, r, name)
		if a.Status != StatusInSync || !strings.Contains(a.Detail, "behind latest v0.5.4") {
			t.Errorf("%s = %+v, want in-sync with behind-latest hint", name, a)
		}
	}
	if r.Summary.ActionNeeded != 2 {
		t.Errorf("ActionNeeded = %d, want 2 (binary + plugin)", r.Summary.ActionNeeded)
	}
}

func TestRun_EnvNotInitialized(t *testing.T) {
	pr, plug := scaffold(t, "", "0.6.0", "0.6.0", "0.6.0")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, ".env")
	if a.Status != StatusNotInitialized || a.Action == "" {
		t.Fatalf(".env = %+v, want not-initialized with an action", a)
	}
	if r.Summary.ActionNeeded != 1 {
		t.Errorf("ActionNeeded = %d, want 1", r.Summary.ActionNeeded)
	}
}

func TestRun_RulesOutdatedGetsRewritten(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.5.0", "0.6.0", "0.6.0")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "rules")
	if a.Status != StatusUpdated || a.CurrentVersion != "0.6.0" {
		t.Fatalf("rules = %+v, want updated/0.6.0", a)
	}
	// lets-kaw72: the `updated` row must carry a past-tense detail, never the
	// pre-install imperative ("Run /lets:init / /lets:update").
	if !strings.Contains(a.Detail, "was outdated (v0.5.0)") {
		t.Errorf("rules Detail = %q, want past-tense 'was outdated (v0.5.0)'", a.Detail)
	}
	if strings.Contains(a.Detail, "/lets:") {
		t.Errorf("rules Detail = %q, must not contain an imperative slash-command on an 'updated' row", a.Detail)
	}
	got, _ := os.ReadFile(filepath.Join(pr, ".claude", "rules", "lets-rules.md"))
	if !strings.Contains(string(got), "version: 0.6.0") {
		t.Fatalf("installed rules not rewritten to 0.6.0:\n%s", got)
	}
}

func TestRun_RulesMissingGetsInstalled(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "", "0.6.0", "0.6.0") // no installed rules
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "rules")
	if a.Status != StatusUpdated {
		t.Fatalf("rules status = %s, want updated", a.Status)
	}
	// lets-kaw72: detail says what it was, not "not installed, run /lets:init".
	if a.Detail != "was missing" {
		t.Errorf("rules Detail = %q, want %q", a.Detail, "was missing")
	}
	if _, err := os.Stat(filepath.Join(pr, ".claude", "rules", "lets-rules.md")); err != nil {
		t.Fatalf("rules file not created: %v", err)
	}
}

func TestRun_BinaryOutdated(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.5.0")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "binary")
	if a.Status != StatusOutdated || a.LatestVersion != "0.6.0" || a.Action == "" {
		t.Fatalf("binary = %+v, want outdated/latest 0.6.0/action set", a)
	}
	if r.Consistent {
		t.Errorf("Consistent = true, want false (binary 0.5.0 != plugin 0.6.0)")
	}
}

func TestRun_BinaryAhead(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.7.0")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, r, "binary").Status != StatusAhead {
		t.Fatalf("binary status = %s, want ahead", find(t, r, "binary").Status)
	}
}

func TestRun_Offline(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	r, err := Run(context.Background(), Options{LatestFn: nil}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"binary", "plugin"} {
		a := find(t, r, name)
		if a.Status != StatusUnknown || !strings.Contains(a.Detail, "offline") {
			t.Errorf("%s = %+v, want unknown/offline", name, a)
		}
	}
}

func TestRun_NetworkError(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	failFn := func(context.Context) (LatestInfo, error) { return LatestInfo{}, fmt.Errorf("boom") }
	r, err := Run(context.Background(), Options{LatestFn: failFn}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, r, "binary").Status != StatusUnknown {
		t.Fatalf("binary status = %s, want unknown on network error", find(t, r, "binary").Status)
	}
}

func TestRun_DevBinary(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "dev")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	// A dev binary makes Run skip the .env regen entirely and report it as dev,
	// and short-circuit the binary version check.
	if find(t, r, ".env").Status != StatusDev {
		t.Errorf(".env status = %s, want dev (regen skipped on dev build)", find(t, r, ".env").Status)
	}
	if find(t, r, "binary").Status != StatusDev {
		t.Errorf("binary status = %s, want dev", find(t, r, "binary").Status)
	}
	// "dev" is ignored by the consistency check -> plugin & rules both 0.6.0 -> consistent.
	if !r.Consistent {
		t.Errorf("Consistent = false, want true (dev binary ignored)")
	}
	// No backup file should be left behind (regen was skipped).
	if _, err := os.Stat(filepath.Join(pr, ".lets", ".env.bak")); err == nil {
		t.Errorf(".env.bak exists - regen should have been skipped on a dev build")
	}
}

// Regression: dev-<metadata> stamps produced by scripts/dev/run.sh must travel
// the same dev short-circuits as the bare "dev" sentinel. Pre-fix, versionArtifact
// pattern-matched the literal "dev" and let dev-feat-abc1234 fall through to
// semver.Compare, which returned 0 for invalid semver and silently reported the
// dev binary as "up to date". consistentVersions had the same bug.
func TestRun_DevBinary_RichStamp(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "dev-feat-abc1234")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, r, ".env").Status != StatusDev {
		t.Errorf(".env status = %s, want dev (regen skipped on rich dev stamp)", find(t, r, ".env").Status)
	}
	if find(t, r, "binary").Status != StatusDev {
		t.Errorf("binary status = %s, want dev (rich dev stamp must short-circuit semver compare)", find(t, r, "binary").Status)
	}
	if !r.Consistent {
		t.Errorf("Consistent = false, want true (rich dev stamp must be excluded from comparison set)")
	}
}

// --- lets-rlue4: order-aware deferral + next_action ---

func assertRulesFileVersion(t *testing.T, projectRoot, want string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"))
	if err != nil {
		t.Fatalf("read installed rules: %v", err)
	}
	if !strings.Contains(string(b), "version: "+want) {
		t.Fatalf("installed rules = %q, want it to still carry version: %s", string(b), want)
	}
}

func TestRun_RulesDeferred_BinaryWins(t *testing.T) {
	// binary outdated (0.6.3 < 0.6.4) AND plugin behind (0.6.3 < 0.6.4); installed
	// rules 0.6.2 would normally advance to the stale 0.6.3 - the half-step. Defer.
	pr, plug := scaffold(t, "0.6.3", "0.6.2", "0.6.3", "0.6.3")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusDeferred {
		t.Errorf("rules status = %s, want deferred", got)
	}
	assertRulesFileVersion(t, pr, "0.6.2") // NOT rewritten
	if r.NextAction == nil || r.NextAction.Kind != "binary" {
		t.Fatalf("next_action = %+v, want kind=binary", r.NextAction)
	}
}

func TestRun_RulesDeferred_PluginNext(t *testing.T) {
	// binary current (0.6.4 == latest), plugin behind (0.6.3); rules 0.6.2 -> deferred.
	pr, plug := scaffold(t, "0.6.4", "0.6.2", "0.6.3", "0.6.4")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusDeferred {
		t.Errorf("rules status = %s, want deferred", got)
	}
	assertRulesFileVersion(t, pr, "0.6.2")
	if r.NextAction == nil || r.NextAction.Kind != "plugin" {
		t.Fatalf("next_action = %+v, want kind=plugin", r.NextAction)
	}
}

func TestRun_RulesUpdated_ReloadNext(t *testing.T) {
	// Everything current; rules 0.6.3 -> rewritten to 0.6.4 -> reload.
	pr, plug := scaffold(t, "0.6.4", "0.6.3", "0.6.4", "0.6.4")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusUpdated {
		t.Errorf("rules status = %s, want updated", got)
	}
	assertRulesFileVersion(t, pr, "0.6.4")
	if r.NextAction == nil || r.NextAction.Kind != "reload" {
		t.Fatalf("next_action = %+v, want kind=reload", r.NextAction)
	}
}

func TestRun_AllSynced_DoneNext(t *testing.T) {
	pr, plug := scaffold(t, "0.6.4", "0.6.4", "0.6.4", "0.6.4")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if r.NextAction == nil || r.NextAction.Kind != "done" || r.NextAction.Version != "0.6.4" {
		t.Fatalf("next_action = %+v, want done/0.6.4", r.NextAction)
	}
}

func TestRun_OfflineLocalBehind_Defers(t *testing.T) {
	// Offline: plugin/binary status unknown, but plugin 0.6.3 < binary 0.6.4
	// locally -> defer via the local-compare leg; next action steers to plugin.
	pr, plug := scaffold(t, "0.6.4", "0.6.2", "0.6.3", "0.6.4")
	r, err := Run(context.Background(), Options{LatestFn: nil}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusDeferred {
		t.Errorf("rules status = %s, want deferred (offline local-behind)", got)
	}
	assertRulesFileVersion(t, pr, "0.6.2")
	if r.NextAction == nil || r.NextAction.Kind != "plugin" {
		t.Fatalf("next_action = %+v, want kind=plugin", r.NextAction)
	}
}

func TestRun_OfflineNoLocalBehind_NoDefer(t *testing.T) {
	// Offline, plugin == binary (no local-behind): defer must NOT fire on
	// uncertainty - rules sync normally.
	pr, plug := scaffold(t, "0.6.3", "0.6.2", "0.6.3", "0.6.3")
	r, err := Run(context.Background(), Options{LatestFn: nil}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusUpdated {
		t.Errorf("rules status = %s, want updated (no defer when plugin==binary offline)", got)
	}
	assertRulesFileVersion(t, pr, "0.6.3")
}

func TestRun_DevBinary_PluginOutdated_Defers(t *testing.T) {
	// A dev binary is excluded from the local compare, but a genuinely outdated
	// plugin still defers (via the plugin-outdated leg); the dev binary neither
	// triggers nor suppresses the defer.
	pr, plug := scaffold(t, "0.6.3", "0.6.2", "0.6.3", "dev")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusDeferred {
		t.Errorf("rules status = %s, want deferred (plugin outdated, dev binary)", got)
	}
	if r.NextAction == nil || r.NextAction.Kind != "plugin" {
		t.Fatalf("next_action = %+v, want kind=plugin", r.NextAction)
	}
}

func TestRun_RulesAhead_NotDeferred(t *testing.T) {
	// Installed rules AHEAD of a behind-plugin: only StateOutdated defers, so the
	// existing downgrade-on-ahead behavior is preserved (file reset to plugin).
	pr, plug := scaffold(t, "0.6.4", "0.6.5", "0.6.3", "0.6.4")
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "rules").Status; got != StatusUpdated {
		t.Errorf("rules status = %s, want updated (ahead is not deferred)", got)
	}
	assertRulesFileVersion(t, pr, "0.6.3") // downgraded to plugin, as before
}

func TestRun_UserRulesDeferred(t *testing.T) {
	// user-rules block mirrors the project block: a behind plugin defers the
	// global rules sync too, leaving the file untouched.
	projectRoot, pluginRoot := scaffold(t, "0.6.4", "0.6.3", "0.6.3", "0.6.4")
	home := userHome(t, "0.6.2")
	userPath := filepath.Join(home, ".claude", "rules", "lets-rules.md")
	before, _ := os.ReadFile(userPath)
	r, err := Run(context.Background(), Options{HomeDir: home, LatestFn: stubLatest("0.6.4")}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(t, r, "user-rules").Status; got != StatusDeferred {
		t.Errorf("user-rules status = %s, want deferred", got)
	}
	after, _ := os.ReadFile(userPath)
	if string(before) != string(after) {
		t.Error("global rules written despite deferral")
	}
}

func TestNextActionCommand_ConstOnly(t *testing.T) {
	// SECURITY: next_action.Command must be byte-equal to the installScriptCmd
	// const for any binary-outdated input - never interpolated with versions.
	for _, bin := range []string{"0.5.0", "0.1.2", "0.6.3"} {
		pr, plug := scaffold(t, bin, "0.6.4", "0.6.4", bin)
		r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.4")}, pr, plug)
		if err != nil {
			t.Fatal(err)
		}
		if r.NextAction == nil || r.NextAction.Kind != "binary" {
			t.Fatalf("binary %s: next_action = %+v, want kind=binary", bin, r.NextAction)
		}
		if r.NextAction.Command != installScriptCmd {
			t.Fatalf("binary %s: Command = %q, want byte-equal installScriptCmd", bin, r.NextAction.Command)
		}
	}
}

func TestRun_OfflineDevBinary_DoneNoVersion(t *testing.T) {
	// Offline + dev binary, everything else in sync: done with NO version (we
	// never verified the latest release) and a hedged message, not "on the latest".
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "dev")
	r, err := Run(context.Background(), Options{LatestFn: nil}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if r.NextAction == nil || r.NextAction.Kind != "done" {
		t.Fatalf("next_action = %+v, want kind=done", r.NextAction)
	}
	if r.NextAction.Version != "" {
		t.Errorf("done Version = %q, want empty (offline+dev, latest unverified)", r.NextAction.Version)
	}
	if !strings.Contains(r.NextAction.Message, "couldn't verify") {
		t.Errorf("done Message = %q, want the hedged offline message", r.NextAction.Message)
	}
}

func TestComputeNextAction_NotInitializedRulesRoutesToInit(t *testing.T) {
	// A not-initialized rules row (scope=user, no global) must drive next_action,
	// not fall through to `done` while sitting in ActionNeeded.
	r := NewResult("/p", "/plug")
	r.Add(Artifact{Name: ".env", Status: StatusInSync, CurrentVersion: "0.6.0"})
	r.Add(Artifact{Name: "binary", Status: StatusUpToDate, CurrentVersion: "0.6.0", LatestVersion: "0.6.0"})
	r.Add(Artifact{Name: "plugin", Status: StatusUpToDate, CurrentVersion: "0.6.0", LatestVersion: "0.6.0"})
	r.Add(Artifact{Name: "rules", Status: StatusNotInitialized, Action: "Run `lets init --user` to restore the global rules"})
	computeNextAction(&r, "0.6.0", LatestInfo{Version: "0.6.0"})
	if r.NextAction == nil || r.NextAction.Kind != "init" {
		t.Fatalf("next_action = %+v, want kind=init", r.NextAction)
	}
	if !strings.Contains(r.NextAction.Message, "lets init --user") {
		t.Errorf("message = %q, want the rules row's Action carried through", r.NextAction.Message)
	}
}

func TestRun_EnvHeaderRefreshed(t *testing.T) {
	pr, plug := scaffold(t, "0.5.0", "0.6.0", "0.6.0", "0.6.0") // .env behind the (real) binary
	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, ".env")
	if a.Status != StatusUpdated || a.CurrentVersion != "0.6.0" {
		t.Fatalf(".env = %+v, want updated/0.6.0", a)
	}
	if !strings.Contains(a.Detail, "v0.5.0") || !strings.Contains(a.Detail, "backup") {
		t.Fatalf(".env Detail = %q, want it to mention the old version and the backup", a.Detail)
	}
	if _, err := os.Stat(filepath.Join(pr, ".lets", ".env.bak")); err != nil {
		t.Fatalf(".env.bak not written: %v", err)
	}
}

func TestRun_BinaryUpToDate_FromCache(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	r, err := Run(context.Background(), Options{LatestFn: stubCached("0.6.0", 90*time.Minute)}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "binary")
	if a.Status != StatusUpToDate {
		t.Fatalf("binary status = %s, want up-to-date", a.Status)
	}
	if !strings.Contains(a.Detail, "checked") || !strings.Contains(a.Detail, "ago") || !strings.Contains(a.Detail, "1h") {
		t.Fatalf("binary Detail = %q, want a 'checked 1h ago'-style provenance note", a.Detail)
	}
}

func TestRun_BinaryAhead_FromCache(t *testing.T) {
	pr, plug := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.7.0") // binary ahead of latest
	r, err := Run(context.Background(), Options{LatestFn: stubCached("0.6.0", 30*time.Minute)}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "binary")
	if a.Status != StatusAhead {
		t.Fatalf("binary status = %s, want ahead", a.Status)
	}
	// Detail joins the cache-provenance note with the "newer than latest" note.
	if !strings.Contains(a.Detail, "checked") || !strings.Contains(a.Detail, "; ") || !strings.Contains(a.Detail, "newer than") {
		t.Fatalf("binary Detail = %q, want '<provenance>; newer than ...'", a.Detail)
	}
}

func TestDurationApprox(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{-5 * time.Minute, "<1m"},
		{30 * time.Second, "<1m"},
		{5 * time.Minute, "5m"},
		{59 * time.Minute, "59m"},
		{90 * time.Minute, "1h"},
		{23 * time.Hour, "23h"},
		{49 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := durationApprox(c.d); got != c.want {
			t.Errorf("durationApprox(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestRun_PluginVersionUnreadable(t *testing.T) {
	pr := t.TempDir()
	plug := t.TempDir() // no .claude-plugin/plugin.json, no rules/
	setVersion(t, "0.6.0")
	envDir := filepath.Join(pr, ".lets")
	_ = os.MkdirAll(envDir, 0o755)
	_ = os.WriteFile(filepath.Join(envDir, ".env"), []byte("LETS_ENV_VERSION=0.6.0\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"), 0o644)

	r, err := Run(context.Background(), Options{LatestFn: stubLatest("0.6.0")}, pr, plug)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, r, "plugin").Status != StatusUnknown {
		t.Errorf("plugin status = %s, want unknown", find(t, r, "plugin").Status)
	}
	if find(t, r, "rules").Status != StatusUnknown {
		t.Errorf("rules status = %s, want unknown (plugin rules unreadable)", find(t, r, "rules").Status)
	}
}

func TestPrintReport_DoneState(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Add(Artifact{Name: ".env", Status: StatusUpToDate, CurrentVersion: "0.6.0"})
	r.Add(Artifact{Name: "binary", Status: StatusUpToDate, CurrentVersion: "0.6.0", LatestVersion: "0.6.0"})
	r.NextAction = &NextAction{Kind: "done", Version: "0.6.0"}
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	if !strings.Contains(out, "LETS Update Status") || !strings.Contains(out, "Everything on v0.6.0.") {
		t.Fatalf("unexpected report:\n%s", out)
	}
}

func TestPrintReport_DoneStateNoVersion(t *testing.T) {
	// Empty Version (offline / unverified) renders the hedged line, never a version.
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Add(Artifact{Name: "binary", Status: StatusDev, CurrentVersion: "dev"})
	r.NextAction = &NextAction{Kind: "done", Version: ""}
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	if !strings.Contains(out, "couldn't verify the latest release") {
		t.Fatalf("empty-Version done should hedge:\n%s", out)
	}
	if strings.Contains(out, "Everything on v") {
		t.Errorf("must not claim a version when Version is empty:\n%s", out)
	}
}

func TestPrintReport_SingleNextAction(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Add(Artifact{Name: ".env", Status: StatusUpToDate, CurrentVersion: "0.6.0"})
	r.Add(Artifact{Name: "binary", Status: StatusOutdated, CurrentVersion: "0.5.0", LatestVersion: "0.6.0", Action: "do the thing"})
	r.NextAction = &NextAction{Kind: "binary", Message: "Update the lets binary (v0.5.0 -> v0.6.0).", Command: installScriptCmd}
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	// One next action - NOT the old per-artifact "N of M need your action" list.
	if !strings.Contains(out, "Next: Update the lets binary") || !strings.Contains(out, installScriptCmd) {
		t.Fatalf("PrintReport missing single next action:\n%s", out)
	}
	if strings.Contains(out, "need your action") || strings.Contains(out, "binary: do the thing") {
		t.Fatalf("PrintReport must not render the old per-artifact action list:\n%s", out)
	}
}

func TestPrintReport_UnknownVersionShowsQuestionMark(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.Add(Artifact{Name: "plugin", Status: StatusUnknown})
	var b bytes.Buffer
	PrintReport(&b, r)
	if strings.Contains(b.String(), "v?") {
		t.Fatalf("should render '?' not 'v?' for an unknown version:\n%s", b.String())
	}
}

// lets-kaw72: the rendered text table for a partial upgrade must read coherently
// - in-sync rows naming what they track + the behind-latest hint, plus the
// inconsistency warning - never two bare "up-to-date" rows at different versions.
func TestPrintReport_PartialUpgradeReadable(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Consistent = false
	r.Add(Artifact{Name: ".env", Status: StatusInSync, CurrentVersion: "0.5.2", Detail: "tracks the lets binary (itself behind latest v0.5.4)"})
	r.Add(Artifact{Name: "rules", Status: StatusInSync, CurrentVersion: "0.5.3", Detail: "tracks the plugin (itself behind latest v0.5.4)"})
	r.Add(Artifact{Name: "binary", Status: StatusOutdated, CurrentVersion: "0.5.2", LatestVersion: "0.5.4", Action: "update binary"})
	r.Add(Artifact{Name: "plugin", Status: StatusOutdated, CurrentVersion: "0.5.3", LatestVersion: "0.5.4", Action: "update plugin"})
	r.NextAction = &NextAction{Kind: "binary", Message: "Update the lets binary (v0.5.2 -> v0.5.4).", Command: installScriptCmd}
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	// Table still reads coherently; the single next action replaces the old list.
	for _, want := range []string{
		"in-sync", "tracks the lets binary", "itself behind latest v0.5.4",
		"Next: Update the lets binary",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "up-to-date") {
		t.Errorf("partial-upgrade report should not show bare 'up-to-date' rows:\n%s", out)
	}
	// The inconsistent-install warning is suppressed when the single next action
	// (binary/plugin) already explains the partial state.
	if strings.Contains(out, "inconsistent") {
		t.Errorf("inconsistent warning should be suppressed on a binary/plugin next action:\n%s", out)
	}
}

// The inconsistent-install warning still shows on the reload/done tail (where a
// version mismatch would otherwise be a surprise), only suppressed on binary/plugin.
func TestPrintReport_InconsistentWarning(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.Consistent = false
	r.Add(Artifact{Name: "rules", Status: StatusUpdated, CurrentVersion: "0.6.0"})
	r.NextAction = &NextAction{Kind: "reload", Message: "Restart Claude Code so the updated rules load - /exit, then reopen."}
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	if !strings.Contains(out, "inconsistent") {
		t.Fatalf("missing inconsistency warning on reload tail:\n%s", out)
	}
	if !strings.Contains(out, "Next: Restart Claude Code") {
		t.Fatalf("missing reload next action:\n%s", out)
	}
}

// --- user-rules artifact (lets-wug9k) ---

// userHome creates a fake home; rulesVer != "" writes
// ~/.claude/rules/lets-rules.md at that version ("MALFORMED" = no frontmatter).
func userHome(t *testing.T, rulesVer string) string {
	t.Helper()
	home := t.TempDir()
	switch rulesVer {
	case "":
	case "MALFORMED":
		p := filepath.Join(home, ".claude", "rules", "lets-rules.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# no frontmatter\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	default:
		rulesFile(t, filepath.Join(home, ".claude", "rules", "lets-rules.md"), rulesVer)
	}
	return home
}

// findMaybe returns the artifact by name or nil (find() fatals on absence).
func findMaybe(r Result, name string) *Artifact {
	for i := range r.Artifacts {
		if r.Artifacts[i].Name == name {
			return &r.Artifacts[i]
		}
	}
	return nil
}

func TestRun_UserRulesAbsent_NoArtifact(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "")
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Artifacts) != 4 {
		t.Fatalf("expected 4 artifacts when user scope absent, got %d", len(r.Artifacts))
	}
	if findMaybe(r, "user-rules") != nil {
		t.Error("user-rules row must be omitted when the file is absent")
	}
	// Negative assertion - the actual bug-catcher: update never bootstraps
	// user scope (that's `lets init --user`'s job).
	if _, err := os.Stat(filepath.Join(home, ".claude", "rules")); err == nil {
		t.Error("update must NOT create ~/.claude/rules")
	}
}

func TestRun_UserRulesInSync(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "0.6.0")
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Artifacts) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(r.Artifacts))
	}
	a := find(t, r, "user-rules")
	if a.Status != StatusInSync || a.Detail != "tracks the plugin" {
		t.Errorf("user-rules: %+v", a)
	}
}

func TestRun_UserRulesOutdated_Rewritten(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "0.5.0")
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "user-rules")
	if a.Status != StatusUpdated || a.CurrentVersion != "0.6.0" {
		t.Errorf("user-rules: %+v", a)
	}
	if !strings.Contains(a.Detail, "was outdated (v0.5.0)") {
		t.Errorf("detail: %q", a.Detail)
	}
}

func TestRun_UserRulesAhead_NotClobbered(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "9.9.9")
	userPath := filepath.Join(home, ".claude", "rules", "lets-rules.md")
	before, _ := os.ReadFile(userPath)
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(userPath)
	if string(before) != string(after) {
		t.Error("ahead global rules were clobbered by update")
	}
	a := find(t, r, "user-rules")
	if a.Status != StatusAhead {
		t.Errorf("status: got %s want ahead", a.Status)
	}
	if !strings.Contains(a.Detail, "not overwritten") {
		t.Errorf("detail: %q", a.Detail)
	}
	// Pins the DELIBERATE exclusion of user-rules from the consistency set:
	// a customized global copy is not a "partial upgrade".
	if !r.Consistent {
		t.Error("ahead user-rules must NOT trip the consistency warning")
	}
}

func TestRun_UserRulesMalformed_Rewritten(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "MALFORMED")
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "user-rules")
	if a.Status != StatusUpdated || a.Detail != "was unparseable" {
		t.Errorf("user-rules: %+v", a)
	}
}

// A broken plugin payload must not crash or clobber the global copy.
func TestRun_UserRulesPluginUnreadable_NoClobber(t *testing.T) {
	projectRoot := t.TempDir()
	pluginRoot := t.TempDir() // no rules/lets-rules.md inside
	setVersion(t, "0.6.0")
	writePluginJSON(t, pluginRoot, `{"name":"lets","version":"0.6.0"}`)
	home := userHome(t, "0.6.0")
	before, _ := os.ReadFile(filepath.Join(home, ".claude", "rules", "lets-rules.md"))

	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "user-rules")
	if a.Status != StatusUnknown || !strings.Contains(a.Detail, "plugin rules unreadable") {
		t.Errorf("user-rules: %+v", a)
	}
	after, _ := os.ReadFile(filepath.Join(home, ".claude", "rules", "lets-rules.md"))
	if string(before) != string(after) {
		t.Error("global rules touched despite unreadable plugin payload")
	}
}

// in-sync user-rules whose upstream (plugin) is behind latest gets the
// annotateInSyncBehind hint, same as project rules.
func TestRun_UserRulesInSync_AnnotatedWhenPluginOutdated(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "0.6.0")
	r, err := Run(context.Background(), Options{HomeDir: home, LatestFn: stubLatest("0.7.0")}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, "user-rules")
	if !strings.Contains(a.Detail, "itself behind latest v0.7.0") {
		t.Errorf("missing behind-latest hint: %q", a.Detail)
	}
}

func TestPrintReport_UserRulesRow(t *testing.T) {
	projectRoot, pluginRoot := scaffold(t, "0.6.0", "0.6.0", "0.6.0", "0.6.0")
	home := userHome(t, "0.6.0")
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	PrintReport(&buf, r)
	if !strings.Contains(buf.String(), "user-rules") {
		t.Errorf("report missing user-rules row:\n%s", buf.String())
	}
}

// --- LETS_RULES_SCOPE matrix (lets-wug9k) ---

// scopeFixture builds a project (.env carrying `scope`, optional project rules
// copy at projVer) + a fake home (optional global copy at globalVer). Plugin is
// always 0.6.0; binary set to dev so the binary/plugin rows don't interfere.
func scopeFixture(t *testing.T, scope, projVer, globalVer string) (projectRoot, pluginRoot, home string) {
	t.Helper()
	projectRoot = t.TempDir()
	pluginRoot = t.TempDir()
	setVersion(t, "dev")
	writePluginJSON(t, pluginRoot, `{"name":"lets","version":"0.6.0"}`)
	rulesFile(t, filepath.Join(pluginRoot, "rules", "lets-rules.md"), "0.6.0")

	envDir := filepath.Join(projectRoot, ".lets")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "LETS_ENV_VERSION=dev\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\nLETS_LAUNCHER=terminal\n"
	if scope != "" {
		body += "LETS_RULES_SCOPE=" + scope + "\n"
	}
	if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if projVer != "" {
		rulesFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"), projVer)
	}
	home = userHome(t, globalVer)
	return projectRoot, pluginRoot, home
}

func TestRun_RulesScopeMatrix(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		projVer     string // "" = project copy absent
		globalVer   string // "" = global absent
		wantStatus  ArtifactStatus
		wantDetail  string // substring; "" = no check
		fileWritten bool   // must the project copy exist after?
	}{
		{"unset_present_regression", "", "0.6.0", "", StatusInSync, "tracks the plugin", true},
		{"user_absent_global_present_delegated", "user", "", "0.6.0", StatusDelegated, "global copy", false},
		{"user_absent_global_absent_notinit", "user", "", "", StatusNotInitialized, "no rules anywhere", false},
		{"user_present_current_global_present_dup", "user", "0.6.0", "0.6.0", StatusInSync, "duplication", true},
		{"user_present_outdated_global_present_dup", "user", "0.5.0", "0.6.0", StatusUpdated, "duplication", true},
		{"user_present_current_global_absent_hint", "user", "0.6.0", "", StatusInSync, "only the project copy", true},
		{"banana_absent_global_present_failsafe", "banana", "", "0.6.0", StatusUpdated, "", true},
		{"project_present_current_global_present_nohint", "project", "0.6.0", "0.6.0", StatusInSync, "tracks the plugin", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot, pluginRoot, home := scopeFixture(t, tt.scope, tt.projVer, tt.globalVer)
			r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
			if err != nil {
				t.Fatal(err)
			}
			a := find(t, r, "rules")
			if a.Status != tt.wantStatus {
				t.Errorf("status: got %s want %s", a.Status, tt.wantStatus)
			}
			if tt.wantDetail != "" && !strings.Contains(a.Detail, tt.wantDetail) {
				t.Errorf("detail: got %q want substring %q", a.Detail, tt.wantDetail)
			}
			_, statErr := os.Stat(filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"))
			if tt.fileWritten && statErr != nil {
				t.Errorf("project copy should exist: %v", statErr)
			}
			if !tt.fileWritten && statErr == nil {
				t.Error("project copy should NOT exist (delegated must never re-create it)")
			}
			// project=project (and unset) must never carry a duplication hint.
			if (tt.scope == "" || tt.scope == "project") && strings.Contains(a.Detail, "duplication") {
				t.Errorf("scope=%q must have no duplication hint: %q", tt.scope, a.Detail)
			}
		})
	}
}

// scope=user carried ONLY in ~/.lets/.env (not the project .env) still engages
// delegated - pins the merged read so hook and update agree.
func TestRun_RulesScopeFromUserEnv(t *testing.T) {
	projectRoot, pluginRoot, home := scopeFixture(t, "", "", "0.6.0")
	// scopeFixture/userHome create ~/.claude/rules but not ~/.lets - make it first.
	if err := os.MkdirAll(filepath.Join(home, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".lets", ".env"), []byte("LETS_RULES_SCOPE=user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	if a := find(t, r, "rules"); a.Status != StatusDelegated {
		t.Errorf("scope from ~/.lets/.env must engage delegated, got %s", a.Status)
	}
}

// Malformed project .env degrades to project semantics (no crash, copy synced).
func TestRun_RulesScopeMalformedEnv(t *testing.T) {
	projectRoot, pluginRoot, home := scopeFixture(t, "project", "0.6.0", "")
	// Overwrite .env with garbage that envfile.Parse may choke on.
	if err := os.WriteFile(filepath.Join(projectRoot, ".lets", ".env"), []byte("\x00not=valid\nbroken"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Run(context.Background(), Options{HomeDir: home}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatalf("malformed .env must not crash: %v", err)
	}
	// rules row must exist and not be delegated.
	if a := find(t, r, "rules"); a.Status == StatusDelegated {
		t.Error("malformed .env must degrade to project, not delegated")
	}
}

// The one-time migration: an initialized project at the current version whose
// .env predates the 6th key reports `.env updated (1 key changed)` once.
func TestRun_RulesScopeKeyMigration(t *testing.T) {
	projectRoot := t.TempDir()
	pluginRoot := t.TempDir()
	setVersion(t, "0.6.0")
	writePluginJSON(t, pluginRoot, `{"name":"lets","version":"0.6.0"}`)
	rulesFile(t, filepath.Join(pluginRoot, "rules", "lets-rules.md"), "0.6.0")
	rulesFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"), "0.6.0")
	envDir := filepath.Join(projectRoot, ".lets")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// At-current version .env but WITHOUT LETS_RULES_SCOPE (a pre-6th-key file).
	old := "LETS_ENV_VERSION=0.6.0\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\nLETS_LAUNCHER=terminal\n"
	if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Run(context.Background(), Options{}, projectRoot, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, r, ".env")
	if a.Status != StatusUpdated || !strings.Contains(a.Detail, "1 key(s) changed") {
		t.Errorf(".env migration: got %+v, want updated + 1 key changed", a)
	}
	data, _ := os.ReadFile(filepath.Join(envDir, ".env"))
	if !strings.Contains(string(data), "LETS_RULES_SCOPE=project") {
		t.Errorf("migration must add LETS_RULES_SCOPE=project:\n%s", data)
	}
}
