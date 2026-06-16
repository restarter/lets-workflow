package sessionstart

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitInitRepo(t *testing.T) (dir, branch string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("checkout", "-q", "-b", "wt-test")
	run("commit", "-q", "--allow-empty", "-m", "init")
	return dir, "wt-test"
}

func writeTaskFile(t *testing.T, dir, branch, content string) string {
	t.Helper()
	d := filepath.Join(dir, ".lets", "sessions")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, ".task-"+strings.ReplaceAll(branch, "/", "-"))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRefreshSessionBoundary_RefreshesExisting(t *testing.T) {
	dir, branch := gitInitRepo(t)
	p := writeTaskFile(t, dir, branch, "task: lets-x\nstart: abc123\nsession: oldsha oldsid\n")

	if err := RefreshSessionBoundary(dir, "newsid"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := readFile(t, p)
	if !strings.Contains(s, "task: lets-x") || !strings.Contains(s, "start: abc123") {
		t.Errorf("task:/start: not preserved:\n%s", s)
	}
	if !strings.Contains(s, "newsid") {
		t.Errorf("session: not refreshed with new sid:\n%s", s)
	}
	if strings.Contains(s, "oldsid") {
		t.Errorf("old sid still present:\n%s", s)
	}
}

func TestRefreshSessionBoundary_AbsentFileNotCreated(t *testing.T) {
	dir, branch := gitInitRepo(t)
	if err := RefreshSessionBoundary(dir, "sid"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	p := filepath.Join(dir, ".lets", "sessions", ".task-"+branch)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("file should not be created, stat err=%v", err)
	}
}

func TestRefreshSessionBoundary_AppendsWhenNoSessionLine(t *testing.T) {
	dir, branch := gitInitRepo(t)
	p := writeTaskFile(t, dir, branch, "task: lets-y\nstart: def456\n")
	if err := RefreshSessionBoundary(dir, "sid2"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := readFile(t, p)
	if !strings.Contains(s, "task: lets-y") || !strings.Contains(s, "start: def456") {
		t.Errorf("preserved lines missing:\n%s", s)
	}
	if !strings.Contains(s, "session: ") || !strings.Contains(s, "sid2") {
		t.Errorf("session line not appended:\n%s", s)
	}
}

func TestRefreshSessionBoundary_EmptySessionIDNoOp(t *testing.T) {
	dir, branch := gitInitRepo(t)
	orig := "task: lets-z\nstart: ghi\nsession: s1 sid1\n"
	p := writeTaskFile(t, dir, branch, orig)
	if err := RefreshSessionBoundary(dir, ""); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := readFile(t, p); got != orig {
		t.Errorf("file changed on empty sessionID:\ngot:  %q\nwant: %q", got, orig)
	}
}

func TestRefreshSessionBoundary_DetachedHeadNoOp(t *testing.T) {
	dir, branch := gitInitRepo(t)
	p := writeTaskFile(t, dir, branch, "task: lets-x\nstart: s\nsession: oldsha oldsid\n")
	orig := readFile(t, p)
	if out, err := exec.Command("git", "-C", dir, "checkout", "--detach", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("detach: %v\n%s", err, out)
	}
	if err := RefreshSessionBoundary(dir, "newsid"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := readFile(t, p); got != orig {
		t.Errorf("detached HEAD must be a no-op:\ngot:  %q\nwant: %q", got, orig)
	}
	// And no empty-slug file should have been created.
	if _, err := os.Stat(filepath.Join(dir, ".lets", "sessions", ".task-")); !os.IsNotExist(err) {
		t.Errorf("empty-slug .task- file should not exist, stat err=%v", err)
	}
}

func TestRefreshSessionBoundary_ThroughLetsSymlink(t *testing.T) {
	dir, branch := gitInitRepo(t)
	// Mirror the worktree layout: .lets is a symlink to a sibling real dir. This
	// is exactly why atomicWrite uses a same-dir temp+rename (cross-device rename
	// would fail) - the real-dir tests never exercise it.
	realLets := t.TempDir()
	if err := os.MkdirAll(filepath.Join(realLets, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realLets, filepath.Join(dir, ".lets")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	p := filepath.Join(dir, ".lets", "sessions", ".task-"+strings.ReplaceAll(branch, "/", "-"))
	if err := os.WriteFile(p, []byte("task: lets-y\nstart: s2\nsession: oldsha oldsid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RefreshSessionBoundary(dir, "freshsid"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := readFile(t, p)
	if !strings.Contains(s, "freshsid") || strings.Contains(s, "oldsid") {
		t.Errorf("refresh through .lets symlink failed:\n%s", s)
	}
	if !strings.Contains(s, "task: lets-y") || !strings.Contains(s, "start: s2") {
		t.Errorf("preserved lines lost through symlink:\n%s", s)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}
