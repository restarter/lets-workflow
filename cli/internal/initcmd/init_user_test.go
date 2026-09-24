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
	home := t.TempDir()
	return UserOptions{
		Language:   language,
		HomeDir:    home,
		PluginRoot: installedPluginRoot(t, home, "0.4.0"),
	}
}

// installedPluginRoot lays out an installed plugin release the rules cache
// trusts: <home>/.claude/plugins/cache/<marketplace>/lets/<ver>.
func installedPluginRoot(t *testing.T, home, ver string) string {
	t.Helper()
	root := filepath.Join(home, ".claude", "plugins", "cache", "lets-workflow", "lets", ver)
	writeFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"lets","version":"`+ver+`"}`)
	writeRulesFile(t, filepath.Join(root, "rules", "lets-rules.md"), ver)
	return root
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
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
	if s := findStep(result.Steps, "Global workflow rules installed"); s == nil || s.Status != StepOK {
		t.Errorf("missing ok install step: %+v", result.Steps)
	}
	if result.Drift.Detected || result.Drift.State != drift.StateEqual {
		t.Errorf("drift after a fresh install: %+v", result.Drift)
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

// A second --user run over an unchanged cache says "matches", not "installed".
func TestRunUser_SecondRunIsSkip(t *testing.T) {
	o := userOpts(t, "")
	if _, err := RunUser(o); err != nil {
		t.Fatal(err)
	}
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	if s := findStep(result.Steps, "matches the running plugin"); s == nil || s.Status != StepSkip {
		t.Errorf("missing skip step: %+v", result.Steps)
	}
	if result.Drift.Detected {
		t.Errorf("no drift expected: %+v", result.Drift)
	}
}

// Only an installed plugin writes the cache: a root outside
// ~/.claude/plugins/cache is a named warning with drift.detected, never a write.
func TestRunUser_UntrustedPluginRootWarns(t *testing.T) {
	o := userOpts(t, "")
	o.PluginRoot = setupFakePluginRoot(t) // a temp dir, not an installed release
	result, err := RunUser(o)
	if err != nil {
		t.Fatal(err)
	}
	if s := findStep(result.Steps, "not an installed plugin"); s == nil || s.Status != StepWarn {
		t.Errorf("missing untrusted-root warn step: %+v", result.Steps)
	}
	if !result.Drift.Detected || !strings.Contains(result.Drift.Message, "not an installed plugin") {
		t.Errorf("drift must be detected with the reason: %+v", result.Drift)
	}
	if _, err := os.Stat(globalRulesPath(o)); !os.IsNotExist(err) {
		t.Errorf("untrusted root must not create the global rules: %v", err)
	}
}

// A newer cache is never downgraded by an older plugin's --user run.
func TestRunUser_NewerCacheKept(t *testing.T) {
	o := userOpts(t, "")
	newer := installedPluginRoot(t, o.HomeDir, "0.5.0")
	if _, err := RunUser(UserOptions{HomeDir: o.HomeDir, PluginRoot: newer}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(globalRulesPath(o))
	result, err := RunUser(o) // the older 0.4.0 plugin
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(globalRulesPath(o))
	if string(before) != string(after) {
		t.Errorf("newer global rules were downgraded")
	}
	if s := findStep(result.Steps, "kept at v0.5.0"); s == nil || s.Status != StepWarn {
		t.Errorf("missing kept-newer warn step: %+v", result.Steps)
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

// A write failure (read-only rules dir) is a named warning in the Result,
// never an error: the global copy is left exactly as it was.
func TestRunUser_ReadOnlyRulesDir_Warns(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("chmod ineffective as root")
	}
	o := userOpts(t, "")
	rulesDir := filepath.Dir(globalRulesPath(o))
	writeRulesFile(t, globalRulesPath(o), "0.3.0") // stale -> write path
	before, _ := os.ReadFile(globalRulesPath(o))
	if err := os.Chmod(rulesDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(rulesDir, 0o755) })
	result, err := RunUser(o)
	if err != nil {
		t.Fatalf("a sync failure must degrade, not error: %v", err)
	}
	if s := findStep(result.Steps, "FAILED"); s == nil || s.Status != StepWarn {
		t.Errorf("missing FAILED warn step: %+v", result.Steps)
	}
	if after, _ := os.ReadFile(globalRulesPath(o)); string(after) != string(before) {
		t.Errorf("global rules changed despite the failure")
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
