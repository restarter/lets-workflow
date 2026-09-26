//go:build unix

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
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
	defer func() { _ = f.Close() }()
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

func TestSessionStart_PeersHealOnClearNotCompact(t *testing.T) {
	dir := t.TempDir()
	var got []string
	old := peersHealFn
	peersHealFn = func(root, sid string) { got = append(got, sid) }
	t.Cleanup(func() { peersHealFn = old })
	const sid = "aaaaaaaa-0000-4000-8000-000000000001"
	runSessionStart(t, dir, `{"source":"clear","session_id":"`+sid+`"}`)
	runSessionStart(t, dir, `{"source":"compact","session_id":"`+sid+`"}`)
	if len(got) != 1 || got[0] != sid {
		t.Errorf("peers heal must run on clear, not compact; got %v", got)
	}
}

const (
	gSidLead = "aaaaaaaa-0000-4000-8000-000000000001"
	gSidPane = "bbbbbbbb-0000-4000-8000-000000000002"
	gSidNew  = "cccccccc-0000-4000-8000-000000000003"
)

// guardWorld is a fake Claude Code session registry for the session-owner guard.
type guardWorld struct {
	t     *testing.T
	dir   string
	alive map[int]bool
}

func newGuardWorld(t *testing.T) *guardWorld {
	t.Helper()
	w := &guardWorld{t: t, dir: t.TempDir(), alive: map[int]bool{}}
	if err := os.MkdirAll(filepath.Join(w.dir, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return w.dir }
	ccregistry.ProcAlive = func(pid int) bool { return w.alive[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
	return w
}

func (w *guardWorld) put(pid int, sid, name, cwd string, started time.Time) {
	w.t.Helper()
	b, _ := json.Marshal(map[string]any{"sessionId": sid, "name": name, "cwd": cwd, "peerProtocol": 1, "status": "idle", "startedAt": started.UnixMilli()})
	if err := os.WriteFile(filepath.Join(w.dir, "sessions", fmt.Sprintf("%d.json", pid)), b, 0o600); err != nil {
		w.t.Fatal(err)
	}
	w.alive[pid] = true
}

func (w *guardWorld) kill(pid int) {
	_ = os.Remove(filepath.Join(w.dir, "sessions", fmt.Sprintf("%d.json", pid)))
	delete(w.alive, pid)
}

// guardRepo is a checkout on feature/x whose task-state file records body; it
// returns the repo, HEAD and the task-state path.
func guardRepo(t *testing.T, body string) (repo, head, taskFile string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	healGit(t, repo, "init", "-q", "-b", "feature/x")
	healGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	out, _ := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	head = strings.TrimSpace(string(out))
	taskFile = filepath.Join(repo, ".lets", "sessions", ".task-feature-x")
	healWrite(t, taskFile, body)
	return repo, head, taskFile
}

func readString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A teammate pane in the lead's worktree runs the same hook: the lead's live
// boundary is not overwritten and the pane is told so; once the lead has ended,
// a new session takes the boundary as before.
func TestHookSessionStart_HeldNotice(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidLead + "\n"
	repo, head, taskFile := guardRepo(t, body)
	w.put(100, gSidLead, "lead", repo, time.Now().Add(-time.Hour))
	w.put(200, gSidPane, "snake-architect", repo, time.Now())

	out := runSessionStart(t, repo, `{"session_id":"`+gSidPane+`","source":"startup"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("a live foreign boundary was overwritten:\n%s", got)
	}
	if !strings.Contains(out, "held by live session aaaaaaaa") {
		t.Errorf("no held Notice:\n%s", out)
	}
	w.kill(100)
	runSessionStart(t, repo, `{"session_id":"`+gSidPane+`","source":"startup"}`)
	if got := readString(t, taskFile); !strings.Contains(got, "session: "+head+" "+gSidPane) {
		t.Errorf("a dead holder must be replaced:\n%s", got)
	}
}

// /clear re-mints the id in the same process: with the old id's role file as proof
// the boundary is carried (sha kept), and peersHeal still moves that role file to
// the new id afterwards - the carry read it without touching it. Without a role
// file nothing is written and the residual gap is named.
func TestHookSessionStart_ClearCarriesWithRoleFile(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidLead + "\n"
	repo, _, taskFile := guardRepo(t, body)
	set := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	w.put(100, gSidNew, "lead", repo, set.Add(-time.Hour)) // same pid, re-minted id
	oldRole := filepath.Join(repo, ".lets", "sessions", "peers", gSidLead+".role")

	// No peers dir (a default install): no proof is possible, so /clear stays silent.
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("no proof: the boundary must stay:\n%s", got)
	}
	if strings.Contains(out, "no peer role file") || strings.Contains(out, ".task-feature-x") {
		t.Errorf("no peers dir: /clear must add no Notice:\n%s", out)
	}
	// Peers in use but no role file for the old id: the gap is named.
	if err := os.MkdirAll(filepath.Dir(oldRole), 0o700); err != nil {
		t.Fatal(err)
	}
	out = runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("no proof: the boundary must stay:\n%s", got)
	}
	if !strings.Contains(out, "no peer role file") {
		t.Errorf("peers dir present: the gap must be named:\n%s", out)
	}

	healWrite(t, oldRole, "role: worker\ntask: lets-abc\npid: 100\nset: "+set.Format(time.RFC3339)+"\n")
	out = runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); !strings.Contains(got, "session: 1111111 "+gSidNew) {
		t.Fatalf("proven: the boundary must be carried with its sha:\n%s\n%s", got, out)
	}
	if !strings.Contains(out, "carried to this session's new id") {
		t.Errorf("a carry is never silent:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "sessions", "peers", gSidNew+".role")); err != nil {
		t.Errorf("peersHeal must still move the role file to the new id: %v", err)
	}
	if _, err := os.Stat(oldRole); !os.IsNotExist(err) {
		t.Errorf("the old id's role file must be gone after peersHeal: %v", err)
	}
}

// The guard never stalls a start: a held task lock times out at its deadline into
// a held Notice, the file untouched.
func TestHookSessionStart_GuardBounded(t *testing.T) {
	newGuardWorld(t)
	body := "task: lets-abc\n"
	repo, _, taskFile := guardRepo(t, body)
	lockPath := filepath.Join(repo, ".lets", "locks", "task-feature-x.lock")
	healWrite(t, lockPath, "")
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	out := runSessionStart(t, repo, `{"session_id":"`+gSidPane+`","source":"startup"}`)
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("the hook waited %s", d)
	}
	if got := readString(t, taskFile); got != body {
		t.Errorf("file changed under a held lock:\n%s", got)
	}
	if !strings.Contains(out, "lock stayed busy") {
		t.Errorf("no held Notice for the lock timeout:\n%s", out)
	}
}

// In a team worktree the recorded lead decides: a pane never writes the boundary,
// even over a dead holder; the lead does, and its resumed pid is refreshed.
func TestHookSessionStart_TeamLeadOnly(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidNew + "\n" // a dead holder
	repo, head, taskFile := guardRepo(t, body)
	healWrite(t, filepath.Join(repo, ".lets", "teams", "snake.md"), "---\nteam: \"snake\"\nworktree: \""+repo+"\"\n---\n")
	reg := filepath.Join(repo, ".lets", "execution", "members-snake.json")
	healWrite(t, reg, `{"schema":1,"scope":"snake","lead":{"session":"`+gSidLead+`","name":"snake-lead","pid":100,"set":"2026-09-26T10:00:00Z"},"members":[]}`)
	w.put(101, gSidLead, "snake-lead", repo, time.Now().Add(-time.Hour)) // the lead, resumed under pid 101
	w.put(200, gSidPane, "snake-architect", repo, time.Now())

	out := runSessionStart(t, repo, `{"session_id":"`+gSidPane+`","source":"startup"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("a pane wrote the boundary in a team worktree:\n%s", got)
	}
	if !strings.Contains(out, "snake-lead") {
		t.Errorf("the refusal must name the lead:\n%s", out)
	}
	runSessionStart(t, repo, `{"session_id":"`+gSidLead+`","source":"startup"}`)
	if got := readString(t, taskFile); !strings.Contains(got, "session: "+head+" "+gSidLead) {
		t.Errorf("the lead must refresh its boundary:\n%s", got)
	}
	if !strings.Contains(readString(t, reg), `"pid": 101`) {
		t.Errorf("the lead's pid must be refreshed:\n%s", readString(t, reg))
	}
}

// A team file that cannot be read refuses the write and says how to fix it.
func TestHookSessionStart_TeamFileErrorNamesFix(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\n"
	repo, _, taskFile := guardRepo(t, body)
	healWrite(t, filepath.Join(repo, ".lets", "teams", "snake.md"), "---\nteam: \"frog\"\nworktree: \""+repo+"\"\n---\n")
	w.put(200, gSidPane, "x", repo, time.Now())
	out := runSessionStart(t, repo, `{"session_id":"`+gSidPane+`","source":"startup"}`)
	if got := readString(t, taskFile); got != body {
		t.Errorf("file changed:\n%s", got)
	}
	if !strings.Contains(out, "fix or remove the team file") || !strings.Contains(out, "snake.md") {
		t.Errorf("the Notice must name the team file and the fix:\n%s", out)
	}
}
