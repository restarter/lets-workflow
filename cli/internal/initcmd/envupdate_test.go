package initcmd

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func writeEnv(t *testing.T, dir, content string) string {
	t.Helper()
	envDir := filepath.Join(dir, ".lets")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(envDir, ".env")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUpdateEnvKeys_PreservesCommentsAndForeignKeys(t *testing.T) {
	dir := t.TempDir()
	original := `# Project config
LETS_LANGUAGE=English

# Custom: I prefer master here
LETS_MERGE_BRANCH=master
LETS_PR_FLOW=local
LETS_TRACKER=beads

# Foreign key for upcoming feature
LETS_FUTURE_FLAG=experimental
GITHUB_TOKEN=hunter2
`
	envPath := writeEnv(t, dir, original)

	prefs := Prefs{Language: "Ukrainian", MergeBranch: "main", PRFlow: "github", Tracker: "beads"}
	action, err := UpdateEnvKeys(envPath, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvUpdated {
		t.Fatalf("Kind: got %q want %q", action.Kind, EnvUpdated)
	}
	sort.Strings(action.ChangedKeys)
	wantChanged := []string{"LETS_LANGUAGE", "LETS_MERGE_BRANCH", "LETS_PR_FLOW"}
	if !reflect.DeepEqual(action.ChangedKeys, wantChanged) {
		t.Errorf("ChangedKeys: got %v want %v", action.ChangedKeys, wantChanged)
	}

	updated, _ := os.ReadFile(envPath)
	out := string(updated)
	if !strings.Contains(out, "# Custom: I prefer master here") {
		t.Error("custom comment not preserved")
	}
	if !strings.Contains(out, "LETS_FUTURE_FLAG=experimental") {
		t.Error("foreign LETS_FUTURE_FLAG not preserved")
	}
	if !strings.Contains(out, "GITHUB_TOKEN=hunter2") {
		t.Error("non-LETS foreign key not preserved")
	}
	if !strings.Contains(out, "LETS_LANGUAGE=Ukrainian") {
		t.Error("LETS_LANGUAGE not updated")
	}
	if !strings.Contains(out, "LETS_PR_FLOW=github") {
		t.Error("LETS_PR_FLOW not updated")
	}

	bak, err := os.ReadFile(envPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != original {
		t.Errorf("backup content differs from original")
	}
}

func TestUpdateEnvKeys_NoOpReturnsSkip_FileUntouched(t *testing.T) {
	dir := t.TempDir()
	original := `LETS_LANGUAGE=English
LETS_MERGE_BRANCH=main
LETS_PR_FLOW=local
LETS_TRACKER=beads
`
	envPath := writeEnv(t, dir, original)

	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads"}
	action, err := UpdateEnvKeys(envPath, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvSkip {
		t.Errorf("Kind: got %q want %q", action.Kind, EnvSkip)
	}
	if len(action.ChangedKeys) != 0 {
		t.Errorf("ChangedKeys: got %v want []", action.ChangedKeys)
	}
	// Strong assertion: 4 LETS_* lines preserved (proves we walked but didn't write)
	if action.PreservedLines != 4 {
		t.Errorf("PreservedLines: got %d want 4", action.PreservedLines)
	}
	// Backup must NOT be created on no-op
	if _, err := os.Stat(envPath + ".bak"); !os.IsNotExist(err) {
		t.Error(".env.bak should not exist on no-op skip")
	}
	// File content unchanged
	after, _ := os.ReadFile(envPath)
	if string(after) != original {
		t.Error(".env content changed on no-op skip")
	}
}

func TestUpdateEnvKeys_AppendsMissingKeys_WithCanonicalComments(t *testing.T) {
	dir := t.TempDir()
	original := "LETS_LANGUAGE=English\n"
	envPath := writeEnv(t, dir, original)

	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads"}
	action, err := UpdateEnvKeys(envPath, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvUpdated {
		t.Fatalf("Kind: got %q want %q", action.Kind, EnvUpdated)
	}
	sort.Strings(action.ChangedKeys)
	want := []string{"LETS_MERGE_BRANCH", "LETS_PR_FLOW", "LETS_TRACKER"}
	if !reflect.DeepEqual(action.ChangedKeys, want) {
		t.Errorf("ChangedKeys: got %v want %v", action.ChangedKeys, want)
	}

	updated, _ := os.ReadFile(envPath)
	out := string(updated)
	for _, k := range want {
		if !strings.Contains(out, k+"=") {
			t.Errorf("missing key %s in output", k)
		}
	}
	if !strings.Contains(out, "# (added by /lets:init)") {
		t.Error("appended-keys block header missing")
	}
	// Each appended key should have its canonical comment from letsconfig.Keys
	if !strings.Contains(out, "# Target branch for merges and PR base") {
		t.Error("LETS_MERGE_BRANCH canonical comment missing in append block")
	}
	if !strings.Contains(out, "# PR flow: github | bitbucket | local") {
		t.Error("LETS_PR_FLOW canonical comment missing in append block")
	}
	if !strings.Contains(out, "# Task tracker (currently 'beads'") {
		t.Error("LETS_TRACKER canonical comment missing in append block")
	}
}

func TestUpdateEnvKeys_CRLFInput(t *testing.T) {
	dir := t.TempDir()
	original := "LETS_LANGUAGE=English\r\nLETS_MERGE_BRANCH=main\r\nLETS_PR_FLOW=local\r\nLETS_TRACKER=beads\r\n"
	envPath := writeEnv(t, dir, original)

	prefs := Prefs{Language: "Italian", MergeBranch: "main", PRFlow: "local", Tracker: "beads"}
	action, err := UpdateEnvKeys(envPath, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvUpdated {
		t.Fatalf("Kind: got %q want %q", action.Kind, EnvUpdated)
	}

	updated, _ := os.ReadFile(envPath)
	if strings.Contains(string(updated), "\r") {
		t.Error("output contains \\r — should be normalized to LF")
	}
	if !strings.Contains(string(updated), "LETS_LANGUAGE=Italian") {
		t.Error("LETS_LANGUAGE not updated")
	}
}

func TestUpdateEnvKeys_ValueWithEqualsSign(t *testing.T) {
	dir := t.TempDir()
	original := "LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=feature/fix=bug\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"
	envPath := writeEnv(t, dir, original)

	prefs := Prefs{Language: "English", MergeBranch: "feature/fix=bug", PRFlow: "local", Tracker: "beads"}
	action, err := UpdateEnvKeys(envPath, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvSkip {
		t.Errorf("Kind: got %q want EnvSkip (values identical with = in branch)", action.Kind)
	}
}

func TestUpdateEnvKeys_EmptyKeyMatchesNothing(t *testing.T) {
	// Pathological line: LETS_=oops should NOT be matched as a managed key.
	// Our tightened regex requires at least one alpha char after LETS_.
	dir := t.TempDir()
	original := "LETS_=oops\nLETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\n"
	envPath := writeEnv(t, dir, original)

	prefs := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads"}
	action, err := UpdateEnvKeys(envPath, prefs)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != EnvSkip {
		t.Errorf("Kind: got %q want EnvSkip (LETS_= line is foreign, not managed)", action.Kind)
	}

	after, _ := os.ReadFile(envPath)
	if !strings.Contains(string(after), "LETS_=oops") {
		t.Error("LETS_=oops line should be preserved verbatim, not interpreted")
	}
}
