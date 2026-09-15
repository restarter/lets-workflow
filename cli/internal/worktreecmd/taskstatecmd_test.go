//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	defer f.Close()
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
