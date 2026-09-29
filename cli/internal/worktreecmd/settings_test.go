//go:build unix

package worktreecmd_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// isolateGitIgnore removes every machine-wide git ignore source - the per-user
// excludes file, a global and a system config - so the info/exclude assertions test
// LETS's own write.
func isolateGitIgnore(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", realTempDir(t))
	t.Setenv("XDG_CONFIG_HOME", realTempDir(t))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// settingsRepo returns a main checkout with a .lets/ dir, a linked worktree on branch
// x, and the path EnsureLetsAdditionalDir must declare.
func settingsRepo(t *testing.T) (repo, wt, want string) {
	t.Helper()
	isolateGitIgnore(t)
	repo = initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets"))
	wt = filepath.Join(realTempDir(t), "wt")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "x", wt)
	return repo, wt, filepath.Join(repo, ".lets")
}

func settingsPath(wt string) string { return filepath.Join(wt, ".claude", "settings.local.json") }

// settingsDirs returns permissions.additionalDirectories of wt's settings.local.json.
func settingsDirs(t *testing.T, wt string) []string {
	t.Helper()
	data, err := os.ReadFile(settingsPath(wt))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	var s struct {
		Permissions struct {
			AdditionalDirectories []string `json:"additionalDirectories"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("settings.local.json is not JSON: %v\n%s", err, data)
	}
	return s.Permissions.AdditionalDirectories
}

// settingsIgnored reports whether git in wt ignores .claude/settings.local.json.
func settingsIgnored(wt string) bool {
	return exec.Command("git", "-C", wt, "check-ignore", "-q", ".claude/settings.local.json").Run() == nil
}

func ensure(wt, repo string) (worktreecmd.Step, string) {
	return worktreecmd.EnsureLetsAdditionalDir(context.Background(), wt, repo)
}

func TestEnsureLetsAdditionalDir_CreatesFileAndIgnoresIt(t *testing.T) {
	repo, wt, want := settingsRepo(t)
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepOK {
		t.Fatalf("status = %s: %s", st.Status, st.Message)
	}
	if got := settingsDirs(t, wt); len(got) != 1 || got[0] != want {
		t.Errorf("additionalDirectories = %v, want [%s]", got, want)
	}
	excl, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if !strings.Contains(string(excl), ".claude/settings.local.json") {
		t.Errorf("info/exclude does not list settings.local.json:\n%s", excl)
	}
	if !settingsIgnored(wt) {
		t.Errorf("git does not ignore settings.local.json in the worktree")
	}
}

func TestEnsureLetsAdditionalDir_KeepsExistingContent(t *testing.T) {
	repo, wt, want := settingsRepo(t)
	const before = `{"cleanupPeriodDays":9007199254740993,"permissions":{"allow":["Bash(make test && go vet ./...)"],"additionalDirectories":["/elsewhere"]},"statusLine":{"type":"command","command":"lets statusline --light"}}`
	mustWrite(t, settingsPath(wt), before, 0o644)
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepOK {
		t.Fatalf("status = %s: %s", st.Status, st.Message)
	}
	if got := settingsDirs(t, wt); len(got) != 2 || got[0] != "/elsewhere" || got[1] != want {
		t.Errorf("additionalDirectories = %v, want [/elsewhere %s]", got, want)
	}
	data, _ := os.ReadFile(settingsPath(wt))
	for _, keep := range []string{`"Bash(make test && go vet ./...)"`, `"lets statusline --light"`, `9007199254740993`} {
		if !strings.Contains(string(data), keep) {
			t.Errorf("lost, re-escaped or rounded %s:\n%s", keep, data)
		}
	}
}

func TestEnsureLetsAdditionalDir_IdempotentAndResolving(t *testing.T) {
	repo, wt, want := settingsRepo(t)
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepOK {
		t.Fatalf("first call: status = %s: %s", st.Status, st.Message)
	}
	first, _ := os.ReadFile(settingsPath(wt))
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepSkip {
		t.Errorf("second call: status = %s: %s", st.Status, st.Message)
	}
	if second, _ := os.ReadFile(settingsPath(wt)); string(second) != string(first) {
		t.Errorf("second call rewrote the file:\n%s\n---\n%s", first, second)
	}

	// An entry reaching the same directory through a symlink counts as present.
	alias := filepath.Join(realTempDir(t), "lets-alias")
	if err := os.Symlink(want, alias); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, settingsPath(wt), `{"permissions":{"additionalDirectories":["`+alias+`"]}}`, 0o644)
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepSkip {
		t.Errorf("symlinked entry: status = %s: %s", st.Status, st.Message)
	}

	// A relative entry resolves against the worktree root, not the process cwd.
	rel, err := filepath.Rel(wt, want)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, settingsPath(wt), `{"permissions":{"additionalDirectories":["`+rel+`"]}}`, 0o644)
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepSkip {
		t.Errorf("relative entry: status = %s: %s", st.Status, st.Message)
	}
}

// Codex #2: an entry that is already there does not excuse a file git does not ignore.
func TestEnsureLetsAdditionalDir_PresentEntryStillIgnored(t *testing.T) {
	repo, wt, want := settingsRepo(t)
	body := `{"permissions":{"additionalDirectories":["` + want + `"]}}`
	mustWrite(t, settingsPath(wt), body, 0o644)
	if settingsIgnored(wt) {
		t.Fatal("precondition: the file must not be ignored yet")
	}
	if st, _ := ensure(wt, repo); st.Status != worktreecmd.StepOK || !strings.Contains(st.Message, "info/exclude") {
		t.Errorf("step = %s %q, want ok naming info/exclude", st.Status, st.Message)
	}
	if !settingsIgnored(wt) {
		t.Errorf("settings.local.json is still not ignored")
	}
	if data, _ := os.ReadFile(settingsPath(wt)); string(data) != body {
		t.Errorf("the settings file was rewritten: %s", data)
	}
}

// Codex #2: a .gitignore negation beats info/exclude - the entry is still written, the
// exposure is reported.
func TestEnsureLetsAdditionalDir_GitignoreNegationReported(t *testing.T) {
	repo, wt, want := settingsRepo(t)
	mustWrite(t, filepath.Join(wt, ".gitignore"), "!.claude/settings.local.json\n", 0o644)
	st, kind := ensure(wt, repo)
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalNotIgnored {
		t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalNotIgnored)
	}
	if got := settingsDirs(t, wt); len(got) != 1 || got[0] != want {
		t.Errorf("additionalDirectories = %v, want [%s]", got, want)
	}
}

func TestEnsureLetsAdditionalDir_LeavesUnusableFilesAlone(t *testing.T) {
	for _, tc := range []struct{ name, body, kind string }{
		{"malformed", `{not json`, worktreecmd.SettingsLocalUnreadable},
		{"null", `null`, worktreecmd.SettingsLocalUnreadable},
		{"array", `[]`, worktreecmd.SettingsLocalUnreadable},
		{"trailing data", `{} {}`, worktreecmd.SettingsLocalUnreadable},
		{"permissions not an object", `{"permissions":[]}`, worktreecmd.SettingsLocalShape},
		{"permissions null", `{"permissions":null}`, worktreecmd.SettingsLocalShape},
		{"dirs not an array", `{"permissions":{"additionalDirectories":"x"}}`, worktreecmd.SettingsLocalShape},
		{"dirs null", `{"permissions":{"additionalDirectories":null}}`, worktreecmd.SettingsLocalShape},
		{"dirs non-string", `{"permissions":{"additionalDirectories":[1]}}`, worktreecmd.SettingsLocalShape},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, wt, _ := settingsRepo(t)
			mustWrite(t, settingsPath(wt), tc.body, 0o644)
			st, kind := ensure(wt, repo)
			if st.Status != worktreecmd.StepWarn || kind != tc.kind || !strings.Contains(st.Message, tc.kind) {
				t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, tc.kind)
			}
			if data, _ := os.ReadFile(settingsPath(wt)); string(data) != tc.body {
				t.Errorf("file was rewritten: %s", data)
			}
		})
	}
}

// Team review BLOCKER: never write through a symlinked .claude/ or leaf.
func TestEnsureLetsAdditionalDir_RefusesSymlinks(t *testing.T) {
	t.Run("symlinked .claude dir", func(t *testing.T) {
		repo, wt, _ := settingsRepo(t)
		outside := realTempDir(t)
		if err := os.Symlink(outside, filepath.Join(wt, ".claude")); err != nil {
			t.Fatal(err)
		}
		if st, kind := ensure(wt, repo); st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalSymlink {
			t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalSymlink)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("wrote through the symlink into %s: %v", outside, entries)
		}
	})
	t.Run("symlinked leaf", func(t *testing.T) {
		repo, wt, _ := settingsRepo(t)
		shared := filepath.Join(realTempDir(t), "shared.json")
		mustWrite(t, shared, "{}\n", 0o644)
		mustMkdir(t, filepath.Join(wt, ".claude"))
		if err := os.Symlink(shared, settingsPath(wt)); err != nil {
			t.Fatal(err)
		}
		if st, kind := ensure(wt, repo); st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalSymlink {
			t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalSymlink)
		}
		if fi, err := os.Lstat(settingsPath(wt)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the leaf symlink was replaced (err=%v)", err)
		}
		if data, _ := os.ReadFile(shared); string(data) != "{}\n" {
			t.Errorf("the link target was rewritten: %s", data)
		}
	})
}

// trackedRepo returns a worktree whose branch tracks .claude/settings.local.json = "{}\n".
func trackedRepo(t *testing.T) (repo, wt string) {
	t.Helper()
	isolateGitIgnore(t)
	repo = initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets"))
	mustWrite(t, filepath.Join(repo, ".claude", "settings.local.json"), "{}\n", 0o644)
	runIn(t, repo, "git", "add", "-f", ".claude/settings.local.json")
	runIn(t, repo, "git", "commit", "-q", "-m", "track it")
	wt = filepath.Join(realTempDir(t), "wt")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "x", wt)
	return repo, wt
}

func TestEnsureLetsAdditionalDir_TrackedFileLeftAlone(t *testing.T) {
	repo, wt := trackedRepo(t)
	if st, kind := ensure(wt, repo); st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalTracked {
		t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalTracked)
	}
	if data, _ := os.ReadFile(settingsPath(wt)); string(data) != "{}\n" {
		t.Errorf("tracked file was rewritten: %s", data)
	}
}

// Skeptic C6 / Codex #1: a git failure is never read as "untracked".
func TestEnsureLetsAdditionalDir_GitFailureFailsClosed(t *testing.T) {
	repo, wt := trackedRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if st, kind := worktreecmd.EnsureLetsAdditionalDir(ctx, wt, repo); st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalGitFailed {
		t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalGitFailed)
	}
	if data, _ := os.ReadFile(settingsPath(wt)); string(data) != "{}\n" {
		t.Errorf("file was rewritten under a failed git probe: %s", data)
	}
}

func TestEnsureLetsAdditionalDir_PresentEntryNegatedMessage(t *testing.T) {
	repo, wt, want := settingsRepo(t)
	mustWrite(t, filepath.Join(wt, ".gitignore"), "!.claude/settings.local.json\n", 0o644)
	mustWrite(t, settingsPath(wt), `{"permissions":{"additionalDirectories":["`+want+`"]}}`, 0o644)
	st, kind := ensure(wt, repo)
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalNotIgnored {
		t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalNotIgnored)
	}
	if strings.Contains(st.Message, "added it to info/exclude") {
		t.Errorf("the message claims the exclude worked: %q", st.Message)
	}
}

func TestEnsureLetsAdditionalDir_ExcludeWriteFailed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	repo, wt, want := settingsRepo(t)
	info := filepath.Join(repo, ".git", "info")
	mustMkdir(t, info)
	if err := os.Chmod(info, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(info, 0o755) })
	st, kind := ensure(wt, repo)
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalExcludeFailed {
		t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalExcludeFailed)
	}
	if got := settingsDirs(t, wt); len(got) != 1 || got[0] != want {
		t.Errorf("additionalDirectories = %v, want [%s]", got, want)
	}
}
