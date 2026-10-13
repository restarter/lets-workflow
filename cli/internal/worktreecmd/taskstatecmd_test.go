//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

const testSID = "2f942be4-23e0-4b17-9eab-df0a4a7298f2"

func tsRepo(t *testing.T, branch string) (repo string) {
	t.Helper()
	repo = initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	if branch != "main" {
		runIn(t, repo, "git", "checkout", "-q", "-b", branch)
	}
	return repo
}

func TestTaskStateSet_ClearTaskKeepsSessionAndOrc(t *testing.T) {
	repo := tsRepo(t, "feature/lets-abc-x")
	mustWrite(t, taskFile(repo, "feature/lets-abc-x"), "task: lets-abc\nstart: abc1234\nsession: abc1234 "+testSID+"\norigin: branch\norc: MAIN-LETS\n", 0o644)
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{ClearTask: true})
	if err != nil || !res.TaskState.Written {
		t.Fatalf("err=%v res=%+v", err, res.TaskState)
	}
	got := readTask(t, repo, "feature/lets-abc-x")
	if strings.Contains(got, "task:") || strings.Contains(got, "start:") || strings.Contains(got, "origin:") ||
		!strings.Contains(got, "session: abc1234 "+testSID) || !strings.Contains(got, "orc: MAIN-LETS") {
		t.Errorf("after --clear-task:\n%s", got)
	}
}

func TestTaskStateSet_OrcRefusedOnMergeBranch(t *testing.T) {
	repo := tsRepo(t, "main")
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orc: "MAIN-LETS", Create: true})
	if err != nil || res.TaskState.Written || res.TaskState.Reason != "orc_on_merge_branch" {
		t.Fatalf("err=%v res=%+v", err, res.TaskState)
	}
	if _, err := os.Stat(taskFile(repo, "main")); !os.IsNotExist(err) {
		t.Error("nothing may be written on the merge-branch")
	}
}

func TestTaskStateSet_TaskMismatchNeedsStart(t *testing.T) {
	repo := tsRepo(t, "feature/x")
	mustWrite(t, taskFile(repo, "feature/x"), "task: lets-aaa\nstart: abc1234\norc: MAIN\n", 0o644)
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Task: "lets-bbb"})
	if err != nil || res.TaskState.Written || res.TaskState.Reason != "task_mismatch" {
		t.Fatalf("without --start: err=%v res=%+v", err, res.TaskState)
	}
	if got := readTask(t, repo, "feature/x"); !strings.Contains(got, "task: lets-aaa") {
		t.Errorf("file changed: %s", got)
	}
	res, err = worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Task: "lets-bbb", Start: "def5678"})
	if err != nil || !res.TaskState.Written {
		t.Fatalf("with --start: err=%v res=%+v", err, res.TaskState)
	}
	if got := readTask(t, repo, "feature/x"); !strings.Contains(got, "task: lets-bbb") || !strings.Contains(got, "start: def5678") || !strings.Contains(got, "orc: MAIN") {
		t.Errorf("replaced file:\n%s", got)
	}
}

func TestTaskStateSet_FileAbsentReboundAndInvalid(t *testing.T) {
	repo := tsRepo(t, "feature/y")
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orc: "MAIN"})
	if err != nil || res.TaskState.Written || res.TaskState.Reason != "file_absent" {
		t.Fatalf("absent: err=%v res=%+v", err, res.TaskState)
	}
	if _, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orc: "MAIN", Create: true}); err != nil {
		t.Fatal(err)
	}
	res, err = worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orc: "MAIN-LIC"})
	if err != nil || res.TaskState.Rebound == nil || res.TaskState.Rebound.From != "MAIN" {
		t.Errorf("rebound: err=%v res=%+v", err, res.TaskState)
	}
	if _, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orc: "x --auto"}); worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("invalid orc: %v, want exit 2", err)
	}
}

func TestTaskStateSet_LockBusyExit26(t *testing.T) {
	repo := tsRepo(t, "feature/z")
	mustWrite(t, taskFile(repo, "feature/z"), "task: lets-z\n", 0o644)
	mustMkdir(t, filepath.Join(repo, ".lets", "locks"))
	f, err := os.OpenFile(filepath.Join(repo, ".lets", "locks", "task-feature-z.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	_, err = worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orc: "MAIN", Wait: 100 * time.Millisecond})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitTaskStateLockBusy {
		t.Errorf("err = %v, want exit 26", err)
	}
}

func TestBranchName_ConventionAndSlug(t *testing.T) {
	repo, _ := adoptRepo(t, "bn-1")
	title := filepath.Join(realTempDir(t), "title.txt")
	mustWrite(t, title, "Fix: Login $(rm -rf /) `now`", 0o644)
	res, err := worktreecmd.BranchName(context.Background(), repo, worktreecmd.BranchNameOptions{Task: "lets-abc", TitleFile: title})
	if err != nil || res.Branch != "feature/lets-abc-fix-login-rm-rf-now" || res.Source != "installed" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	res, err = worktreecmd.BranchName(context.Background(), repo, worktreecmd.BranchNameOptions{Task: "lets-abc", TitleFile: title, Worktree: true})
	if err != nil || !strings.HasPrefix(res.Branch, "worktree-lets-abc-") {
		t.Errorf("worktree: err=%v res=%+v", err, res)
	}
	mustWrite(t, title, "Задача без латиниці", 0o644)
	if res, _ := worktreecmd.BranchName(context.Background(), repo, worktreecmd.BranchNameOptions{Task: "lets-abc", TitleFile: title}); res.Branch != "feature/lets-abc-task" {
		t.Errorf("non-Latin title: %q", res.Branch)
	}
	if _, err := worktreecmd.BranchName(context.Background(), repo, worktreecmd.BranchNameOptions{Task: "-rf", TitleFile: title}); worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("invalid task: %v", err)
	}
	// board override renders the team's shape
	mustWrite(t, filepath.Join(repo, ".claude", "rules", "tracker-beads.board.md"), "## Worktree\n\nid: `PWA-[0-9]+`.\nbranch: `feature/{id}-{slug}`.\n", 0o644)
	mustWrite(t, title, "Fix login", 0o644)
	if res, err := worktreecmd.BranchName(context.Background(), repo, worktreecmd.BranchNameOptions{Task: "PWA-45122", TitleFile: title}); err != nil || res.Branch != "feature/PWA-45122-fix-login" || res.Source != "board" {
		t.Errorf("board: err=%v res=%+v", err, res)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Fix login":                "fix-login",
		"  --Hello__World!! ":      "hello-world",
		"":                         "task",
		"Задача":                   "task",
		strings.Repeat("abc ", 30): strings.TrimRight(strings.Repeat("abc-", 13)[:50], "-"),
	} {
		if got := worktreecmd.Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTaskCandidateFor(t *testing.T) {
	repo, wt := adoptRepo(t, "feature/lets-abc-fix-login")
	res, err := worktreecmd.TaskCandidateFor(context.Background(), wt, "", "")
	if err != nil || res.TaskCandidate.ID != "lets-abc" || res.TaskCandidate.Source != "created" {
		t.Fatalf("created shape: err=%v cand=%+v", err, res.TaskCandidate)
	}
	// accept shapes are adopt-only
	_, wt2 := adoptRepo(t, "lets-ip06f-peer-messaging-orca")
	if res, _ := worktreecmd.TaskCandidateFor(context.Background(), wt2, "", ""); res.TaskCandidate.ID != "" || res.TaskCandidate.Reason != "no_match" {
		t.Errorf("accept shape: %+v", res.TaskCandidate)
	}
	// an untrusted ref through a file
	ref := filepath.Join(realTempDir(t), "ref.txt")
	mustWrite(t, ref, "worktree-lets-xyz-thing\n", 0o644)
	if res, _ := worktreecmd.TaskCandidateFor(context.Background(), repo, ref, ""); res.TaskCandidate.ID != "lets-xyz" {
		t.Errorf("ref-file: %+v", res.TaskCandidate)
	}
	mustWrite(t, ref, "bad ref $(x)\n", 0o644)
	if res, _ := worktreecmd.TaskCandidateFor(context.Background(), repo, ref, ""); res.TaskCandidate.Reason != "ref_invalid" {
		t.Errorf("bad ref: %+v", res.TaskCandidate)
	}
	// undeclared convention
	installAdapter(t, repo, "beads", "links: `.beads/.env` (0600).")
	if res, _ := worktreecmd.TaskCandidateFor(context.Background(), wt, "", ""); res.TaskCandidate.Reason != "convention_undeclared" {
		t.Errorf("undeclared: %+v", res.TaskCandidate)
	}
	// a board template without id: names new branches but reads no id back (lets-puvic):
	// detect-task's legacy fallback keys on convention_undeclared.
	boardFile := filepath.Join(repo, ".claude", "rules", "tracker-beads.board.md")
	mustWrite(t, boardFile, "## Worktree\n\nbranch: `feature/{id}-{slug}`.\n", 0o644)
	if res, _ := worktreecmd.TaskCandidateFor(context.Background(), wt, "", ""); res.TaskCandidate.ID != "" || res.TaskCandidate.Reason != "convention_undeclared" {
		t.Errorf("undeclared with a board template: %+v", res.TaskCandidate)
	}
	if err := os.Remove(boardFile); err != nil {
		t.Fatal(err)
	}
	// an installed copy that predates ## Worktree falls back to the plugin's adapter
	plugin := fakePluginRoot(t, beadsConvention)
	if res, _ := worktreecmd.TaskCandidateFor(context.Background(), wt, "", plugin); res.TaskCandidate.ID != "lets-abc" {
		t.Errorf("plugin fallback: %+v", res.TaskCandidate)
	}
	title := filepath.Join(realTempDir(t), "title.txt")
	mustWrite(t, title, "Fix login", 0o644)
	installAdapter(t, repo, "beads", "links: `.beads/.env` (0600).\nid: `[a-z]+-[a-z0-9]+`.\nbranch: `bug/{id}-{slug}`.")
	if res, err := worktreecmd.BranchName(context.Background(), repo, worktreecmd.BranchNameOptions{Task: "lets-abc", TitleFile: title, PluginRoot: plugin}); err != nil || res.Branch != "bug/lets-abc-fix-login" {
		t.Errorf("branch-name with a declared installed copy must ignore the plugin: err=%v res=%+v", err, res)
	}
}

// fakePluginRoot is a minimal LETS plugin install holding one beads adapter.
func fakePluginRoot(t *testing.T, worktree string) string {
	t.Helper()
	root := realTempDir(t)
	mustWrite(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"lets"}`, 0o644)
	mustWrite(t, filepath.Join(root, "rules", "tracker-beads.md"), "# Tracker adapter: beads\n\n## Worktree\n\n"+worktree+"\n", 0o644)
	return root
}

func strp(s string) *string { return &s }

// fakeSelf registers one live session (name "" = unnamed) in a fake Claude Code
// registry and returns its id.
func fakeSelf(t *testing.T, name string) string {
	t.Helper()
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "sessions"))
	sid := "aaaaaaaa-0000-4000-8000-0000000000aa"
	mustWrite(t, filepath.Join(home, "sessions", "4343.json"), `{"name":"`+name+`","sessionId":"`+sid+`","cwd":"/x","peerProtocol":1}`, 0o644)
	oldHome, oldAlive, oldParent := ccregistry.HomeDir, ccregistry.ProcAlive, ccregistry.ParentOf
	ccregistry.HomeDir = func() string { return home }
	ccregistry.ProcAlive = func(pid int) bool { return pid == 4343 }
	// this test process runs under that session: its parent's parent is pid 4343
	ccregistry.ParentOf = fakeParents(map[int]int{os.Getppid(): 4343, 4343: 1})
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive, ccregistry.ParentOf = oldHome, oldAlive, oldParent })
	return sid
}

// fakeParents is a ccregistry.ParentOf over a fixed process tree; a pid outside it
// cannot be read.
func fakeParents(parents map[int]int) func(int) (int, error) {
	return func(pid int) (int, error) {
		if p, ok := parents[pid]; ok {
			return p, nil
		}
		return 0, errors.New("no such process")
	}
}

func tsSet(t *testing.T, repo string, o worktreecmd.TaskStateOptions) {
	t.Helper()
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, o)
	if err != nil || !res.TaskState.Written {
		t.Fatalf("%+v: err=%v res=%+v", o, err, res.TaskState)
	}
}

// Records land on the merge-branch file (created when missing), merge only the
// fields given, keep every other line, and clear by name.
func TestTaskStateSet_OrchestratorRecords(t *testing.T) {
	repo := tsRepo(t, "main")
	sid := fakeSelf(t, "MAIN-LETS")
	tsSet(t, repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, OrchestratorScope: strp("docs"), Standing: strp("on")})
	tsSet(t, repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-ETH"})
	tsSet(t, repo, worktreecmd.TaskStateOptions{Task: "lets-abc"})
	want := "orchestrator: MAIN-LETS scope=docs standing=on\norchestrator: MAIN-ETH\ntask: lets-abc\n"
	if got := readTask(t, repo, "main"); got != want {
		t.Fatalf("file:\n%s\nwant:\n%s", got, want)
	}
	tsSet(t, repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-LETS"})
	tsSet(t, repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-LETS", Standing: strp("off")})
	tsSet(t, repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-ETH", OrchestratorScope: strp("cli")})
	tsSet(t, repo, worktreecmd.TaskStateOptions{ClearOrchestrator: "MAIN-ETH"})
	want = "orchestrator: MAIN-LETS scope=docs standing=off\ntask: lets-abc\n"
	if got := readTask(t, repo, "main"); got != want {
		t.Errorf("file:\n%s\nwant:\n%s", got, want)
	}
}

// Orchestrator records always go to the merge-branch file, whatever HEAD is: from
// a feature-branch worktree too (F1). That branch's own keys cannot ride along.
func TestTaskStateSet_OrchestratorFromWorktree(t *testing.T) {
	repo := tsRepo(t, "main")
	wt := filepath.Join(filepath.Dir(repo), "wt-feature")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "feature/lets-abc-x", wt)
	if err := os.Symlink(filepath.Join(repo, ".lets"), filepath.Join(wt, ".lets")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, taskFile(repo, "feature/lets-abc-x"), "task: lets-abc\n", 0o644)
	sid := fakeSelf(t, "MAIN-LETS")
	res, err := worktreecmd.TaskStateSet(context.Background(), wt, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("on")})
	if err != nil || !res.TaskState.Written || res.TaskState.Path != taskFile(repo, "main") {
		t.Fatalf("from a worktree: err=%v res=%+v", err, res.TaskState)
	}
	if got := readTask(t, repo, "main"); got != "orchestrator: MAIN-LETS standing=on\n" {
		t.Errorf("merge-branch file: %q", got)
	}
	tsSet(t, wt, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("off")})
	tsSet(t, wt, worktreecmd.TaskStateOptions{ClearOrchestrator: "OTHER"})
	if got := readTask(t, repo, "main"); got != "orchestrator: MAIN-LETS standing=off\n" {
		t.Errorf("standing toggled from the worktree: %q", got)
	}
	if got := readTask(t, repo, "feature/lets-abc-x"); got != "task: lets-abc\n" {
		t.Errorf("the worktree's own file must not change: %q", got)
	}
	_, err = worktreecmd.TaskStateSet(context.Background(), wt, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, ClearOrc: true})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("this branch's keys with orchestrator records off the merge-branch: %v, want usage", err)
	}
}

// A detached HEAD has no task-state of its own, but orchestrator records alone still
// go to the merge-branch file; a call that also writes HEAD's keys is refused.
func TestTaskStateSet_OrchestratorFromDetachedHead(t *testing.T) {
	repo := tsRepo(t, "main")
	runIn(t, repo, "git", "checkout", "-q", "--detach")
	sid := fakeSelf(t, "MAIN-LETS")
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("off")})
	if err != nil || !res.TaskState.Written || res.TaskState.Path != taskFile(repo, "main") {
		t.Fatalf("detached HEAD, records only: err=%v res=%+v", err, res.TaskState)
	}
	if got := readTask(t, repo, "main"); got != "orchestrator: MAIN-LETS standing=off\n" {
		t.Errorf("merge-branch file: %q", got)
	}
	_, err = worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, SessionSHA: "abc1234", SessionID: testSID})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("detached HEAD with this branch's keys: %v, want detached_head (usage)", err)
	}
}

// /lets:start --main writes its session boundary, clears orc: and records itself in
// ONE call on the merge-branch (F2).
func TestTaskStateSet_MainModeOneCall(t *testing.T) {
	repo := tsRepo(t, "main")
	mustWrite(t, taskFile(repo, "main"), "orc: STALE\nfuture: keep\n", 0o644)
	sid := fakeSelf(t, "MAIN-LETS")
	tsSet(t, repo, worktreecmd.TaskStateOptions{ClearOrc: true, SessionSHA: "abc1234", SessionID: testSID, Create: true, SelfOrchestrator: true, Session: sid, OrchestratorScope: strp("docs")})
	if got := readTask(t, repo, "main"); got != "future: keep\nsession: abc1234 "+testSID+"\norchestrator: MAIN-LETS scope=docs\n" {
		t.Errorf("one call: %q", got)
	}
}

// --self-orchestrator records this session's registry name; an unnamed or unknown
// session is refused with the /rename remedy.
func TestTaskStateSet_SelfOrchestrator(t *testing.T) {
	repo := tsRepo(t, "main")
	sid := fakeSelf(t, "MAIN-LETS")
	tsSet(t, repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, OrchestratorScope: strp("docs")})
	tsSet(t, repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("on")})
	if got := readTask(t, repo, "main"); got != "orchestrator: MAIN-LETS scope=docs standing=on\n" {
		t.Errorf("file: %s", got)
	}
	bad := fakeSelf(t, "x --auto")
	if res, _ := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: bad}); res.Error == nil || !strings.Contains(res.Error.Message, "fails the name grammar") {
		t.Errorf("a registry name failing the grammar is named: %+v", res.Error)
	}
	unnamed := fakeSelf(t, "")
	for _, s := range []string{unnamed, "bbbbbbbb-0000-4000-8000-0000000000bb"} {
		res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: s})
		if err == nil || res.Error == nil || res.Error.Kind != "orchestrator_needs_name" || !strings.Contains(res.Error.Remediation, "/rename") {
			t.Errorf("session %s: err=%v res=%+v", s, err, res.Error)
		}
	}
	if _, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true}); worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("no session id: %v, want exit 2", err)
	}
}

// A grant is written only by the orchestrator itself: --standing on for a named
// orchestrator is refused with the rule; --standing off for one stays allowed.
func TestTaskStateSet_StandingOnOnlySelf(t *testing.T) {
	repo := tsRepo(t, "main")
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-LETS", Standing: strp("on")})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage || res.Error == nil || !strings.Contains(res.Error.Message, "--self-orchestrator") {
		t.Errorf("err=%v res=%+v", err, res.Error)
	}
	tsSet(t, repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-LETS", Standing: strp("off")})
	if got := readTask(t, repo, "main"); got != "orchestrator: MAIN-LETS standing=off\n" {
		t.Errorf("revoking stays open to anyone: %q", got)
	}
}

// --self-orchestrator writes only when the claimed session's own process runs the
// command: a session id from the environment of another process is refused
// orchestrator_not_own_process (exit 2), and so is an ancestry that cannot be read.
func TestTaskStateSet_SelfOrchestratorOwnProcess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		parents map[int]int
	}{
		{"not an ancestor", map[int]int{os.Getppid(): 777, 777: 1}},
		{"ancestry unreadable", map[int]int{os.Getppid(): 777}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := tsRepo(t, "main")
			sid := fakeSelf(t, "MAIN-LETS")
			ccregistry.ParentOf = fakeParents(tc.parents)
			res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("on")})
			if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage || res.Error == nil || res.Error.Kind != "orchestrator_not_own_process" || res.Error.Remediation == "" {
				t.Errorf("err=%v res=%+v", err, res.Error)
			}
			if _, err := os.Stat(taskFile(repo, "main")); !os.IsNotExist(err) {
				t.Error("a refused call must write nothing")
			}
		})
	}
}

// A name two live sessions carry is refused orchestrator_name_not_unique (exit 2),
// even from the session's own process: the record is keyed by name, so writing it
// would set the other holder's grant too.
func TestTaskStateSet_SelfOrchestratorNameNotUnique(t *testing.T) {
	repo := tsRepo(t, "main")
	sid := fakeSelf(t, "MAIN-LETS")
	home := ccregistry.HomeDir()
	mustWrite(t, filepath.Join(home, "sessions", "5454.json"), `{"name":"MAIN-LETS","sessionId":"bbbbbbbb-0000-4000-8000-0000000000bb","cwd":"/y","peerProtocol":1}`, 0o644)
	ccregistry.ProcAlive = func(pid int) bool { return pid == 4343 || pid == 5454 }
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("on")})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage || res.Error == nil || res.Error.Kind != "orchestrator_name_not_unique" || !strings.Contains(res.Error.Remediation, "/rename") {
		t.Errorf("err=%v res=%+v", err, res.Error)
	}
	if _, err := os.Stat(taskFile(repo, "main")); !os.IsNotExist(err) {
		t.Error("a refused call must write nothing")
	}
	// the ancestry check comes first: a process that is not the claimed session learns
	// nothing about the name
	ccregistry.ParentOf = fakeParents(map[int]int{os.Getppid(): 777, 777: 1})
	if res, _ := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid}); res.Error == nil || res.Error.Kind != "orchestrator_not_own_process" {
		t.Errorf("not own process first: %+v", res.Error)
	}
}

// addSession registers one more live session in fakeSelf's registry.
func addSession(t *testing.T, pid int, name, sid string) {
	t.Helper()
	mustWrite(t, filepath.Join(ccregistry.HomeDir(), "sessions", strconv.Itoa(pid)+".json"), `{"name":"`+name+`","sessionId":"`+sid+`","cwd":"/y","peerProtocol":1}`, 0o644)
	prev := ccregistry.ProcAlive
	ccregistry.ProcAlive = func(p int) bool { return p == pid || prev(p) }
}

// The caller's session is the NEAREST Claude Code process above the command: a session
// B launched under orchestrator A (A -> make -> B -> shell -> lets) cannot write A's
// record by exporting A's id; it can write its own. Plain wrappers beneath A stay
// transparent.
func TestTaskStateSet_SelfOrchestratorNestedSession(t *testing.T) {
	repo := tsRepo(t, "main")
	sidA := fakeSelf(t, "MAIN-LETS") // pid 4343
	sidB := "cccccccc-0000-4000-8000-0000000000cc"
	addSession(t, 6565, "WORKER-B", sidB)
	ccregistry.ParentOf = fakeParents(map[int]int{os.Getppid(): 6565, 6565: 600, 600: 4343, 4343: 1})
	res, err := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sidA, Standing: strp("on")})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage || res.Error == nil || res.Error.Kind != "orchestrator_not_own_process" || !strings.Contains(res.Error.Message, sidB) {
		t.Errorf("B claiming A: err=%v res=%+v", err, res.Error)
	}
	if _, err := os.Stat(taskFile(repo, "main")); !os.IsNotExist(err) {
		t.Fatal("a refused call must write nothing")
	}
	tsSet(t, repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sidB, Standing: strp("on")})
	if got := readTask(t, repo, "main"); got != "orchestrator: WORKER-B standing=on\n" {
		t.Errorf("B's own record: %q", got)
	}
	// shell -> make -> env beneath A: transparent
	ccregistry.ParentOf = fakeParents(map[int]int{os.Getppid(): 600, 600: 601, 601: 4343, 4343: 1})
	tsSet(t, repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sidA, Standing: strp("on")})
	if got := readTask(t, repo, "main"); !strings.Contains(got, "orchestrator: MAIN-LETS standing=on\n") {
		t.Errorf("A through plain wrappers: %q", got)
	}
}

// A pure --standing off only tightens, so it passes while the name is shared (still
// from the session's own process); anything else on a shared name is refused.
func TestTaskStateSet_StandingOffWithSharedName(t *testing.T) {
	repo := tsRepo(t, "main")
	sid := fakeSelf(t, "MAIN-LETS")
	mustWrite(t, taskFile(repo, "main"), "orchestrator: MAIN-LETS standing=on\n", 0o644)
	addSession(t, 5454, "MAIN-LETS", "bbbbbbbb-0000-4000-8000-0000000000bb")
	tsSet(t, repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("off")})
	if got := readTask(t, repo, "main"); got != "orchestrator: MAIN-LETS standing=off\n" {
		t.Errorf("revoke on a shared name: %q", got)
	}
	for _, o := range []worktreecmd.TaskStateOptions{
		{SelfOrchestrator: true, Session: sid, Standing: strp("off"), OrchestratorScope: strp("docs")},
		{SelfOrchestrator: true, Session: sid, OrchestratorScope: strp("docs")},
		{SelfOrchestrator: true, Session: sid, Standing: strp("on")},
	} {
		if res, _ := worktreecmd.TaskStateSet(context.Background(), repo, o); res.Error == nil || res.Error.Kind != "orchestrator_name_not_unique" {
			t.Errorf("%+v on a shared name: %+v", o, res.Error)
		}
	}
	ccregistry.ParentOf = fakeParents(map[int]int{os.Getppid(): 777, 777: 1})
	if res, _ := worktreecmd.TaskStateSet(context.Background(), repo, worktreecmd.TaskStateOptions{SelfOrchestrator: true, Session: sid, Standing: strp("off")}); res.Error == nil || res.Error.Kind != "orchestrator_not_own_process" {
		t.Errorf("a revoke still needs the own process: %+v", res.Error)
	}
}

// Flag combinations that cannot mean one write, and invalid values, are usage errors
// and write nothing.
func TestTaskStateSet_OrchestratorUsage(t *testing.T) {
	repo := tsRepo(t, "main")
	for _, o := range []worktreecmd.TaskStateOptions{
		{Standing: strp("on")},
		{OrchestratorScope: strp("docs")},
		{Orchestrator: "A", SelfOrchestrator: true},
		{Orchestrator: "A", ClearOrchestrator: "B"},
		{ClearOrchestrator: "B", Standing: strp("off")},
		{Orchestrator: "A", Orc: "B"},
		{Orchestrator: "A", Task: "lets-abc"},
		{SelfOrchestrator: true, Start: "abc1234"},
		{Orchestrator: "A", Standing: strp("maybe")},
		{Orchestrator: "A", Standing: strp("on")}, // only the orchestrator itself grants
		{Standing: strp("on"), OrchestratorScope: strp("docs")},
		{Orchestrator: "x --auto"},
		{Orchestrator: "A", OrchestratorScope: strp("a=b")},
	} {
		if _, err := worktreecmd.TaskStateSet(context.Background(), repo, o); worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
			t.Errorf("%+v: err=%v, want exit 2", o, err)
		}
	}
	if _, err := os.Stat(taskFile(repo, "main")); !os.IsNotExist(err) {
		t.Error("a refused call must write nothing")
	}
}
