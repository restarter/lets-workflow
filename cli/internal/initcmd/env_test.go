package initcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

func TestRegenerateEnv_FreshCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	prefs := Prefs{Language: "Ukrainian", MergeBranch: "main", PRFlow: "github", Tracker: "beads"}
	action, err := RegenerateEnv(path, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvCreated {
		t.Errorf("Kind = %s, want %s", action.Kind, EnvCreated)
	}
	if action.NewVersion != version.Version {
		t.Errorf("NewVersion = %q, want %q", action.NewVersion, version.Version)
	}
	if action.BackupPath != "" {
		t.Errorf("BackupPath = %q, want empty (fresh create)", action.BackupPath)
	}

	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte(letsconfig.VersionKeyName+"="+version.Version)) {
		t.Errorf("missing %s line:\n%s", letsconfig.VersionKeyName, data)
	}
	if !bytes.Contains(data, []byte("LETS_LANGUAGE=Ukrainian")) {
		t.Errorf("LETS_LANGUAGE missing:\n%s", data)
	}
}

func TestRegenerateEnv_PreservesValuesOnVersionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	// Old .env without version marker, with user values
	old := []byte("# old header\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=develop\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n")
	if err := os.WriteFile(path, old, 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty prefs (no CLI flags) → preserve all existing, just refresh version + header
	action, err := RegenerateEnv(path, Prefs{Tracker: "beads"})
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvRegenerated {
		t.Errorf("Kind = %s, want %s", action.Kind, EnvRegenerated)
	}

	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("LETS_LANGUAGE=English")) {
		t.Error("LETS_LANGUAGE lost during regen")
	}
	if !bytes.Contains(data, []byte("LETS_MERGE_BRANCH=develop")) {
		t.Error("LETS_MERGE_BRANCH lost during regen")
	}
	if !bytes.Contains(data, []byte("LETS_PR_FLOW=local")) {
		t.Error("LETS_PR_FLOW lost during regen")
	}
	if !bytes.Contains(data, []byte(letsconfig.VersionKeyName+"=")) {
		t.Error("version marker not added on regen")
	}
}

func TestRegenerateEnv_OverridesViaPrefsFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pass language flag → overrides; other prefs preserved from file
	action, err := RegenerateEnv(path, Prefs{Language: "Ukrainian", Tracker: "beads"})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("LETS_LANGUAGE=Ukrainian")) {
		t.Error("LANGUAGE not overridden")
	}
	if !bytes.Contains(data, []byte("LETS_MERGE_BRANCH=main")) {
		t.Error("MERGE_BRANCH lost")
	}

	if !contains(action.ChangedKeys, "LETS_LANGUAGE") {
		t.Errorf("ChangedKeys missing LETS_LANGUAGE: %v", action.ChangedKeys)
	}
}

func TestRegenerateEnv_PreservesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\nMY_TOKEN=secret\nFOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := RegenerateEnv(path, Prefs{Language: "Ukrainian", Tracker: "beads"})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("MY_TOKEN=secret")) {
		t.Error("foreign key MY_TOKEN lost")
	}
	if !bytes.Contains(data, []byte("FOO=bar")) {
		t.Error("foreign key FOO lost")
	}
	if !bytes.Contains(data, []byte("# User-added keys")) {
		t.Error("foreign separator missing")
	}
}

func TestRegenerateEnv_PreservesUserTracker(t *testing.T) {
	// Tracker has no CLI flag → cobra always fills prefs.Tracker from defaults.
	// User-customized LETS_TRACKER in .env must survive regen unchanged.
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=linear\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate cobra-wrapper-filled defaults: prefs.Tracker = "beads"
	_, err := RegenerateEnv(path, Prefs{Tracker: "beads"})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("LETS_TRACKER=linear")) {
		t.Errorf("user-customized LETS_TRACKER overwritten:\n%s", data)
	}
}

func TestRegenerateEnv_BackupCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	initial := "LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	// Force regen via prefs flag change
	_, err := RegenerateEnv(path, Prefs{Language: "Ukrainian", Tracker: "beads"})
	if err != nil {
		t.Fatal(err)
	}

	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("expected .bak created, got %v", err)
	}
	if string(bak) != initial {
		t.Errorf(".bak content mismatch:\n got:  %q\n want: %q", bak, initial)
	}
	fi, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Errorf(".bak mode = %o, want 0600", mode)
	}
}

func TestRegenerateEnv_BackupOverwritten(t *testing.T) {
	// Verify .bak rotates between runs (not stale pre-first-run content).
	path := filepath.Join(t.TempDir(), ".env")
	preRun1 := "LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"
	if err := os.WriteFile(path, []byte(preRun1), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RegenerateEnv(path, Prefs{Language: "Ukrainian", Tracker: "beads"}); err != nil {
		t.Fatalf("run 1: %v", err)
	}

	// Mutate path between runs to prove .bak rotated.
	if err := os.WriteFile(path, []byte("BETWEEN=runs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RegenerateEnv(path, Prefs{Language: "Italian", Tracker: "beads"}); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	bak, _ := os.ReadFile(path + ".bak")
	if !bytes.Contains(bak, []byte("BETWEEN=runs")) {
		t.Errorf(".bak content didn't rotate, got: %s", bak)
	}
}

func TestRegenerateEnv_SkipWhenInSync(t *testing.T) {
	// File at canonical version + same values + no prefs flags → EnvSkip,
	// no .bak written, no file mutation.
	path := filepath.Join(t.TempDir(), ".env")
	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads"}

	// Create
	if _, err := RegenerateEnv(path, prefs); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)

	// Re-run with same prefs → should skip
	action, err := RegenerateEnv(path, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvSkip {
		t.Errorf("Kind = %s, want %s", action.Kind, EnvSkip)
	}
	if action.BackupPath != "" {
		t.Errorf("BackupPath should be empty on skip, got %q", action.BackupPath)
	}

	// File untouched
	now, _ := os.ReadFile(path)
	if !bytes.Equal(original, now) {
		t.Error(".env modified despite EnvSkip")
	}
	// No .bak created
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak unexpectedly created on skip path")
	}
}

func TestReadEnvVersion(t *testing.T) {
	tmp := t.TempDir()

	// Missing file → ""
	if v := readEnvVersion(filepath.Join(tmp, "missing.env")); v != "" {
		t.Errorf("missing file: got %q, want empty", v)
	}

	// File without marker → ""
	noMarker := filepath.Join(tmp, "nomarker.env")
	_ = os.WriteFile(noMarker, []byte("LETS_LANGUAGE=English\n"), 0o644)
	if v := readEnvVersion(noMarker); v != "" {
		t.Errorf("no marker: got %q, want empty", v)
	}

	// File with marker → returns version
	withMarker := filepath.Join(tmp, "marker.env")
	_ = os.WriteFile(withMarker, []byte("LETS_ENV_VERSION=1.2.3\nLETS_LANGUAGE=English\n"), 0o644)
	if v := readEnvVersion(withMarker); v != "1.2.3" {
		t.Errorf("with marker: got %q, want %q", v, "1.2.3")
	}
}

func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
