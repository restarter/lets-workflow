package sessionstart

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// newSID is a real session-id shape: taskstate refuses anything else.
const newSID = "2f942be4-23e0-4b17-9eab-df0a4a7298f2"

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

	if o, _ := RefreshSessionBoundary(dir, newSID, taskstate.SessionGuard{}); o != OutcomeWritten {
		t.Fatalf("outcome = %v, want OutcomeWritten", o)
	}
	s := readFile(t, p)
	if !strings.Contains(s, "task: lets-x") || !strings.Contains(s, "start: abc123") {
		t.Errorf("task:/start: not preserved:\n%s", s)
	}
	if !strings.Contains(s, newSID) {
		t.Errorf("session: not refreshed with new sid:\n%s", s)
	}
	if strings.Contains(s, "oldsid") {
		t.Errorf("old sid still present:\n%s", s)
	}
}

func TestRefreshSessionBoundary_AbsentFileNotCreated(t *testing.T) {
	dir, branch := gitInitRepo(t)
	if o, _ := RefreshSessionBoundary(dir, newSID, taskstate.SessionGuard{}); o != OutcomeSkipped {
		t.Fatalf("outcome = %v, want OutcomeSkipped", o)
	}
	p := filepath.Join(dir, ".lets", "sessions", ".task-"+branch)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("file should not be created, stat err=%v", err)
	}
}

func TestRefreshSessionBoundary_AppendsWhenNoSessionLine(t *testing.T) {
	dir, branch := gitInitRepo(t)
	p := writeTaskFile(t, dir, branch, "task: lets-y\nstart: def456\n")
	if o, _ := RefreshSessionBoundary(dir, newSID, taskstate.SessionGuard{}); o != OutcomeWritten {
		t.Fatalf("outcome = %v, want OutcomeWritten", o)
	}
	s := readFile(t, p)
	if !strings.Contains(s, "task: lets-y") || !strings.Contains(s, "start: def456") {
		t.Errorf("preserved lines missing:\n%s", s)
	}
	if !strings.Contains(s, "session: ") || !strings.Contains(s, newSID) {
		t.Errorf("session line not appended:\n%s", s)
	}
}

func TestRefreshSessionBoundary_EmptySessionIDNoOp(t *testing.T) {
	dir, branch := gitInitRepo(t)
	orig := "task: lets-z\nstart: ghi\nsession: s1 sid1\n"
	p := writeTaskFile(t, dir, branch, orig)
	if o, _ := RefreshSessionBoundary(dir, "", taskstate.SessionGuard{}); o != OutcomeSkipped {
		t.Fatalf("outcome = %v, want OutcomeSkipped", o)
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
	if o, _ := RefreshSessionBoundary(dir, newSID, taskstate.SessionGuard{}); o != OutcomeSkipped {
		t.Fatalf("outcome = %v, want OutcomeSkipped", o)
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
	if o, _ := RefreshSessionBoundary(dir, newSID, taskstate.SessionGuard{}); o != OutcomeWritten {
		t.Fatalf("outcome = %v, want OutcomeWritten", o)
	}
	s := readFile(t, p)
	if !strings.Contains(s, newSID) || strings.Contains(s, "oldsid") {
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

func TestRefreshSessionBoundary_KeepsOrcAndUnknownLines(t *testing.T) {
	dir, branch := gitInitRepo(t)
	p := writeTaskFile(t, dir, branch, "task: lets-x\norc: MAIN-PWA\nfuture: kept\nsession: oldsha oldsid\n")
	if o, _ := RefreshSessionBoundary(dir, newSID, taskstate.SessionGuard{}); o != OutcomeWritten {
		t.Fatalf("outcome = %v, want OutcomeWritten", o)
	}
	s := readFile(t, p)
	for _, want := range []string{"task: lets-x", "orc: MAIN-PWA", "future: kept", newSID} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q after refresh:\n%s", want, s)
		}
	}
}

func TestRefreshSessionBoundary_MalformedSIDNotWritten(t *testing.T) {
	dir, branch := gitInitRepo(t)
	orig := "task: lets-x\nsession: oldsha oldsid\n"
	p := writeTaskFile(t, dir, branch, orig)
	if o, _ := RefreshSessionBoundary(dir, "not a session id", taskstate.SessionGuard{}); o != OutcomeSkipped {
		t.Fatalf("outcome = %v, want OutcomeSkipped", o)
	}
	if got := readFile(t, p); got != orig {
		t.Errorf("a malformed sid must not be written:\n%s", got)
	}
}

func TestRefreshSessionBoundary_NonLetsRepoUntouched(t *testing.T) {
	dir, _ := gitInitRepo(t)
	if o, _ := RefreshSessionBoundary(dir, "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d", taskstate.SessionGuard{}); o != OutcomeSkipped {
		t.Fatalf("outcome = %v in a repo without .lets, want OutcomeSkipped", o)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".lets")); !os.IsNotExist(err) {
		t.Errorf("a new session in a repo without .lets must not create it (err=%v)", err)
	}
}

const (
	sidOwn     = "aaaaaaaa-0000-4000-8000-000000000001"
	sidForeign = "bbbbbbbb-0000-4000-8000-000000000002"
)

// guardFor is a guard over a fixed world: alive sids, and rotations old -> new.
func guardFor(alive map[string]bool, rotated map[string]string) taskstate.SessionGuard {
	return taskstate.SessionGuard{
		Liveness: func(sid string) taskstate.Liveness {
			if alive[sid] {
				return taskstate.LiveAlive
			}
			return taskstate.LiveDead
		},
		Rotated: func(sid string) (string, bool) {
			to, ok := rotated[sid]
			return to, ok
		},
	}
}

// A teammate pane runs the same hook: a live foreign holder is never overwritten,
// and the refusal says so (holder + remedy); a dead holder is replaced.
func TestRefresh_ForeignLiveUntouched(t *testing.T) {
	dir, branch := gitInitRepo(t)
	body := "task: lets-x\nstart: abc123\nsession: 1111111 " + sidForeign + "\n"
	p := writeTaskFile(t, dir, branch, body)
	o, notice := RefreshSessionBoundary(dir, sidOwn, guardFor(map[string]bool{sidForeign: true}, nil))
	if o != OutcomeHeld || readFile(t, p) != body {
		t.Fatalf("outcome=%v file:\n%s", o, readFile(t, p))
	}
	if !strings.Contains(notice, "bbbbbbbb") || !strings.Contains(notice, "run /lets:start in that chat to rewrite the session") {
		t.Errorf("notice must name the holder and the remedy: %q", notice)
	}
	o, _ = RefreshSessionBoundary(dir, sidOwn, guardFor(nil, nil))
	if o != OutcomeWritten || !strings.Contains(readFile(t, p), sidOwn) {
		t.Errorf("dead holder: outcome=%v file:\n%s", o, readFile(t, p))
	}
}

// The decision is made under the task lock on the state it will overwrite: a
// foreign session recorded while the refresh waited for the lock is not clobbered.
func TestRefresh_InsideDerive(t *testing.T) {
	dir, branch := gitInitRepo(t)
	p := writeTaskFile(t, dir, branch, "task: lets-x\n")
	lockDir := filepath.Join(dir, ".lets", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(lockDir, "task-"+branch+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	type res struct {
		o Outcome
	}
	done := make(chan res)
	go func() {
		o, _ := RefreshSessionBoundary(dir, sidOwn, guardFor(map[string]bool{sidForeign: true}, nil))
		done <- res{o}
	}()
	time.Sleep(200 * time.Millisecond)
	body := "task: lets-x\nsession: 1111111 " + sidForeign + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = fsutil.UnlockFile(f)
	if r := <-done; r.o != OutcomeHeld || readFile(t, p) != body {
		t.Errorf("outcome=%v file:\n%s", r.o, readFile(t, p))
	}
}

// A busy task lock past the deadline is held + Notice, never a blind write.
func TestRefresh_LockBusyHeld(t *testing.T) {
	dir, branch := gitInitRepo(t)
	body := "task: lets-x\n"
	p := writeTaskFile(t, dir, branch, body)
	lockDir := filepath.Join(dir, ".lets", "locks")
	_ = os.MkdirAll(lockDir, 0o755)
	f, err := os.OpenFile(filepath.Join(lockDir, "task-"+branch+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	o, notice := RefreshSessionBoundary(dir, sidOwn, guardFor(nil, nil))
	if o != OutcomeBusy || notice == "" || readFile(t, p) != body {
		t.Errorf("outcome=%v notice=%q", o, notice)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("waited %v, want about 1s", d)
	}
}

// /clear carries the boundary (sha kept) ONLY on proof the recorded id was
// re-minted into this one; no proof writes nothing and names the gap; a foreign
// live holder is held.
func TestCarry_ProvenOnly(t *testing.T) {
	dir, branch := gitInitRepo(t)
	body := "task: lets-x\nsession: 1111111 " + sidForeign + "\n"
	p := writeTaskFile(t, dir, branch, body)

	o, notice := CarrySession(dir, sidOwn, guardFor(nil, nil))
	if o != OutcomeNoProof || readFile(t, p) != body || !strings.Contains(notice, "no peer role file") {
		t.Errorf("no proof: outcome=%v notice=%q file:\n%s", o, notice, readFile(t, p))
	}
	o, _ = CarrySession(dir, sidOwn, guardFor(map[string]bool{sidForeign: true}, nil))
	if o != OutcomeHeld || readFile(t, p) != body {
		t.Errorf("foreign live: outcome=%v", o)
	}
	o, notice = CarrySession(dir, sidOwn, guardFor(nil, map[string]string{sidForeign: sidOwn}))
	if o != OutcomeWritten || notice == "" || !strings.Contains(readFile(t, p), "session: 1111111 "+sidOwn) {
		t.Errorf("proven: outcome=%v file:\n%s", o, readFile(t, p))
	}
	if o, _ = CarrySession(dir, sidOwn, guardFor(nil, nil)); o != OutcomeUnchanged {
		t.Errorf("already own: outcome=%v", o)
	}
}
