package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// TestHookSessionStart_E2E exercises the cobra → sessionstart pipeline with
// the --rules flag. Project root falls through to whatever DetectProjectRoot
// finds at test time - typically the lets-workflow repo root since tests
// run from cli/internal/cli/. We deliberately don't sandbox the project
// root here: assertions only check Contains() so the test tolerates
// whatever .env happens to live at the detected root.
//
// Phase 4b: rules emission was removed. Output is now just the LETS Config
// block (+ optional drift notice when installed rules differ from plugin).
func TestHookSessionStart_E2E(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	// Plugin rules with frontmatter so driftCheck has something to compare.
	if err := os.WriteFile(rulesPath, []byte("---\nversion: 0.4.0\n---\nRULES BODY\n"), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "session-start", "--rules=" + rulesPath})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	// Rules body should NOT be in output (Phase 4b: rules moved to project's
	// .claude/rules/lets-rules.md, hook only emits Config + drift notice).
	if strings.Contains(out, "RULES BODY") {
		t.Errorf("rules body should not be emitted in Phase 4b, got:\n%s", out)
	}
	if !strings.Contains(out, "## LETS Config") {
		t.Errorf("expected LETS Config block (test runs inside git repo), got: %q", out)
	}
	if !strings.Contains(out, "LETS_PROJECT_ROOT=") {
		t.Errorf("expected LETS_PROJECT_ROOT line, got: %q", out)
	}
}

// TestHookSessionStart_PreCompact_OutputParity locks the contract that the
// two hook subcommands produce byte-identical output today (they share
// runHookSessionPipeline). Future PreCompact-specific divergence should
// be intentional - this test forces the change to be visible by failing,
// rather than letting one subcommand silently drift while the other
// stays correct. Closes S15 from the 2026-05-08 review.
func TestHookSessionStart_PreCompact_OutputParity(t *testing.T) {
	// session-start also syncs the global rules cache (lets-tg008); a global copy
	// byte-identical to the plugin rules keeps that sync a silent no-op whatever
	// the host project's LETS_RULES_SCOPE, so the parity below stays meaningful.
	rulesPath := filepath.Join(t.TempDir(), "rules", "lets-rules.md")
	writeTestFile(t, rulesPath, "---\nversion: 0.4.0\n---\n")
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".claude", "rules", "lets-rules.md"), "---\nversion: 0.4.0\n---\n")
	t.Setenv("HOME", home)

	run := func(t *testing.T, sub string) string {
		t.Helper()
		root := cli.NewRootCmd()
		root.SetArgs([]string{"hook", sub, "--rules=" + rulesPath})
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute %s: %v", sub, err)
		}
		return buf.String()
	}

	got1 := run(t, "session-start")
	got2 := run(t, "precompact")

	if got1 != got2 {
		t.Errorf("session-start vs precompact output diverged.\nsession-start:\n%s\nprecompact:\n%s", got1, got2)
	}
}

// TestHookSessionStart_PreCompact_ParityOnUserScopePath pins parity on the
// user-scope branch deterministically (the unsandboxed parity test above
// exercises whatever the host repo's state happens to be): fake home with
// global rules + ~/.lets/.env, fresh git repo without project rules. A parity
// bug specific to the user-scope path (e.g. one subcommand forgets to pass
// homeDir) is exactly the divergence this fixture catches. The global copy is
// byte-identical to the plugin rules so session-start's cache sync (lets-tg008)
// is a silent no-op - that sync is session-start-only by design.
func TestHookSessionStart_PreCompact_ParityOnUserScopePath(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.4.0\n---\n")
	writeTestFile(t, filepath.Join(home, ".lets", ".env"), "LETS_LANGUAGE=Ukrainian\n")
	t.Setenv("HOME", home)

	repo := t.TempDir()
	gitArgs := [][]string{{"init", "-q", "-b", "main"}}
	for _, args := range gitArgs {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	chdirTo(t, repo)

	rulesPath := filepath.Join(t.TempDir(), "rules", "lets-rules.md")
	writeTestFile(t, rulesPath, "---\nversion: 0.4.0\n---\n")

	run := func(t *testing.T, sub string) string {
		t.Helper()
		root := cli.NewRootCmd()
		root.SetArgs([]string{"hook", sub, "--rules=" + rulesPath})
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute %s: %v", sub, err)
		}
		return buf.String()
	}

	got1 := run(t, "session-start")
	got2 := run(t, "precompact")
	if got1 != got2 {
		t.Errorf("session-start vs precompact diverged on the user-scope path.\nsession-start:\n%s\nprecompact:\n%s", got1, got2)
	}
	// Sanity: the user-scope path covers the missing project copy silently.
	if strings.Contains(got1, "LETS Notice") {
		t.Errorf("user-scope path must not nag:\n%s", got1)
	}
	if !strings.Contains(got1, "LETS_LANGUAGE=Ukrainian") {
		t.Errorf("user env overlay missing:\n%s", got1)
	}
}

// writeTestFile mirrors the sessionstart package's writeFile helper for
// cli_test fixtures (mkdir parents + write).
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestHookSessionStart_RulesFlagRequired(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "session-start"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --rules is missing")
	}
	if !strings.Contains(err.Error(), "rules") {
		t.Errorf("error should mention rules flag, got: %v", err)
	}
}

// TestHookSessionStart_SessionRefreshGating pins the safety-critical gating of
// the proactive session-boundary refresh (lets-dsdmp, review B1): it MUST refresh
// the .task file's session: line ONLY on source=startup with a non-empty
// session_id, and leave the file byte-identical on resume/compact/malformed/empty
// stdin. A regression here (flipped source check, dropped filter, mis-parse) would
// silently move the boundary on every resume/compact and corrupt /lets:end's
// session diff - this table makes that fail loudly. task:/start: are always
// preserved.
func TestHookSessionStart_SessionRefreshGating(t *testing.T) {
	git := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	cases := []struct {
		name      string
		stdin     string
		refreshed bool
	}{
		{"startup refreshes", `{"session_id":"0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d","source":"startup"}`, true},
		{"resume untouched", `{"session_id":"0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d","source":"resume"}`, false},
		{"compact untouched", `{"session_id":"0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d","source":"compact"}`, false},
		{"empty session_id no-op", `{"session_id":"","source":"startup"}`, false},
		{"malformed stdin no-op", `not json at all`, false},
		{"empty stdin no-op", ``, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			repo := t.TempDir()
			git(t, repo, "init", "-q", "-b", "wt-test")
			git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
			out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
			if err != nil {
				t.Fatalf("rev-parse: %v", err)
			}
			head := strings.TrimSpace(string(out))
			chdirTo(t, repo)

			taskFile := filepath.Join(repo, ".lets", "sessions", ".task-wt-test")
			writeTestFile(t, taskFile, "task: lets-x\nstart: abc123\nsession: OLDSHA OLDSID\n")

			rulesPath := filepath.Join(t.TempDir(), "rules.md")
			writeTestFile(t, rulesPath, "---\nversion: 0.4.0\n---\n")

			root := cli.NewRootCmd()
			root.SetArgs([]string{"hook", "session-start", "--rules=" + rulesPath})
			root.SetIn(strings.NewReader(tc.stdin))
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetErr(&buf)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}

			got, err := os.ReadFile(taskFile)
			if err != nil {
				t.Fatalf("read task file: %v", err)
			}
			s := string(got)
			if !strings.Contains(s, "task: lets-x") || !strings.Contains(s, "start: abc123") {
				t.Errorf("task:/start: not preserved:\n%s", s)
			}
			if tc.refreshed {
				if !strings.Contains(s, "session: "+head+" 0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d") {
					t.Errorf("expected session refreshed to %q with the new sid, got:\n%s", head, s)
				}
				if strings.Contains(s, "OLDSID") {
					t.Errorf("old sid still present after refresh:\n%s", s)
				}
			} else {
				if !strings.Contains(s, "session: OLDSHA OLDSID") {
					t.Errorf("expected session UNCHANGED, got:\n%s", s)
				}
			}
		})
	}
}

// installTestPlugin lays out an installed plugin release the rules cache trusts
// (<home>/.claude/plugins/cache/<marketplace>/lets/<ver>) and returns its
// rules/lets-rules.md - the --rules path Claude Code passes the hook.
func installTestPlugin(t *testing.T, home, ver string) string {
	t.Helper()
	root := filepath.Join(home, ".claude", "plugins", "cache", "lets-workflow", "lets", ver)
	writeTestFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"lets","version":"`+ver+`"}`)
	rules := filepath.Join(root, "rules", "lets-rules.md")
	writeTestFile(t, rules, "---\nname: lets-rules\nversion: "+ver+"\n---\n\nRULES "+ver+"\n")
	return rules
}

// The global rules cache is synced on EVERY SessionStart source, compact
// included (lets-tg008), and its outcome lands in the Notice. Run outside a
// project, where the Notice is the only output. rulesSyncFn is not reachable
// from package cli_test, so each source is proven by its effect: a pristine
// older global copy is refreshed on every run.
func TestHookSessionStart_RulesSyncRunsOnEverySource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRules := installTestPlugin(t, home, "0.9.1")
	newRules := installTestPlugin(t, home, "0.9.2")
	old, err := os.ReadFile(oldRules)
	if err != nil {
		t.Fatal(err)
	}
	chdirTo(t, t.TempDir()) // not a git project
	global := filepath.Join(home, ".claude", "rules", "lets-rules.md")

	for _, src := range []string{"startup", "resume", "clear", "compact"} {
		writeTestFile(t, global, string(old))
		root := cli.NewRootCmd()
		root.SetArgs([]string{"hook", "session-start", "--rules=" + newRules})
		root.SetIn(strings.NewReader(`{"source":"` + src + `"}`))
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		if err := root.Execute(); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		out := buf.String()
		if !strings.Contains(out, "## LETS Notice") || !strings.Contains(out, "refreshed v0.9.1 -> v0.9.2") || strings.Contains(out, "## LETS Config") {
			t.Errorf("%s: want only the refresh Notice, got:\n%s", src, out)
		}
		if got, _ := os.ReadFile(global); !bytes.Contains(got, []byte("RULES 0.9.2")) {
			t.Errorf("%s: global copy not refreshed:\n%s", src, got)
		}
	}
}

// The sync runs AFTER the self-heal: in the first session of an unlinked
// worktree of a LETS_RULES_SCOPE=user project, only a linked .lets/.env makes
// the sync see the user scope and create the missing global copy. Proven by
// effect (package cli_test cannot swap the hooks): a sync that ran first would
// see no project and create nothing.
func TestHookSessionStart_RulesSyncRunsAfterSelfHeal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-heal is unix-only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	rules := installTestPlugin(t, home, "0.9.2")

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, wt := filepath.Join(base, "repo"), filepath.Join(base, "wt")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "init")
	writeTestFile(t, filepath.Join(repo, ".lets", ".env"), "LETS_TRACKER=none\nLETS_RULES_SCOPE=user\n")
	git("worktree", "add", "-q", "-b", "feature/x", wt)
	chdirTo(t, wt)

	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "session-start", "--rules=" + rules})
	root.SetIn(strings.NewReader(`{"source":"startup"}`))
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if fi, err := os.Lstat(filepath.Join(wt, ".lets")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("self-heal did not link .lets: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Global workflow rules installed") {
		t.Errorf("sync must run after self-heal and create the global copy:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "rules", "lets-rules.md")); err != nil {
		t.Errorf("global copy not created: %v", err)
	}
}
