//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"path/filepath"
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
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return home }
	ccregistry.ProcAlive = func(pid int) bool { return pid == 4343 }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
	return sid
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
	tsSet(t, repo, worktreecmd.TaskStateOptions{Orchestrator: "MAIN-LETS", OrchestratorScope: strp("docs"), Standing: strp("on")})
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

// Orchestrator records are written only on the merge-branch file.
func TestTaskStateSet_OrchestratorOffMergeBranch(t *testing.T) {
	repo := tsRepo(t, "feature/lets-abc-x")
	mustWrite(t, taskFile(repo, "feature/lets-abc-x"), "task: lets-abc\n", 0o644)
	for _, o := range []worktreecmd.TaskStateOptions{
		{Orchestrator: "MAIN-LETS"},
		{ClearOrchestrator: "MAIN-LETS"},
		{SelfOrchestrator: true, Standing: strp("on")},
	} {
		res, err := worktreecmd.TaskStateSet(context.Background(), repo, o)
		if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage || res.Error == nil || res.Error.Kind != "orchestrator_off_merge_branch" {
			t.Errorf("%+v: err=%v res=%+v", o, err, res.Error)
		}
	}
	if got := readTask(t, repo, "feature/lets-abc-x"); got != "task: lets-abc\n" {
		t.Errorf("file changed: %s", got)
	}
	if _, err := os.Stat(taskFile(repo, "main")); !os.IsNotExist(err) {
		t.Error("nothing may be written on the merge-branch file from another branch")
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
		{Orchestrator: "A", Standing: strp("maybe")},
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
