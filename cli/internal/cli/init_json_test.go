package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
)

// runInitJSON executes lets init via cobra and unmarshals the JSON output.
// Reuses chdirTo (defined in init_test.go, same _test package) for serial
// directory switching. Tests in this package run serially - never use t.Parallel.
func runInitJSON(t *testing.T, dir string, args ...string) initcmd.Result {
	t.Helper()
	if dir != "" {
		chdirTo(t, dir)
	}
	cmd := cli.NewInitCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	execErr := cmd.Execute()

	if stdout.Len() == 0 {
		t.Fatalf("Execute returned %v, no stdout. stderr=%q", execErr, stderr.String())
	}

	var result initcmd.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %q\nstderr: %q\nexec err: %v",
			err, stdout.String(), stderr.String(), execErr)
	}
	return result
}

// initGitRepo creates a temp dir as a git repo so DetectProjectRoot succeeds.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	return dir
}

func TestInit_JSON_FirstTime(t *testing.T) {
	dir := initGitRepo(t)
	pluginRoot := makeFakePluginRoot(t)

	result := runInitJSON(t, dir, "--json", "--plugin-root="+pluginRoot,
		"--language=Ukrainian", "--merge-branch=main", "--pr-flow=local", "--skip-beads")

	if !result.OK {
		t.Errorf("OK: got false, error: %s", result.Error)
	}
	if result.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: got %d want 1", result.SchemaVersion)
	}
	if result.EnvAction.Kind != initcmd.EnvCreated {
		t.Errorf("EnvAction.Kind: got %q want %q", result.EnvAction.Kind, initcmd.EnvCreated)
	}
	if len(result.Steps) == 0 {
		t.Error("Steps empty")
	}
	for _, s := range result.Steps {
		switch s.Status {
		case initcmd.StepOK, initcmd.StepSkip, initcmd.StepWarn, initcmd.StepErr, initcmd.StepMigrate:
		default:
			t.Errorf("invalid step status: %q", s.Status)
		}
	}
	// Step shape stability: marshal a step and assert wire-format keys
	stepBytes, _ := json.Marshal(result.Steps[0])
	if !strings.Contains(string(stepBytes), `"status"`) {
		t.Error("Step JSON missing 'status' key")
	}
	if !strings.Contains(string(stepBytes), `"message"`) {
		t.Error("Step JSON missing 'message' key")
	}
}

func TestInit_JSON_ReRun_NoFlagChanges_KindSkip(t *testing.T) {
	// Re-run with same prefs flags as first run → no value changes → EnvSkip.
	dir := initGitRepo(t)
	pluginRoot := makeFakePluginRoot(t)
	args := []string{"--json", "--plugin-root=" + pluginRoot, "--language=English",
		"--merge-branch=main", "--pr-flow=local", "--skip-beads"}

	_ = runInitJSON(t, dir, args...)
	result := runInitJSON(t, dir, args...)

	if !result.OK {
		t.Errorf("OK: got false, error: %s", result.Error)
	}
	if result.EnvAction.Kind != initcmd.EnvSkip {
		t.Errorf("EnvAction.Kind on re-run: got %q want %q",
			result.EnvAction.Kind, initcmd.EnvSkip)
	}
}

func TestInit_JSON_ChangedPrefs_TriggersRegenerate(t *testing.T) {
	// Slash-command "Change config" path: passing different prefs values
	// triggers EnvRegenerated. Foreign keys + custom comments preserved.
	dir := initGitRepo(t)
	pluginRoot := makeFakePluginRoot(t)
	baseArgs := []string{"--json", "--plugin-root=" + pluginRoot, "--language=English",
		"--merge-branch=main", "--pr-flow=local", "--skip-beads"}

	_ = runInitJSON(t, dir, baseArgs...)

	envPath := filepath.Join(dir, ".lets", ".env")
	original, _ := os.ReadFile(envPath)
	// Inject a foreign key; canonical comments are owned by RegenerateEnv and
	// will be re-rendered, but foreign KEY=VALUE survives the regen.
	custom := string(original) + "\nLETS_FUTURE=test\n"
	_ = os.WriteFile(envPath, []byte(custom), 0o644)

	result := runInitJSON(t, dir, "--json", "--plugin-root="+pluginRoot,
		"--language=Italian", "--merge-branch=main", "--pr-flow=github", "--skip-beads")

	if !result.OK {
		t.Errorf("OK: got false, error: %s", result.Error)
	}
	if result.EnvAction.Kind != initcmd.EnvRegenerated {
		t.Errorf("EnvAction.Kind: got %q want %q", result.EnvAction.Kind, initcmd.EnvRegenerated)
	}

	hasLang := false
	hasFlow := false
	for _, k := range result.EnvAction.ChangedKeys {
		if k == "LETS_LANGUAGE" {
			hasLang = true
		}
		if k == "LETS_PR_FLOW" {
			hasFlow = true
		}
	}
	if !hasLang || !hasFlow {
		t.Errorf("ChangedKeys missing LETS_LANGUAGE or LETS_PR_FLOW: got %v",
			result.EnvAction.ChangedKeys)
	}
	if result.EnvAction.BackupPath == "" {
		t.Error("BackupPath empty")
	}
	updated, _ := os.ReadFile(envPath)
	if !bytes.Contains(updated, []byte("LETS_FUTURE=test")) {
		t.Error("foreign key not preserved")
	}
	if !bytes.Contains(updated, []byte("# User-added keys")) {
		t.Error("foreign-keys section header missing")
	}
}

func TestInit_JSON_EarlyError_NotInGit(t *testing.T) {
	dir := t.TempDir() // no git init
	pluginRoot := makeFakePluginRoot(t)

	result := runInitJSON(t, dir, "--json", "--plugin-root="+pluginRoot, "--skip-beads")

	if result.OK {
		t.Error("OK: got true want false")
	}
	if result.Error == "" {
		t.Error("Error empty on early failure")
	}
	if !strings.Contains(result.Error, "git") {
		t.Errorf("Error doesn't mention git: %q", result.Error)
	}
}

func TestInit_JSON_EarlyError_InsideWorktree(t *testing.T) {
	mainDir := initGitRepo(t)
	for _, c := range [][]string{
		{"git", "config", "user.email", "t@t"},
		{"git", "config", "user.name", "t"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = mainDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", c, err, out)
		}
	}
	wtDir := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "worktree", "add", wtDir, "-b", "wtbranch")
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
	pluginRoot := makeFakePluginRoot(t)

	result := runInitJSON(t, wtDir, "--json", "--plugin-root="+pluginRoot, "--skip-beads")

	if result.OK {
		t.Error("OK: got true want false (worktree should be rejected)")
	}
	if !strings.Contains(strings.ToLower(result.Error), "worktree") {
		t.Errorf("Error doesn't mention worktree: %q", result.Error)
	}
}

func TestInit_JSON_PartialFailure(t *testing.T) {
	// Force a mid-flight failure: pre-create .lets/.env.example as a DIRECTORY
	// so Step 6 atomicWriteBytes fails after Steps 1-5 complete.
	dir := initGitRepo(t)
	pluginRoot := makeFakePluginRoot(t)

	exampleAsDir := filepath.Join(dir, ".lets", ".env.example")
	if err := os.MkdirAll(exampleAsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result := runInitJSON(t, dir, "--json", "--plugin-root="+pluginRoot, "--skip-beads")

	if result.OK {
		t.Error("OK: got true want false (Step 5 should fail when .env is a dir)")
	}
	if result.Error == "" {
		t.Error("Error empty on partial failure")
	}
	if len(result.Steps) == 0 {
		t.Error("Steps empty — partial-completion contract requires work-completed-so-far")
	}
	if result.Summary.OKCount == 0 {
		t.Error("Summary.OKCount==0 — partial-completion contract requires earlier successful steps to be counted")
	}
}

func TestResult_SchemaContract(t *testing.T) {
	// Hard-code expected JSON field set. When fields are added/removed,
	// this test breaks LOUD, forcing the SchemaVersion bump conversation.
	r := initcmd.NewResult("/p", "/q")
	r.OK = true
	r.Steps = []initcmd.Step{{Status: initcmd.StepOK, Message: "x"}}
	bs, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(bs, &m); err != nil {
		t.Fatal(err)
	}
	wantKeys := map[string]bool{
		"schema_version": true,
		"ok":             true,
		"project_root":   true,
		"plugin_root":    true,
		"steps":          true,
		"env_action":     true,
		"drift":          true,
		"summary":        true,
		// "error" omitempty - absent when ok=true
	}
	for k := range m {
		if !wantKeys[k] {
			t.Errorf("unexpected JSON key %q (did you add a field? bump SchemaVersion in result.go)", k)
		}
		delete(wantKeys, k)
	}
	for k := range wantKeys {
		t.Errorf("missing JSON key %q", k)
	}
}
