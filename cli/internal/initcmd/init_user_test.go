package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/drift"
)

func userOpts(t *testing.T, language string) UserOptions {
	t.Helper()
	return UserOptions{
		Language:   language,
		HomeDir:    t.TempDir(),
		PluginRoot: setupFakePluginRoot(t),
	}
}

func globalRulesPath(o UserOptions) string {
	return filepath.Join(o.HomeDir, ".claude", "rules", "lets-rules.md")
}

func findStep(steps []Step, substr string) *Step {
	for i := range steps {
		if strings.Contains(steps[i].Message, substr) {
			return &steps[i]
		}
	}
	return nil
}

func TestRunUser_FreshInstall(t *testing.T) {
	o := userOpts(t, "Ukrainian")
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := os.ReadFile(globalRulesPath(o))
	if err != nil {
		t.Fatalf("global rules not written: %v", err)
	}
	if !strings.Contains(string(rules), "version: 0.4.0") {
		t.Errorf("rules version mismatch:\n%s", rules)
	}
	env, err := os.ReadFile(filepath.Join(o.HomeDir, ".lets", ".env"))
	if err != nil {
		t.Fatalf("user env not written: %v", err)
	}
	for _, want := range []string{"LETS_LANGUAGE=Ukrainian", "LETS_LAUNCHER=terminal"} {
		if !strings.Contains(string(env), want) {
			t.Errorf("missing %q in user env:\n%s", want, env)
		}
	}
	if strings.Contains(string(env), "LETS_MERGE_BRANCH=") {
		t.Errorf("per-project key managed in user env:\n%s", env)
	}
	if s := findStep(result.Steps, "lets-rules.md installed"); s == nil || s.Status != StepOK {
		t.Errorf("missing ok install step: %+v", result.Steps)
	}
	if result.ProjectRoot != o.HomeDir {
		t.Errorf("ProjectRoot should carry HomeDir as scope root: got %q", result.ProjectRoot)
	}
}

// RunUser is a deliberate SUBSET of project Run - pins that nothing
// project-bound appears under the fake home.
func TestRunUser_StaysASubset(t *testing.T) {
	o := userOpts(t, "")
	if _, err := RunUser(o); err != nil {
		t.Fatal(err)
	}
	for _, reject := range []string{
		filepath.Join(o.HomeDir, ".claude", "settings.json"),
		filepath.Join(o.HomeDir, ".lets", ".env.example"),
		filepath.Join(o.HomeDir, ".gitignore"),
	} {
		if _, err := os.Stat(reject); err == nil {
			t.Errorf("user-scope install must not create %s", reject)
		}
	}
}

func TestRunUser_IdempotentRerun(t *testing.T) {
	o := userOpts(t, "Ukrainian")
	if _, err := RunUser(o); err != nil {
		t.Fatal(err)
	}
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range result.Steps {
		if s.Status != StepSkip {
			t.Errorf("re-run step not skip: %+v", s)
		}
	}
}

func TestRunUser_OutdatedGlobalRewritten(t *testing.T) {
	o := userOpts(t, "")
	writeRulesFile(t, globalRulesPath(o), "0.3.0")
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	if s := findStep(result.Steps, "updated (v0.3.0 -> v0.4.0)"); s == nil {
		t.Errorf("missing updated step: %+v", result.Steps)
	}
	if result.Drift.State != drift.StateEqual {
		t.Errorf("post-write drift should be equal, got %s", result.Drift.State)
	}
}

// The global file is the documented opt-out mechanism (GH #8395) - a NEWER
// version is never clobbered, unlike the project copy.
func TestRunUser_AheadNotClobbered(t *testing.T) {
	o := userOpts(t, "")
	writeRulesFile(t, globalRulesPath(o), "9.9.9")
	before, _ := os.ReadFile(globalRulesPath(o))
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(globalRulesPath(o))
	if string(before) != string(after) {
		t.Errorf("ahead global rules were clobbered")
	}
	if s := findStep(result.Steps, "AHEAD"); s == nil || s.Status != StepWarn {
		t.Errorf("missing AHEAD warn step: %+v", result.Steps)
	}
	if result.Drift.State != drift.StateAhead {
		t.Errorf("Drift.State: got %s want ahead", result.Drift.State)
	}
}

// unknown (unparseable frontmatter) IS overwritten - deliberate: lets-* files
// are plugin-owned by convention and an unversioned copy can't be tracked.
func TestRunUser_UnknownOverwritten(t *testing.T) {
	o := userOpts(t, "")
	if err := os.MkdirAll(filepath.Dir(globalRulesPath(o)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalRulesPath(o), []byte("# hand-stripped frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	if s := findStep(result.Steps, "refreshed (was: unparseable"); s == nil || s.Status != StepOK {
		t.Errorf("missing refreshed step: %+v", result.Steps)
	}
	data, _ := os.ReadFile(globalRulesPath(o))
	if !strings.Contains(string(data), "version: 0.4.0") {
		t.Errorf("unparseable global not rewritten:\n%s", data)
	}
}

func TestRunUser_CustomizedEnvSurvives(t *testing.T) {
	o := userOpts(t, "Ukrainian")
	if _, err := RunUser(o); err != nil {
		t.Fatal(err)
	}
	// Re-run with NO language flag: existing customized value must survive
	// the Prefs plumbing end-to-end (not just at RegenerateUserEnv level).
	o.Language = ""
	if _, err := RunUser(o); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(filepath.Join(o.HomeDir, ".lets", ".env"))
	if !strings.Contains(string(env), "LETS_LANGUAGE=Ukrainian") {
		t.Errorf("customized LETS_LANGUAGE lost through RunUser re-run:\n%s", env)
	}
}

func TestRunUser_GuardHomeDir(t *testing.T) {
	plugin := setupFakePluginRoot(t)
	for _, home := range []string{"", "/", "."} {
		if _, err := RunUser(UserOptions{HomeDir: home, PluginRoot: plugin}); err == nil {
			t.Errorf("HomeDir %q: expected refusal, got nil", home)
		}
	}
}

// Stat succeeds but ReadVersion fails on an unreadable file -> StateUnknown ->
// rewritten via AtomicWriteBytes (temp+rename ignores old file perms) -> ok.
func TestRunUser_UnreadableGlobalFile_Rewritten(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("chmod ineffective as root")
	}
	o := userOpts(t, "")
	writeRulesFile(t, globalRulesPath(o), "0.4.0")
	if err := os.Chmod(globalRulesPath(o), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(globalRulesPath(o), 0o644) })
	result, err := RunUser(o)
	if err != nil {
		t.Fatalf("unreadable file should degrade, not error: %v", err)
	}
	if s := findStep(result.Steps, "refreshed (was: unparseable"); s == nil {
		t.Errorf("expected unknown->refreshed path: %+v", result.Steps)
	}
}

// Read-only rules DIR is the genuine degraded-write case: AtomicWriteBytes
// can't create its temp file -> RunUser returns an error with partial Result.
func TestRunUser_ReadOnlyRulesDir_Errors(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("chmod ineffective as root")
	}
	o := userOpts(t, "")
	rulesDir := filepath.Dir(globalRulesPath(o))
	writeRulesFile(t, globalRulesPath(o), "0.3.0") // outdated -> write path
	if err := os.Chmod(rulesDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(rulesDir, 0o755) })
	if _, err := RunUser(o); err == nil {
		t.Error("expected write error on read-only rules dir")
	}
}

// writeRulesFile writes a frontmattered rules file at path.
func writeRulesFile(t *testing.T, path, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: lets-rules\nversion: " + version + "\n---\n# Rules\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
