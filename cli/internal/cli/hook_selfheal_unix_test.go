//go:build unix

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

func healGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func healWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// healRepo makes a main checkout (initialized as a LETS project when initialized)
// and a linked worktree at <tmp>/wt on feature/x, with no .lets link.
func healRepo(t *testing.T, initialized bool) (repo, wt string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo = filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	healGit(t, repo, "init", "-q", "-b", "main")
	healGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	if initialized {
		healWrite(t, filepath.Join(repo, ".lets", ".env"), "LETS_TRACKER=none\n")
	}
	wt = filepath.Join(base, "wt")
	healGit(t, repo, "worktree", "add", "-q", "-b", "feature/x", wt)
	return repo, wt
}

func runSessionStart(t *testing.T, cwd, payload string) string {
	t.Helper()
	t.Chdir(cwd)
	root := NewRootCmd()
	root.SetArgs([]string{"hook", "session-start", "--rules=" + filepath.Join(t.TempDir(), "rules", "lets-rules.md")})
	root.SetIn(strings.NewReader(payload))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("session-start: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestSelfHeal_AdoptsBeforeConfig(t *testing.T) {
	repo, wt := healRepo(t, true)
	healWrite(t, filepath.Join(wt, ".lets", "cache", "usage"), "x") // the statusline raced adopt

	out := runSessionStart(t, wt, `{"session_id":"0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d","source":"startup"}`)

	fi, err := os.Lstat(filepath.Join(wt, ".lets"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".lets is not a symlink after self-heal (err=%v)\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(wt, ".lets.pre-adopt", "cache", "usage")); err != nil {
		t.Errorf("cache-only .lets was not moved aside: %v", err)
	}
	if !strings.Contains(out, "LETS_TRACKER=none") {
		t.Errorf("LETS Config must come from the main checkout's .lets/.env:\n%s", out)
	}
	if strings.Contains(out, "adopt failed") || strings.Contains(out, "adopt skipped") {
		t.Errorf("a successful adopt must emit no Notice:\n%s", out)
	}
	_ = repo
}

func TestSelfHeal_SkipsUninitializedRepo(t *testing.T) {
	repo, wt := healRepo(t, false)
	// a main .lets holding only a statusline cache is not an initialized project
	healWrite(t, filepath.Join(repo, ".lets", "cache", "usage"), "x")
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	before, _ := os.ReadFile(exclude)

	runSessionStart(t, wt, `{"source":"startup"}`)

	if _, err := os.Lstat(filepath.Join(wt, ".lets")); !os.IsNotExist(err) {
		t.Errorf("uninitialized repo: worktree .lets must not exist (err=%v)", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".lets", "locks")); !os.IsNotExist(err) {
		t.Errorf("uninitialized repo: no lock file may be created in main (err=%v)", err)
	}
	if after, _ := os.ReadFile(exclude); !bytes.Equal(before, after) {
		t.Errorf("info/exclude changed:\n%s", after)
	}
}

func TestSelfHeal_SkipsAgentWorktree(t *testing.T) {
	repo, _ := healRepo(t, true)
	agent := filepath.Join(repo, ".claude", "worktrees", "agent-1")
	healGit(t, repo, "worktree", "add", "-q", "-b", "agent-1", agent)
	if got := selfHeal(agent, ""); got != "" {
		t.Errorf("agent worktree: notice %q", got)
	}
	if _, err := os.Lstat(filepath.Join(agent, ".lets")); !os.IsNotExist(err) {
		t.Errorf("agent worktree must be left alone (err=%v)", err)
	}
}

func TestSelfHeal_OnlyOnStartResumeClear(t *testing.T) {
	_, wt := healRepo(t, true)
	calls := 0
	old := selfHealFn
	selfHealFn = func(string, string) string { calls++; return "" }
	t.Cleanup(func() { selfHealFn = old })

	runSessionStart(t, wt, `{"source":"compact"}`)
	t.Chdir(wt)
	pre := NewRootCmd()
	pre.SetArgs([]string{"hook", "precompact", "--rules=" + filepath.Join(t.TempDir(), "lets-rules.md")})
	pre.SetIn(strings.NewReader(`{"source":"startup"}`))
	pre.SetOut(&bytes.Buffer{})
	if err := pre.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("compact / precompact must never self-heal, got %d calls", calls)
	}
	for _, src := range []string{"startup", "resume", "clear"} {
		runSessionStart(t, wt, `{"source":"`+src+`"}`)
	}
	if calls != 3 {
		t.Errorf("startup/resume/clear must each self-heal once, got %d calls", calls)
	}
}

func TestSelfHeal_LockBusyTimesOut(t *testing.T) {
	repo, wt := healRepo(t, true)
	lockDir := filepath.Join(repo, ".lets", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(lockDir, "adopt.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	old := selfHealLockDeadline
	selfHealLockDeadline = 200 * time.Millisecond
	t.Cleanup(func() { selfHealLockDeadline = old })

	start := time.Now()
	got := selfHeal(wt, "")
	if !strings.Contains(got, "adopt skipped") || !strings.Contains(got, "adopt.lock") {
		t.Errorf("notice = %q, want the adopt-skipped timeout naming the lock", got)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("self-heal waited %s, want about the deadline", d)
	}
	if fi, err := os.Lstat(filepath.Join(wt, ".lets")); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		t.Error("nothing may be linked while another adopt holds the lock")
	}
}
