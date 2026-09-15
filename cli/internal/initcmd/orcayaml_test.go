package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func orcaProject(t *testing.T, launcher string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if launcher != "" {
		if err := os.WriteFile(filepath.Join(root, ".lets", ".env"), []byte("LETS_LAUNCHER="+launcher+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestOrcaYAML_WrittenSkippedForeign(t *testing.T) {
	root := orcaProject(t, "orca")
	step, err := EnsureOrcaYAML(root)
	if err != nil || step.Status != StepOK || !strings.Contains(step.Message, "commit it") {
		t.Fatalf("absent: %+v %v", step, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "orca.yaml"))
	if string(data) != OrcaYAMLContent {
		t.Errorf("content:\n%s", data)
	}
	if step, _ := EnsureOrcaYAML(root); step.Status != StepSkip {
		t.Errorf("marker + same must skip: %+v", step)
	}
	foreign := "scripts:\n  setup: \"make bootstrap\"\n"
	if err := os.WriteFile(filepath.Join(root, "orca.yaml"), []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}
	if step, _ := EnsureOrcaYAML(root); step.Status != StepWarn || !strings.Contains(step.Message, "lets worktree adopt") {
		t.Errorf("foreign must warn with the snippet: %+v", step)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "orca.yaml")); string(data) != foreign {
		t.Error("a foreign orca.yaml must be left untouched")
	}
}

func TestOrcaYAML_OnlyForProjectOrcaLauncher(t *testing.T) {
	root := orcaProject(t, "terminal")
	if step, _ := EnsureOrcaYAML(root); step.Message != "" {
		t.Errorf("terminal launcher: %+v", step)
	}
	// orca only in a user-level ~/.lets/.env: no committed team file
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".lets", ".env"), []byte("LETS_LAUNCHER=orca\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bare := orcaProject(t, "")
	EnsureOrcaYAML(bare)
	for _, r := range []string{root, bare} {
		if _, err := os.Stat(filepath.Join(r, "orca.yaml")); !os.IsNotExist(err) {
			t.Errorf("%s: no orca.yaml expected", r)
		}
	}
}

func TestOrcaYAML_HooksCannotFail(t *testing.T) {
	for _, line := range strings.Split(OrcaYAMLContent, "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "setup:"):
			if !strings.Contains(trim, "|| echo") || !strings.Contains(trim, "$HOME/.local/bin") {
				t.Errorf("setup must end in a command that cannot fail and carry the GUI PATH: %s", trim)
			}
		case strings.HasPrefix(trim, "archive:"):
			if !strings.HasSuffix(strings.TrimSuffix(trim, `"`), "|| true") {
				t.Errorf("archive must end in || true: %s", trim)
			}
		}
	}
}
