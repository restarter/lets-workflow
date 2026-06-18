package initcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTrackerSource adds a plugin-shipped tracker-<name>.md to pluginRoot/rules.
func writeTrackerSource(t *testing.T, pluginRoot, name, ver string) {
	t.Helper()
	body := "---\nname: tracker-" + name + "\nversion: " + ver + "\n---\n# tracker " + name + "\n"
	if err := os.WriteFile(filepath.Join(pluginRoot, "rules", "tracker-"+name+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeProjectEnv writes a minimal, parseable .lets/.env with the given tracker
// (used to exercise the "existing .env value is the source of truth" path).
func writeProjectEnv(t *testing.T, projectRoot, tracker string) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".lets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "LETS_ENV_VERSION=0.0.1\n" +
		"LETS_LANGUAGE=English\n" +
		"LETS_MERGE_BRANCH=main\n" +
		"LETS_PR_FLOW=local\n" +
		"LETS_TRACKER=" + tracker + "\n" +
		"LETS_LAUNCHER=terminal\n" +
		"LETS_RULES_SCOPE=project\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func trackerPrefs(tracker string) Prefs {
	return Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: tracker, SkipBeads: true}
}

func exists(t *testing.T, p string) bool {
	t.Helper()
	_, err := os.Stat(p)
	return err == nil
}

// Basic install: a fresh project with LETS_TRACKER=beads installs tracker-beads.md.
func TestRun_TrackerInstalled(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	writeTrackerSource(t, pluginRoot, "beads", "0.6.4")

	if _, err := Run(context.Background(), trackerPrefs("beads"), tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	if !exists(t, filepath.Join(tmp, ".claude", "rules", "tracker-beads.md")) {
		t.Error("tracker-beads.md not installed")
	}
}

// B1 regression: the resolved tracker comes from .lets/.env, NOT prefs.Tracker.
// A project whose .env says planfix-mcp, re-inited with the cobra DEFAULT
// prefs.Tracker=beads, must install tracker-planfix-mcp.md and NOT tracker-beads.md.
func TestRun_TrackerFromEnvNotDefault(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	writeTrackerSource(t, pluginRoot, "beads", "0.6.4")
	writeTrackerSource(t, pluginRoot, "planfix-mcp", "0.6.4")
	writeProjectEnv(t, tmp, "planfix-mcp")

	// prefs.Tracker = "beads" simulates the cobra default when --tracker is unset.
	if _, err := Run(context.Background(), trackerPrefs("beads"), tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(tmp, ".claude", "rules")
	if !exists(t, filepath.Join(rules, "tracker-planfix-mcp.md")) {
		t.Error("tracker-planfix-mcp.md must be installed (resolved from .env)")
	}
	if exists(t, filepath.Join(rules, "tracker-beads.md")) {
		t.Error("tracker-beads.md must NOT be installed - prefs.Tracker leaked over the .env value (B1)")
	}
}

// B2 regression: switching trackers removes the prior plugin-shipped tracker file
// but NEVER a user-authored tracker-<custom>.md, a *.board.md, lets-rules.md, or git.md.
func TestRun_TrackerSwitchPreservesUserFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	writeTrackerSource(t, pluginRoot, "beads", "0.6.4")
	writeTrackerSource(t, pluginRoot, "none", "0.6.4")

	rules := filepath.Join(tmp, ".claude", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing files that must survive a switch.
	userFiles := map[string]string{
		"tracker-beads.md":      "stale beads (plugin-shipped, should be removed)",
		"tracker-custom.md":     "user-authored adapter - KEEP",
		"tracker-none.board.md": "user board - KEEP",
		"git.md":                "user git rules - KEEP",
	}
	for name, content := range userFiles {
		if err := os.WriteFile(filepath.Join(rules, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeProjectEnv(t, tmp, "none")

	if _, err := Run(context.Background(), trackerPrefs("none"), tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(rules, "tracker-beads.md")) {
		t.Error("tracker-beads.md (plugin-shipped, not active) should be removed on switch")
	}
	for _, keep := range []string{"tracker-custom.md", "tracker-none.board.md", "git.md", "lets-rules.md"} {
		if !exists(t, filepath.Join(rules, keep)) {
			t.Errorf("%s must be preserved across a tracker switch", keep)
		}
	}
	if !exists(t, filepath.Join(rules, "tracker-none.md")) {
		t.Error("tracker-none.md must be installed after switch")
	}
}

// Invalid LETS_TRACKER (path-traversal shape) is rejected before any filepath.Join:
// a StepWarn, no write/delete outside the rules dir, no crash.
func TestRun_TrackerInvalidName(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	writeProjectEnv(t, tmp, "../evil")

	result, err := Run(context.Background(), trackerPrefs("beads"), tmp, pluginRoot)
	if err != nil {
		t.Fatalf("invalid tracker must not error: %v", err)
	}
	if !hasStepContaining(result.Steps, "not a valid adapter name") {
		t.Errorf("expected a StepWarn about the invalid adapter name; steps:\n%v", result.Steps)
	}
	// No traversal artifact anywhere near the rules dir.
	if exists(t, filepath.Join(tmp, ".claude", "evil.md")) {
		t.Error("path traversal wrote outside .claude/rules/")
	}
}

// LETS_TRACKER names a valid-but-unshipped adapter: a StepWarn, no install, no crash.
func TestRun_TrackerSourceMissing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t) // no tracker-bogus.md shipped
	writeProjectEnv(t, tmp, "bogus")

	result, err := Run(context.Background(), trackerPrefs("beads"), tmp, pluginRoot)
	if err != nil {
		t.Fatalf("missing tracker source must not error: %v", err)
	}
	if !hasStepContaining(result.Steps, "no adapter shipped") {
		t.Errorf("expected a StepWarn about the missing adapter; steps:\n%v", result.Steps)
	}
	if exists(t, filepath.Join(tmp, ".claude", "rules", "tracker-bogus.md")) {
		t.Error("nothing should be installed for an unshipped adapter")
	}
}

// Board profile is scaffolded once and NEVER overwritten on a later run.
func TestRun_BoardScaffoldOnce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	writeTrackerSource(t, pluginRoot, "planfix-mcp", "0.6.4")
	if err := os.WriteFile(filepath.Join(pluginRoot, "rules", "tracker-planfix-mcp.board.md"),
		[]byte("---\nname: tracker-planfix-mcp.board\nversion: 0.0.0\n---\n# board template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProjectEnv(t, tmp, "planfix-mcp")

	if _, err := Run(context.Background(), trackerPrefs("planfix-mcp"), tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	boardDst := filepath.Join(tmp, ".claude", "rules", "tracker-planfix-mcp.board.md")
	if !exists(t, boardDst) {
		t.Fatal("board profile not scaffolded on first run")
	}
	// User edits the board, runs init again - the edit must survive.
	if err := os.WriteFile(boardDst, []byte("MY EDITED BOARD"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), trackerPrefs("planfix-mcp"), tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(boardDst)
	if string(got) != "MY EDITED BOARD" {
		t.Errorf("board profile was overwritten on re-run; got:\n%s", got)
	}
}

// .mcp.json can carry a tracker adapter's secret token (e.g. PLANFIX_TOKEN for
// planfix-mcp); lets init must gitignore it so a user project can't commit the
// token - the tracker docs promise this. Regression for the branch review's B1.
func TestRun_GitignoresMcpJson(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)
	pluginRoot := setupFakePluginRoot(t)
	if _, err := Run(context.Background(), trackerPrefs("beads"), tmp, pluginRoot); err != nil {
		t.Fatal(err)
	}
	gi, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), ".mcp.json") {
		t.Errorf(".gitignore must contain .mcp.json (secret-bearing MCP config); got:\n%s", gi)
	}
}

func hasStepContaining(steps []Step, sub string) bool {
	for _, s := range steps {
		if strings.Contains(s.Message, sub) {
			return true
		}
	}
	return false
}
