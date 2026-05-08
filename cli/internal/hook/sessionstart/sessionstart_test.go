package sessionstart_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRun_NoProjectRoot_SuppressesEverything(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "---\nversion: 0.4.0\n---\n# Rules\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := buf.String(); got != "" {
		t.Errorf("expected empty output (no project root), got %q", got)
	}
}

func TestRun_BasicConfigBlock(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "---\nversion: 0.4.0\n---\n# Rules\n")
	// Pre-install matching rules so no drift notice
	writeFile(t, filepath.Join(dir, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.4.0\n---\n# Installed\n")
	writeFile(t, filepath.Join(dir, ".lets", ".env"),
		"# Comment\nLETS_LANGUAGE=Ukrainian\nLETS_MERGE_BRANCH=develop\nLETS_PR_FLOW=github\nLETS_TRACKER=beads\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, dir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := "## LETS Config\n\n" +
		"LETS_PROJECT_ROOT=" + dir + "\n" +
		"LETS_LANGUAGE=Ukrainian\n" +
		"LETS_MERGE_BRANCH=develop\n" +
		"LETS_PR_FLOW=github\n" +
		"LETS_TRACKER=beads\n"
	if got := buf.String(); got != want {
		t.Errorf("output mismatch:\nGOT:\n%s\nWANT:\n%s", got, want)
	}
}

func TestRun_DriftMissing(t *testing.T) {
	tmp := t.TempDir()
	pluginRules := filepath.Join(tmp, "plugin-rules.md")
	writeFile(t, pluginRules, "---\nversion: 0.4.0\n---\n# Rules\n")

	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".lets", ".env"), "LETS_LANGUAGE=English\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, pluginRules, projectRoot); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "rules not installed") {
		t.Errorf("missing 'rules not installed' notice:\n%s", out)
	}
	if !strings.Contains(out, "LETS_LANGUAGE=English") {
		t.Errorf("missing config block")
	}
}

func TestRun_DriftOutdated(t *testing.T) {
	tmp := t.TempDir()
	pluginRules := filepath.Join(tmp, "plugin-rules.md")
	writeFile(t, pluginRules, "---\nversion: 0.4.0\n---\n# Rules\n")

	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.3.0\n---\n# old\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, pluginRules, projectRoot); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "outdated (installed v0.3.0 < plugin v0.4.0)") {
		t.Errorf("missing 'outdated' notice:\n%s", out)
	}
}

func TestRun_DriftSilent_WhenSameVersion(t *testing.T) {
	tmp := t.TempDir()
	pluginRules := filepath.Join(tmp, "plugin-rules.md")
	writeFile(t, pluginRules, "---\nversion: 0.4.0\n---\n")

	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.4.0\n---\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, pluginRules, projectRoot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "LETS Notice") {
		t.Errorf("unexpected drift notice when versions match:\n%s", buf.String())
	}
}

// Installed > plugin: shouldn't happen in normal flow, but should NOT spam a notice.
func TestRun_DriftSilent_WhenInstalledAhead(t *testing.T) {
	tmp := t.TempDir()
	pluginRules := filepath.Join(tmp, "plugin-rules.md")
	writeFile(t, pluginRules, "---\nversion: 0.4.0\n---\n")

	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 99.0.0\n---\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, pluginRules, projectRoot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "LETS Notice") {
		t.Errorf("unexpected drift notice when installed > plugin:\n%s", buf.String())
	}
}

func TestRun_DriftMalformedSemver_TreatsAsUnknown(t *testing.T) {
	tmp := t.TempDir()
	pluginRules := filepath.Join(tmp, "plugin-rules.md")
	writeFile(t, pluginRules, "---\nversion: 0.4.0\n---\n")

	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"),
		"---\nversion: not-a-version\n---\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, pluginRules, projectRoot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "version unknown") {
		t.Errorf("expected 'version unknown' notice for malformed semver:\n%s", buf.String())
	}
}

func TestRun_PluginRulesMissing_NoNotice(t *testing.T) {
	// If plugin rules path is broken, hook can't compare - silent.
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".lets", ".env"), "LETS_LANGUAGE=English\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, "/nonexistent/plugin-rules.md", projectRoot); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "LETS Notice") {
		t.Errorf("plugin rules missing should be silent, got notice:\n%s", out)
	}
	if !strings.Contains(out, "LETS_LANGUAGE=English") {
		t.Errorf("config block missing")
	}
}

func TestRun_OutputUnderSizeBudget(t *testing.T) {
	tmp := t.TempDir()
	pluginRules := filepath.Join(tmp, "plugin-rules.md")
	writeFile(t, pluginRules, "---\nversion: 0.4.0\n---\n")
	projectRoot := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.4.0\n---\n")
	writeFile(t, filepath.Join(projectRoot, ".lets", ".env"),
		"LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, pluginRules, projectRoot); err != nil {
		t.Fatal(err)
	}
	if buf.Len() >= 9000 {
		t.Errorf("hook output %d bytes - approaching 10K cap. Trim emission.", buf.Len())
	}
}

func TestRun_EmptyEnvValues_Skipped(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "---\nversion: 0.4.0\n---\n")
	writeFile(t, filepath.Join(dir, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.4.0\n---\n")
	writeFile(t, filepath.Join(dir, ".lets", ".env"),
		"LETS_LANGUAGE=\nLETS_MERGE_BRANCH=main\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, dir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "LETS_MERGE_BRANCH=main\n") {
		t.Errorf("expected MERGE_BRANCH line, got %q", got)
	}
	if strings.Contains(got, "LETS_LANGUAGE=\n") {
		t.Errorf("empty LANGUAGE should be skipped, got %q", got)
	}
}

func TestRun_NonWhitelistedKeys_Ignored(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "---\nversion: 0.4.0\n---\n")
	writeFile(t, filepath.Join(dir, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.4.0\n---\n")
	writeFile(t, filepath.Join(dir, ".lets", ".env"),
		"LETS_LANGUAGE=English\nMALICIOUS_KEY=evil\nGITHUB_TOKEN=secret\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, dir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := buf.String()
	if strings.Contains(got, "MALICIOUS_KEY") || strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("non-whitelisted keys leaked into output: %s", got)
	}
	if !strings.Contains(got, "LETS_LANGUAGE=English") {
		t.Errorf("whitelisted key missing: %s", got)
	}
}
