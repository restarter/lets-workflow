package initcmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/drift"
)

func TestRun_FreshProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)

	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", SkipBeads: true}
	result, err := Run(context.Background(), prefs, tmp, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}

	wantPaths := []string{
		".lets/sessions",
		".lets/.env",
		".lets/.env.example",
		".gitignore",
		".claude/settings.json",
		".claude/rules/lets-rules.md",
	}
	for _, p := range wantPaths {
		if _, err := os.Stat(filepath.Join(tmp, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}

	envData, _ := os.ReadFile(filepath.Join(tmp, ".lets", ".env"))
	if !strings.Contains(string(envData), "LETS_LANGUAGE=English") {
		t.Errorf(".env content wrong:\n%s", envData)
	}

	// Tighten .env.example assertion: bytes-equal vs renderEnvExample() catches
	// regressions where Step 6 writes wrong path or empty/stale content.
	gotExample, err := os.ReadFile(filepath.Join(tmp, ".lets", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	if !bytes.Equal(gotExample, renderEnvExample()) {
		t.Errorf(".env.example content != renderEnvExample() output\n--- got ---\n%s\n--- want ---\n%s", gotExample, renderEnvExample())
	}

	if len(result.Steps) < 5 {
		t.Errorf("steps = %d, want >=5", len(result.Steps))
	}
}

func TestRun_Idempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", SkipBeads: true}

	if _, err := Run(context.Background(), prefs, tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), prefs, tmp, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	skipCount := 0
	envSkipped := false
	for _, s := range result.Steps {
		if s.Status == StepSkip {
			skipCount++
			if strings.Contains(s.Message, ".lets/.env") {
				envSkipped = true
			}
		}
	}
	if skipCount < 2 {
		t.Errorf("second run had %d skips, want >=2", skipCount)
	}
	if !envSkipped {
		t.Error("second run did not skip .env step (regression: re-run rewrites .env unconditionally)")
	}
}

func TestRun_DriftRecomputedAfterInstall(t *testing.T) {
	// Regression: result.Drift used to carry the pre-install snapshot (e.g.
	// State=missing) even when the install step succeeded, producing JSON
	// output that contradicted the steps[] entries. Now it's recomputed
	// against the freshly-written file.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)

	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", SkipBeads: true}
	res, err := Run(context.Background(), prefs, tmp, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}

	if res.Drift.Detected {
		t.Errorf("Drift.Detected = true after successful install; want false. State=%s", res.Drift.State)
	}
	if res.Drift.State != drift.StateEqual {
		t.Errorf("Drift.State = %s after install; want %s", res.Drift.State, drift.StateEqual)
	}
	if res.Drift.Message != "" {
		t.Errorf("Drift.Message = %q after install; want empty", res.Drift.Message)
	}
	if res.Drift.InstalledVersion == "" {
		t.Error("Drift.InstalledVersion empty after install")
	}
}

func TestRun_RefusesHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", SkipBeads: true}
	_, err := Run(context.Background(), prefs, home, "/tmp")
	if err == nil || !strings.Contains(err.Error(), "$HOME") {
		t.Errorf("expected $HOME refusal, got %v", err)
	}
}

func TestRun_RefusesRoot(t *testing.T) {
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", SkipBeads: true}
	_, err := Run(context.Background(), prefs, "/", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "filesystem root") {
		t.Errorf("expected root refusal, got %v", err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// setupFakePluginRoot creates a minimal plugin structure with a rules file.
// Returns the path. Note: no config-template.env is created — `.env.example` is
// generated from letsconfig.Keys defaults at lets init time (post lets-8ilsl).
func setupFakePluginRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	rules := `---
name: lets-rules
version: 0.4.0
---
# Test rules
`
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "lets-rules.md"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
