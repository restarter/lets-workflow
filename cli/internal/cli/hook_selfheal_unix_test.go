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
		t.Errorf("the proof carry must run on clear, not compact; got %v", got)
	}
}

const (
	gSidLead = "aaaaaaaa-0000-4000-8000-000000000001"
	gSidPane = "bbbbbbbb-0000-4000-8000-000000000002"
	gSidNew  = "cccccccc-0000-4000-8000-000000000003"
	gSidNext = "dddddddd-0000-4000-8000-000000000004"
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

// putRow writes a registry row of the observed shape verbatim (kind, jobId,
// parkedJobId, procStart, a missing nameSource ...) and marks pid alive.
func (w *guardWorld) putRow(pid int, row map[string]any) {
	w.t.Helper()
	b, _ := json.Marshal(row)
	if err := os.WriteFile(filepath.Join(w.dir, "sessions", fmt.Sprintf("%d.json", pid)), b, 0o600); err != nil {
		w.t.Fatal(err)
	}
	w.alive[pid] = true
}

// interactiveRow is an interactive session's row as Claude Code writes it.
func interactiveRow(pid int, sid, cwd string, startedMs int64) map[string]any {
	return map[string]any{
		"pid": pid, "sessionId": sid, "cwd": cwd, "startedAt": startedMs, "procStart": "Mon Oct 12 17:00:00 2026",
		"kind": "interactive", "status": "idle", "name": "snake-lead", "nameSource": "user", "peerProtocol": 1,
	}
}

// bgRow is a background job's row: the shared name, no nameSource; jobID "" omits jobId.
func bgRow(pid int, sid, cwd string, startedMs int64, jobID string) map[string]any {
	m := map[string]any{
		"pid": pid, "sessionId": sid, "cwd": cwd, "startedAt": startedMs, "procStart": "Mon Oct 12 17:01:40 2026",
		"kind": "bg", "status": "idle", "name": "snake-lead", "peerProtocol": 1,
	}
	if jobID != "" {
		m["jobId"] = jobID
	}
	return m
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

// A second session in the same worktree runs the same hook: the live holder's
// boundary is not overwritten and the newcomer is told so; once the holder has
// ended, a new session takes the boundary as before.
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

const gStartedMs = int64(1790797661911)

func proofLine(sid string, pid int, ms int64) string {
	return fmt.Sprintf("session_proof: %s %d %d\n", sid, pid, ms)
}

// /clear re-mints the id in the same process: the session_proof recorded beside the
// line proves it (no peers dir at all), the boundary is carried with its sha and the
// new proof binds the new id. Without the proof nothing is written and the gap is
// named - with or without a peers dir.
func TestHookSessionStart_ClearCarriesWithSessionProof(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidLead + "\n"
	repo, _, taskFile := guardRepo(t, body)
	w.putRow(100, interactiveRow(100, gSidNew, repo, gStartedMs)) // same pid, re-minted id

	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("no proof: the boundary must stay:\n%s", got)
	}
	if !strings.Contains(out, "no session_proof recorded its process") {
		t.Errorf("no proof: the gap must be named even without a peers dir:\n%s", out)
	}

	healWrite(t, taskFile, body+proofLine(gSidLead, 100, gStartedMs))
	out = runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	want := "task: lets-abc\nsession: 1111111 " + gSidNew + "\n" + proofLine(gSidNew, 100, gStartedMs)
	if got := readString(t, taskFile); got != want {
		t.Fatalf("proven: the boundary must be carried with its sha and a new proof:\n%s\n%s", got, out)
	}
	if !strings.Contains(out, "carried to this session's new id") {
		t.Errorf("a carry is never silent:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "sessions", "peers")); !os.IsNotExist(err) {
		t.Errorf("the carry must not need or create a peers dir: %v", err)
	}
}

// An older binary rewrote session: and left the proof of the previous id beside it:
// that proof names another session and proves nothing about the stored one.
func TestHookSessionStart_ProofOfOldIdProvesNothing(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidPane + "\n" + proofLine(gSidLead, 100, gStartedMs)
	repo, _, taskFile := guardRepo(t, body)
	w.putRow(100, interactiveRow(100, gSidNew, repo, gStartedMs))
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("a proof of another id carried:\n%s", got)
	}
	if !strings.Contains(out, "no session_proof recorded its process") {
		t.Errorf("want the no-proof Notice:\n%s", out)
	}
}

// Two /clear in a row in the same process: each carry writes the proof the next
// one needs.
func TestHookSessionStart_ClearTwiceCarries(t *testing.T) {
	w := newGuardWorld(t)
	repo, _, taskFile := guardRepo(t, "task: lets-abc\nsession: 1111111 "+gSidLead+"\n"+proofLine(gSidLead, 100, gStartedMs))
	w.putRow(100, interactiveRow(100, gSidNew, repo, gStartedMs))
	runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	w.putRow(100, interactiveRow(100, gSidNext, repo, gStartedMs))
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNext+`","source":"clear"}`)
	want := "task: lets-abc\nsession: 1111111 " + gSidNext + "\n" + proofLine(gSidNext, 100, gStartedMs)
	if got := readString(t, taskFile); got != want {
		t.Errorf("second carry:\n%s\n%s", got, out)
	}
}

// `claude -r` resumes the same id in a new process: the resume re-records only the
// proof, so a /clear in that new process still carries.
func TestHookSessionStart_ResumeThenClearCarries(t *testing.T) {
	w := newGuardWorld(t)
	repo, _, taskFile := guardRepo(t, "task: lets-abc\nsession: 1111111 "+gSidLead+"\n"+proofLine(gSidLead, 100, gStartedMs))
	const p2Ms = gStartedMs + 3_600_000
	w.putRow(300, interactiveRow(300, gSidLead, repo, p2Ms)) // P1 (pid 100) is gone
	runSessionStart(t, repo, `{"session_id":"`+gSidLead+`","source":"resume"}`)
	if got, want := readString(t, taskFile), "task: lets-abc\nsession: 1111111 "+gSidLead+"\n"+proofLine(gSidLead, 300, p2Ms); got != want {
		t.Fatalf("resume must rewrite only the proof:\n%s", got)
	}
	w.putRow(300, interactiveRow(300, gSidNew, repo, p2Ms))
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); !strings.Contains(got, "session: 1111111 "+gSidNew+"\n") {
		t.Errorf("resume then /clear must carry:\n%s\n%s", got, out)
	}
}

// The proof names an exact process: the recorded pid now listing a startedAt 800 ms
// off is another process (Rotated's one-second slack alone would accept it).
func TestHookSessionStart_ProofStartedMismatchNoProof(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidLead + "\n" + proofLine(gSidLead, 100, gStartedMs)
	repo, _, taskFile := guardRepo(t, body)
	w.putRow(100, interactiveRow(100, gSidNew, repo, gStartedMs+800))
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if got := readString(t, taskFile); got != body {
		t.Fatalf("a pid with another startedAt carried:\n%s", got)
	}
	if !strings.Contains(out, "no session_proof recorded its process") {
		t.Errorf("want the no-proof Notice:\n%s", out)
	}
}

// A session parked into a background job (lets-56ymc): the job that continues it
// takes the boundary on startup, though the parked row is still listed.
func TestHookSessionStart_ParkedContinuationRefreshes(t *testing.T) {
	w := newGuardWorld(t)
	repo, head, taskFile := guardRepo(t, "task: lets-abc\nsession: 1111111 "+gSidLead+"\n")
	parked := interactiveRow(100, gSidLead, repo, gStartedMs)
	parked["parkedJobId"] = "deadbeef"
	w.putRow(100, parked)
	w.putRow(200, bgRow(200, gSidNew, repo, gStartedMs+60_000, "deadbeef"))
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"startup"}`)
	want := "task: lets-abc\nsession: " + head + " " + gSidNew + "\n" + proofLine(gSidNew, 200, gStartedMs+60_000)
	if got := readString(t, taskFile); got != want {
		t.Errorf("the continuing job must take the boundary:\n%s\n%s", got, out)
	}
}

// The same parked session is not continued in a caller whose job id differs or is
// missing: the live parked holder keeps the line.
func TestHookSessionStart_UnrelatedParkedHeld(t *testing.T) {
	for _, jobID := range []string{"feedface", ""} {
		w := newGuardWorld(t)
		body := "task: lets-abc\nsession: 1111111 " + gSidLead + "\n"
		repo, _, taskFile := guardRepo(t, body)
		parked := interactiveRow(100, gSidLead, repo, gStartedMs)
		parked["parkedJobId"] = "deadbeef"
		w.putRow(100, parked)
		w.putRow(200, bgRow(200, gSidNew, repo, gStartedMs+60_000, jobID))
		out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"startup"}`)
		if got := readString(t, taskFile); got != body {
			t.Errorf("jobId %q: an unrelated job took the boundary:\n%s", jobID, got)
		}
		if !strings.Contains(out, "held by live session aaaaaaaa") {
			t.Errorf("jobId %q: want the HeldLive Notice:\n%s", jobID, out)
		}
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

// A team worktree has no lead check any more: a recorded lead (a THIRD live
// session in the old members registry) decides nothing. A dead holder's line is
// refreshed and no Notice names the lead; a live foreign holder is HeldLive.
func TestHookSessionStart_TeamWorktreeLiveHolderHeld(t *testing.T) {
	w := newGuardWorld(t)
	body := "task: lets-abc\nsession: 1111111 " + gSidNew + "\n" // a dead holder
	repo, head, taskFile := guardRepo(t, body)
	healWrite(t, filepath.Join(repo, ".lets", "teams", "snake.md"), "---\nteam: \"snake\"\nworktree: \""+repo+"\"\n---\n")
	healWrite(t, filepath.Join(repo, ".lets", "execution", "members-snake.json"), `{"schema":1,"scope":"snake","lead":{"session":"`+gSidLead+`","name":"snake-lead","pid":101,"set":"2026-09-26T10:00:00Z"},"members":[]}`)
	w.put(101, gSidLead, "snake-lead", repo, time.Now().Add(-time.Hour)) // the recorded lead, live
	w.put(200, gSidPane, "snake-architect", repo, time.Now())

	out := runSessionStart(t, repo, `{"session_id":"`+gSidPane+`","source":"startup"}`)
	if got := readString(t, taskFile); !strings.Contains(got, "session: "+head+" "+gSidPane+"\n") {
		t.Fatalf("a dead holder's line must be refreshed:\n%s\n%s", got, out)
	}
	if strings.Contains(out, "snake-lead") || strings.Contains(out, "team worktree") {
		t.Errorf("no Notice may name the lead:\n%s", out)
	}
	out = runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"startup"}`)
	if got := readString(t, taskFile); !strings.Contains(got, "session: "+head+" "+gSidPane+"\n") {
		t.Errorf("a live foreign holder was overwritten:\n%s", got)
	}
	if !strings.Contains(out, "held by live session bbbbbbbb") {
		t.Errorf("want the HeldLive Notice:\n%s", out)
	}
}

// healSettingsDirs returns permissions.additionalDirectories of wt's settings.local.json.
func healSettingsDirs(t *testing.T, wt string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json"))
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

const healStartup = `{"session_id":"0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d","source":"startup"}`

func healLink(t *testing.T, repo, wt string) {
	t.Helper()
	if err := os.Symlink(filepath.Join(repo, ".lets"), filepath.Join(wt, ".lets")); err != nil {
		t.Fatal(err)
	}
}

func TestSelfHeal_BackfillsLinkedWorktree(t *testing.T) {
	repo, wt := healRepo(t, true)
	healLink(t, repo, wt)
	out := runSessionStart(t, wt, healStartup)
	if got := healSettingsDirs(t, wt); len(got) != 1 || got[0] != filepath.Join(repo, ".lets") {
		t.Errorf("additionalDirectories = %v\n%s", got, out)
	}
	if strings.Contains(out, "settings_local_") {
		t.Errorf("a successful backfill must emit no Notice:\n%s", out)
	}
}

func TestSelfHeal_BackfillNoticeOnMalformedSettings(t *testing.T) {
	repo, wt := healRepo(t, true)
	healLink(t, repo, wt)
	healWrite(t, filepath.Join(wt, ".claude", "settings.local.json"), "{not json")
	out := runSessionStart(t, wt, healStartup)
	if !strings.Contains(out, "settings_local_unreadable") {
		t.Errorf("no Notice for an unreadable settings.local.json:\n%s", out)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json")); string(data) != "{not json" {
		t.Errorf("malformed settings.local.json was rewritten: %s", data)
	}
}

// S1 (architect) / Codex #5: an unlinked worktree gets the Notice on its FIRST start.
func TestSelfHeal_AdoptPathNoticeOnFirstStart(t *testing.T) {
	_, wt := healRepo(t, true)
	healWrite(t, filepath.Join(wt, ".claude", "settings.local.json"), "{not json")
	out := runSessionStart(t, wt, healStartup)
	if fi, err := os.Lstat(filepath.Join(wt, ".lets")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".lets is not linked after the first start (err=%v)\n%s", err, out)
	}
	if !strings.Contains(out, "settings_local_unreadable") {
		t.Errorf("the first start must surface the settings warning:\n%s", out)
	}
}

func TestSelfHeal_TrackedSettingsNoNotice(t *testing.T) {
	repo, wt := healRepo(t, true)
	healLink(t, repo, wt)
	healWrite(t, filepath.Join(wt, ".claude", "settings.local.json"), "{}\n")
	healGit(t, wt, "add", "-f", ".claude/settings.local.json")
	healGit(t, wt, "commit", "-q", "-m", "track it")
	out := runSessionStart(t, wt, healStartup)
	if strings.Contains(out, "settings_local_") {
		t.Errorf("a tracked settings file must not raise a Notice:\n%s", out)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json")); string(data) != "{}\n" {
		t.Errorf("tracked settings.local.json was rewritten: %s", data)
	}
}

func TestSelfHeal_GitignoreNegationNoNotice(t *testing.T) {
	repo, wt := healRepo(t, true)
	healLink(t, repo, wt)
	healWrite(t, filepath.Join(wt, ".gitignore"), "!.claude/settings.local.json\n")
	out := runSessionStart(t, wt, healStartup)
	if strings.Contains(out, "settings_local_") {
		t.Errorf("a .gitignore negation must not raise a Notice:\n%s", out)
	}
	if got := healSettingsDirs(t, wt); len(got) != 1 || got[0] != filepath.Join(repo, ".lets") {
		t.Errorf("additionalDirectories = %v\n%s", got, out)
	}
}

func TestSelfHeal_ExcludeWriteFailedIsANotice(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	repo, wt := healRepo(t, true)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	healLink(t, repo, wt)
	info := filepath.Join(repo, ".git", "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(info, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(info, 0o755) })
	out := runSessionStart(t, wt, healStartup)
	if !strings.Contains(out, "settings_local_exclude_failed") {
		t.Errorf("a failed info/exclude write must be a Notice:\n%s", out)
	}
}
