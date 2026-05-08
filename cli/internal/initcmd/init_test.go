package initcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_FreshProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)

	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", SkipBeads: true}
	steps, err := Run(context.Background(), prefs, RunOptions{}, tmp, pluginRoot)
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

	if len(steps) < 5 {
		t.Errorf("steps = %d, want >=5", len(steps))
	}
}

func TestRun_Idempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", SkipBeads: true}

	if _, err := Run(context.Background(), prefs, RunOptions{}, tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	steps, err := Run(context.Background(), prefs, RunOptions{}, tmp, pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	skipCount := 0
	for _, s := range steps {
		if s.Status == StepSkip {
			skipCount++
		}
	}
	if skipCount < 2 {
		t.Errorf("second run had %d skips, want >=2", skipCount)
	}
}

func TestRun_RefusesHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", SkipBeads: true}
	_, err := Run(context.Background(), prefs, RunOptions{}, home, "/tmp")
	if err == nil || !strings.Contains(err.Error(), "$HOME") {
		t.Errorf("expected $HOME refusal, got %v", err)
	}
}

func TestRun_RefusesRoot(t *testing.T) {
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", SkipBeads: true}
	_, err := Run(context.Background(), prefs, RunOptions{}, "/", "/tmp")
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

// setupFakePluginRoot creates a minimal plugin structure with a rules file
// and config-template.env. Returns the path.
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
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hooks", "config-template.env"), []byte("# template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
