package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	if err := os.WriteFile(rulesPath, []byte("---\nversion: 0.4.0\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
// NEW user-scope branch deterministically (the unsandboxed parity test above
// exercises whatever the host repo's state happens to be): fake home with
// DRIFTED global rules + ~/.lets/.env, fresh git repo without project rules.
// A parity bug specific to the user-scope path (e.g. one subcommand forgets
// to pass homeDir) is exactly the divergence this fixture catches.
func TestHookSessionStart_PreCompact_ParityOnUserScopePath(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".claude", "rules", "lets-rules.md"),
		"---\nversion: 0.3.0\n---\n")
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

	rulesPath := filepath.Join(t.TempDir(), "rules.md")
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
	// Sanity: the fixture actually exercised the new branch.
	if !strings.Contains(got1, "Global workflow rules outdated") {
		t.Errorf("fixture did not hit the user-scope notice branch:\n%s", got1)
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
		{"startup refreshes", `{"session_id":"NEWSID","source":"startup"}`, true},
		{"resume untouched", `{"session_id":"NEWSID","source":"resume"}`, false},
		{"compact untouched", `{"session_id":"NEWSID","source":"compact"}`, false},
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
				if !strings.Contains(s, "session: "+head+" NEWSID") {
					t.Errorf("expected session refreshed to %q NEWSID, got:\n%s", head, s)
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
