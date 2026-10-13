//go:build unix

package worktreecmd_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
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

func TestEnsureLetsAdditionalDir_ChangedDuringMergeNotOverwritten(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{{"appeared readable", 0o644}, {"appeared unreadable", 0o000}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mode == 0 && os.Geteuid() == 0 {
				t.Skip("root reads any file")
			}
			repo, wt, _ := settingsRepo(t)
			restore := worktreecmd.SetSettingsStage(func(stage string) {
				if stage == "before-reread" {
					mustWrite(t, settingsPath(wt), `{"model":"x"}`, tc.mode)
				}
			})
			defer restore()
			st, kind := ensure(wt, repo)
			if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalChanged {
				t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalChanged)
			}
			_ = os.Chmod(settingsPath(wt), 0o644)
			if data, _ := os.ReadFile(settingsPath(wt)); string(data) != `{"model":"x"}` {
				t.Errorf("the file another writer created was overwritten: %s", data)
			}
		})
	}
}

func TestEnsureLetsAdditionalDir_FinalIgnoreProbeFailsClosed(t *testing.T) {
	repo, wt, _ := settingsRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restore := worktreecmd.SetSettingsStage(func(stage string) {
		if stage == "after-exclude" {
			cancel()
		}
	})
	defer restore()
	st, kind := worktreecmd.EnsureLetsAdditionalDir(ctx, wt, repo)
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalGitFailed {
		t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, worktreecmd.SettingsLocalGitFailed)
	}
	if _, err := os.Stat(settingsPath(wt)); !os.IsNotExist(err) {
		t.Errorf("settings.local.json was written after a failed ignore probe (err=%v)", err)
	}
}

// Team allow rules (lets-qysjv, ported by lets-gbfpd): EnsureTeamAllowRules shares
// EnsureLetsAdditionalDir's merge core, and writes only while git ignores the file.

// allowImpact is EnsureTeamAllowRules's warning tail.
const allowImpact = " - lets worktree switch and lets worktree task-state will prompt, and auto mode may refuse them; approve them when asked, or add the rules with /permissions"

// allowAdded is EnsureTeamAllowRules's ok message for both rules.
const allowAdded = "added `Bash(lets worktree switch:*)`, `Bash(lets worktree task-state:*)` to .claude/settings.local.json (remove them to revoke)"

func ensureAllow(wt, repo string) (worktreecmd.Step, string) {
	return worktreecmd.EnsureTeamAllowRules(context.Background(), wt, repo)
}

// settingsAllow returns permissions.allow of wt's settings.local.json.
func settingsAllow(t *testing.T, wt string) []string {
	t.Helper()
	data, err := os.ReadFile(settingsPath(wt))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	var s struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("settings.local.json is not JSON: %v\n%s", err, data)
	}
	return s.Permissions.Allow
}

func TestTeamAllowRules_NoMembersRule(t *testing.T) {
	want := []string{"Bash(lets worktree switch:*)", "Bash(lets worktree task-state:*)"}
	if got := worktreecmd.TeamAllowRules(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("TeamAllowRules = %v, want %v (lets members is a deprecated stub - no rule)", got, want)
	}
}

func TestEnsureTeamAllowRules_AddsOnceInOrder(t *testing.T) {
	repo, wt, _ := settingsRepo(t)
	if st, kind := ensureAllow(wt, repo); st.Status != worktreecmd.StepOK || kind != "" || st.Message != allowAdded {
		t.Fatalf("first call: %s %q kind %q, want ok %q", st.Status, st.Message, kind, allowAdded)
	}
	if got, want := settingsAllow(t, wt), worktreecmd.TeamAllowRules(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("allow = %v, want %v", got, want)
	}
	if !settingsIgnored(wt) {
		t.Error("git does not ignore settings.local.json")
	}
	first, _ := os.ReadFile(settingsPath(wt))
	if st, _ := ensureAllow(wt, repo); st.Status != worktreecmd.StepSkip {
		t.Errorf("second call: %s %q", st.Status, st.Message)
	}
	if second, _ := os.ReadFile(settingsPath(wt)); string(second) != string(first) {
		t.Errorf("second call rewrote the file:\n%s\n---\n%s", first, second)
	}
}

func TestEnsureTeamAllowRules_KeepsUserContent(t *testing.T) {
	repo, wt, _ := settingsRepo(t)
	const before = `{"cleanupPeriodDays":9007199254740993,"permissions":{"allow":["Bash(make test && go vet ./...)","Bash(lets worktree switch:*)"],"additionalDirectories":["/elsewhere"]}}`
	mustWrite(t, settingsPath(wt), before, 0o644)
	st, kind := ensureAllow(wt, repo)
	if st.Status != worktreecmd.StepOK || kind != "" {
		t.Fatalf("%s %q kind %q", st.Status, st.Message, kind)
	}
	// The step names only what was appended, never a rule the user already had.
	if want := "added `Bash(lets worktree task-state:*)` to .claude/settings.local.json (remove them to revoke)"; st.Message != want {
		t.Errorf("message = %q, want %q", st.Message, want)
	}
	want := []string{"Bash(make test && go vet ./...)", "Bash(lets worktree switch:*)", "Bash(lets worktree task-state:*)"}
	if got := settingsAllow(t, wt); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("allow = %v, want %v (an existing rule is not duplicated)", got, want)
	}
	if got := settingsDirs(t, wt); len(got) != 1 || got[0] != "/elsewhere" {
		t.Errorf("additionalDirectories = %v, want [/elsewhere]", got)
	}
	data, _ := os.ReadFile(settingsPath(wt))
	for _, keep := range []string{`"Bash(make test && go vet ./...)"`, `9007199254740993`} {
		if !strings.Contains(string(data), keep) {
			t.Errorf("lost, re-escaped or rounded %s:\n%s", keep, data)
		}
	}
}

func TestEnsureTeamAllowRules_LeavesUnusableFilesAlone(t *testing.T) {
	plain := func(body string) func(t *testing.T) (repo, wt string) {
		return func(t *testing.T) (string, string) {
			repo, wt, _ := settingsRepo(t)
			mustWrite(t, settingsPath(wt), body, 0o644)
			return repo, wt
		}
	}
	for _, tc := range []struct {
		name, kind string
		setup      func(t *testing.T) (repo, wt string)
	}{
		{"malformed", worktreecmd.SettingsLocalUnreadable, plain(`{not json`)},
		{"permissions not an object", worktreecmd.SettingsLocalShape, plain(`{"permissions":[]}`)},
		{"allow not an array", worktreecmd.SettingsLocalShape, plain(`{"permissions":{"allow":"x"}}`)},
		{"allow non-string", worktreecmd.SettingsLocalShape, plain(`{"permissions":{"allow":[1]}}`)},
		{"symlinked .claude", worktreecmd.SettingsLocalSymlink, func(t *testing.T) (string, string) {
			repo, wt, _ := settingsRepo(t)
			outside := realTempDir(t)
			mustWrite(t, filepath.Join(outside, "settings.local.json"), `{"model":"x"}`, 0o644)
			if err := os.Symlink(outside, filepath.Join(wt, ".claude")); err != nil {
				t.Fatal(err)
			}
			return repo, wt
		}},
		{"tracked", worktreecmd.SettingsLocalTracked, trackedRepo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, wt := tc.setup(t)
			before, _ := os.ReadFile(settingsPath(wt))
			st, kind := ensureAllow(wt, repo)
			if st.Status != worktreecmd.StepWarn || kind != tc.kind || !strings.HasPrefix(st.Message, tc.kind+": ") || !strings.HasSuffix(st.Message, allowImpact) {
				t.Errorf("step = %s %q kind %q, want warn %s", st.Status, st.Message, kind, tc.kind)
			}
			if after, _ := os.ReadFile(settingsPath(wt)); string(after) != string(before) {
				t.Errorf("file was rewritten:\n%s\n---\n%s", before, after)
			}
		})
	}
}

// Rules in a file git would publish are repository-supplied: a .gitignore negation
// means nothing is written, unlike the additional directory.
func TestEnsureTeamAllowRules_NegatedIgnoreWritesNothing(t *testing.T) {
	repo, wt, _ := settingsRepo(t)
	mustWrite(t, filepath.Join(wt, ".gitignore"), "!.claude/settings.local.json\n", 0o644)
	const body = `{"model":"x"}`
	mustWrite(t, settingsPath(wt), body, 0o644)
	st, kind := ensureAllow(wt, repo)
	want := "settings_local_not_ignored: .claude/settings.local.json left unchanged (still not ignored after info/exclude (a .gitignore negation?), and LETS writes it only while git ignores it)" + allowImpact
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalNotIgnored || st.Message != want {
		t.Errorf("got  %s %q %q\nwant warn %q %q", st.Status, kind, st.Message, worktreecmd.SettingsLocalNotIgnored, want)
	}
	if data, _ := os.ReadFile(settingsPath(wt)); string(data) != body {
		t.Errorf("the file was written: %s", data)
	}
}

func TestEnsureTeamAllowRules_ExcludeFailedWritesNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	repo, wt, _ := settingsRepo(t)
	info := filepath.Join(repo, ".git", "info")
	mustMkdir(t, info)
	if err := os.Chmod(info, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(info, 0o755) })
	st, kind := ensureAllow(wt, repo)
	const prefix = "settings_local_exclude_failed: .claude/settings.local.json left unchanged (could not add it to info/exclude ("
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalExcludeFailed || !strings.HasPrefix(st.Message, prefix) ||
		!strings.HasSuffix(st.Message, "and LETS writes it only while git ignores it)"+allowImpact) {
		t.Errorf("got %s %q %q, want warn %s", st.Status, kind, st.Message, worktreecmd.SettingsLocalExcludeFailed)
	}
	if _, err := os.Stat(settingsPath(wt)); !os.IsNotExist(err) {
		t.Errorf("settings.local.json was written (err=%v)", err)
	}
}

// A repo that tracks other .claude files keeps them; the personal file is ignored and
// both writers land in it.
func TestEnsureSettings_RepoTracksOtherClaudeFiles(t *testing.T) {
	isolateGitIgnore(t)
	repo := initRepo(t)
	const rules = "# a project rule\n"
	mustWrite(t, filepath.Join(repo, ".claude", "rules", "x.md"), rules, 0o644)
	mustWrite(t, filepath.Join(repo, ".claude", "settings.json"), `{"model":"y"}`, 0o644)
	runIn(t, repo, "git", "add", ".claude")
	runIn(t, repo, "git", "commit", "-q", "-m", "project claude files")
	mustMkdir(t, filepath.Join(repo, ".lets"))
	wt := filepath.Join(realTempDir(t), "wt")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "x", wt)

	if st, kind := ensure(wt, repo); st.Status != worktreecmd.StepOK || kind != "" {
		t.Fatalf("additional dir: %s %q kind %q", st.Status, st.Message, kind)
	}
	if st, kind := ensureAllow(wt, repo); st.Status != worktreecmd.StepOK || kind != "" {
		t.Fatalf("allow rules: %s %q kind %q", st.Status, st.Message, kind)
	}
	if out := gitOutput(t, wt, "ls-files", "--", ".claude/settings.local.json"); strings.TrimSpace(out) != "" {
		t.Errorf("settings.local.json is tracked: %q", out)
	}
	if !settingsIgnored(wt) {
		t.Error("git does not ignore settings.local.json")
	}
	if out := gitOutput(t, wt, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("the worktree is not clean:\n%s", out)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, ".claude", "rules", "x.md")); string(b) != rules {
		t.Errorf(".claude/rules/x.md changed: %q", b)
	}
	if len(settingsDirs(t, wt)) != 1 || len(settingsAllow(t, wt)) != len(worktreecmd.TeamAllowRules()) {
		t.Errorf("dirs %v, allow %v", settingsDirs(t, wt), settingsAllow(t, wt))
	}
}

func TestEnsureTeamAllowRules_ChangedDuringMergeNotOverwritten(t *testing.T) {
	repo, wt, _ := settingsRepo(t)
	restore := worktreecmd.SetSettingsStage(func(stage string) {
		if stage == "before-reread" {
			mustWrite(t, settingsPath(wt), `{"model":"x"}`, 0o644)
		}
	})
	defer restore()
	st, kind := ensureAllow(wt, repo)
	const want = "settings_local_changed: .claude/settings.local.json left unchanged (it changed while LETS merged it; add the rules with /permissions)" + allowImpact
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalChanged || st.Message != want {
		t.Errorf("step = %s %q kind %q, want warn %q", st.Status, st.Message, kind, want)
	}
	if data, _ := os.ReadFile(settingsPath(wt)); string(data) != `{"model":"x"}` {
		t.Errorf("the other writer's file was overwritten: %s", data)
	}
}

// Two LETS writers on one worktree - EnsureLetsAdditionalDir and the team grant -
// keep both entries: the merge holds a per-worktree lock from its first read to its
// rename, so the grant waits and merges into the newer file instead of being renamed
// over (lets-qysjv).
func TestMergeSettings_ConcurrentWritersKeepBoth(t *testing.T) {
	for _, at := range []string{"before-reread", "before-write"} {
		t.Run(at, func(t *testing.T) {
			repo, wt, want := settingsRepo(t)
			var fired atomic.Bool // never blocks: the grant's own stage calls pass through
			var wg sync.WaitGroup
			var grant worktreecmd.Step
			restore := worktreecmd.SetSettingsStage(func(stage string) {
				if stage != at || !fired.CompareAndSwap(false, true) {
					return
				}
				started, done := make(chan struct{}), make(chan struct{})
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer close(done)
					close(started)
					grant, _ = worktreecmd.EnsureTeamAllowRules(context.Background(), wt, repo)
				}()
				<-started
				select { // the grant finishes (no lock) or blocks on the lock
				case <-done:
				case <-time.After(time.Second):
				}
			})
			t.Cleanup(restore)
			st, kind := ensure(wt, repo)
			wg.Wait()
			if st.Status != worktreecmd.StepOK || kind != "" {
				t.Errorf("ensure = %s %q kind %q, want ok", st.Status, st.Message, kind)
			}
			if grant.Status != worktreecmd.StepOK {
				t.Errorf("grant = %s %q, want ok", grant.Status, grant.Message)
			}
			if got := settingsDirs(t, wt); !slices.Contains(got, want) {
				t.Errorf("additionalDirectories = %v, want %s", got, want)
			}
			if got := settingsAllow(t, wt); strings.Join(got, "|") != strings.Join(worktreecmd.TeamAllowRules(), "|") {
				t.Errorf("allow = %v, want %v", got, worktreecmd.TeamAllowRules())
			}
		})
	}
}

// A lock another LETS process holds writes nothing: the merge waits until ctx's
// deadline, then warns settings_local_changed.
func TestMergeSettings_HeldLockWritesNothing(t *testing.T) {
	repo, wt, _ := settingsRepo(t)
	gitDir := strings.TrimSpace(gitOutput(t, wt, "rev-parse", "--absolute-git-dir"))
	f, err := os.OpenFile(filepath.Join(gitDir, "lets-settings-local.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fsutil.UnlockFile(f) }()
	short := func() context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		t.Cleanup(cancel)
		return ctx
	}
	const busy = "settings_local_changed: .claude/settings.local.json left unchanged (another LETS process is updating it; the next session start retries) - "
	st, kind := worktreecmd.EnsureLetsAdditionalDir(short(), wt, repo)
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalChanged || !strings.HasPrefix(st.Message, busy) {
		t.Errorf("ensure = %s %q kind %q, want warn %q...", st.Status, st.Message, kind, busy)
	}
	// team-init only - no SessionStart backfill, and a second team-init stops at team_exists - so the grant never promises a retry
	const teamBusy = "settings_local_changed: .claude/settings.local.json left unchanged (another LETS process is updating it; add the rules with /permissions) - "
	st, kind = worktreecmd.EnsureTeamAllowRules(short(), wt, repo)
	if st.Status != worktreecmd.StepWarn || kind != worktreecmd.SettingsLocalChanged || !strings.HasPrefix(st.Message, teamBusy) || strings.Contains(st.Message, "next session start") || strings.Contains(st.Message, "re-run team-init") {
		t.Errorf("grant = %s %q kind %q, want warn %q...", st.Status, st.Message, kind, teamBusy)
	}
	if _, err := os.Lstat(settingsPath(wt)); !os.IsNotExist(err) {
		t.Errorf("settings.local.json was written (err=%v)", err)
	}
}
