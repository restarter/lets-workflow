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

func TestRun_FullOutput(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "# Rules\nrule body\n")
	writeFile(t, filepath.Join(dir, ".lets", ".env"),
		"# Comment\nLETS_LANGUAGE=Ukrainian\nLETS_MERGE_BRANCH=develop\nLETS_PR_FLOW=github\nLETS_TRACKER=beads\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, dir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := buf.String()
	want := "# Rules\nrule body\n\n## LETS Config\n\n" +
		"LETS_PROJECT_ROOT=" + dir + "\n" +
		"LETS_LANGUAGE=Ukrainian\n" +
		"LETS_MERGE_BRANCH=develop\n" +
		"LETS_PR_FLOW=github\n" +
		"LETS_TRACKER=beads\n"
	if got != want {
		t.Errorf("output mismatch:\nGOT:\n%s\nWANT:\n%s", got, want)
	}
}

func TestRun_NoProjectRoot_SuppressesConfig(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "rules body\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := buf.String(); got != "rules body\n" {
		t.Errorf("expected only rules body, got %q", got)
	}
}

func TestRun_NoEnvFile_OnlyProjectRoot(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "rules\n")

	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, dir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := "rules\n\n## LETS Config\n\nLETS_PROJECT_ROOT=" + dir + "\n"
	if got := buf.String(); got != want {
		t.Errorf("output mismatch:\nGOT:\n%s\nWANT:\n%s", got, want)
	}
}

func TestRun_EmptyEnvValues_Skipped(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "rules\n")
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
	if strings.Contains(got, "LETS_LANGUAGE=") {
		t.Errorf("empty LANGUAGE should be skipped, got %q", got)
	}
}

func TestRun_RulesFileMissing_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	err := sessionstart.Run(&buf, filepath.Join(dir, "nonexistent.md"), dir)
	if err == nil {
		t.Fatal("expected error for missing rules file")
	}
	if !strings.Contains(err.Error(), "read rules file") {
		t.Errorf("error should mention rules file, got: %v", err)
	}
}

func TestRun_NonWhitelistedKeys_Ignored(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	writeFile(t, rulesPath, "rules\n")
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
