package initcmd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

var updateGoldens = flag.Bool("update", false, "regenerate testdata/golden_env_*.txt")

func TestRenderEnv_Golden(t *testing.T) {
	tests := []struct {
		name   string
		prefs  Prefs
		golden string
	}{
		{"default", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", Launcher: "terminal"}, "testdata/golden_env_default.txt"},
		{"ukrainian", Prefs{Language: "Ukrainian", MergeBranch: "main", PRFlow: "local", Tracker: "beads", Launcher: "terminal"}, "testdata/golden_env_ukrainian.txt"},
		{"github", Prefs{Language: "English", MergeBranch: "main", PRFlow: "github", Tracker: "beads", Launcher: "terminal"}, "testdata/golden_env_github.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderEnv(tt.prefs)
			if *updateGoldens {
				if err := os.MkdirAll(filepath.Dir(tt.golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tt.golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("regenerated %s", tt.golden)
				return
			}
			want, err := os.ReadFile(tt.golden)
			if err != nil {
				t.Fatalf("read golden: %v\n\nRegenerate via:\n  go test ./internal/initcmd -run TestRenderEnv_Golden -update", err)
			}
			if string(got) != string(want) {
				t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s\n\nRegenerate via:\n  go test ./internal/initcmd -run TestRenderEnv_Golden -update", tt.golden, got, want)
			}
		})
	}
}

// TestRenderEnv_NonEmptyValues catches "Tracker zero-value leaked through" type
// bugs independently of golden contents. Runs without -update.
func TestRenderEnv_NonEmptyValues(t *testing.T) {
	prefs := Prefs{
		Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads", Launcher: "terminal",
	}
	got := string(renderEnv(prefs))
	for _, expected := range []string{
		"LETS_LANGUAGE=English",
		"LETS_MERGE_BRANCH=main",
		"LETS_PR_FLOW=local",
		"LETS_TRACKER=beads",
		"LETS_LAUNCHER=terminal",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("renderEnv missing %q in output:\n%s", expected, got)
		}
	}
	for _, k := range []string{"LETS_LANGUAGE=", "LETS_MERGE_BRANCH=", "LETS_PR_FLOW=", "LETS_TRACKER=", "LETS_LAUNCHER="} {
		if strings.Contains(got, k+"\n") {
			t.Errorf("renderEnv has empty value for %s", k)
		}
	}
}

// TestRenderEnvExample_Output mirrors NonEmptyValues for the example file.
// Critical: .env.example ships to every user (no plugin template fallback after
// Task 10), so a regression here is silently broken docs.
func TestRenderEnvExample_Output(t *testing.T) {
	got := string(renderEnvExample())

	if !strings.HasPrefix(got, letsconfig.ExampleHeader) {
		t.Errorf("renderEnvExample missing ExampleHeader prefix:\n%s", got)
	}
	if strings.HasPrefix(got, letsconfig.Header) {
		t.Error("renderEnvExample using active-config Header instead of ExampleHeader")
	}
	for _, k := range letsconfig.Keys {
		wantLine := k.Name + "=" + k.Default
		if !strings.Contains(got, wantLine) {
			t.Errorf("renderEnvExample missing %q", wantLine)
		}
		if !strings.Contains(got, "# "+k.Comment) {
			t.Errorf("renderEnvExample missing comment for %s: %q", k.Name, k.Comment)
		}
		if strings.Contains(got, k.Name+"=\n") {
			t.Errorf("renderEnvExample has empty value for %s", k.Name)
		}
	}
}
