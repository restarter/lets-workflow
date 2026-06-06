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
		body := "# LETS plugin config\nLETS_ENV_VERSION=" + envVer + "\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"
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

func TestPrintReport_AllInSync(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Add(Artifact{Name: ".env", Status: StatusUpToDate, CurrentVersion: "0.6.0"})
	r.Add(Artifact{Name: "binary", Status: StatusUpToDate, CurrentVersion: "0.6.0", LatestVersion: "0.6.0"})
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	if !strings.Contains(out, "LETS Update Status") || !strings.Contains(out, "All 2 artifacts in sync.") {
		t.Fatalf("unexpected report:\n%s", out)
	}
}

func TestPrintReport_ActionsListed(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Add(Artifact{Name: ".env", Status: StatusUpToDate, CurrentVersion: "0.6.0"})
	r.Add(Artifact{Name: "binary", Status: StatusOutdated, CurrentVersion: "0.5.0", LatestVersion: "0.6.0", Action: "do the thing"})
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	if !strings.Contains(out, "1 of 2 artifacts need your action") || !strings.Contains(out, "binary: do the thing") {
		t.Fatalf("PrintReport output missing action section:\n%s", out)
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
	var b bytes.Buffer
	PrintReport(&b, r)
	out := b.String()
	for _, want := range []string{
		"in-sync", "tracks the lets binary", "itself behind latest v0.5.4",
		"inconsistent", "2 of 4 artifacts need your action",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "up-to-date") {
		t.Errorf("partial-upgrade report should not show bare 'up-to-date' rows:\n%s", out)
	}
}

func TestPrintReport_InconsistentWarning(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.Consistent = false
	r.Add(Artifact{Name: "binary", Status: StatusOutdated, CurrentVersion: "0.5.0", Action: "x"})
	var b bytes.Buffer
	PrintReport(&b, r)
	if !strings.Contains(b.String(), "inconsistent") {
		t.Fatalf("missing inconsistency warning:\n%s", b.String())
	}
}
