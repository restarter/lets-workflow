//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func recordOne(t *testing.T, dir string, o worktreecmd.RecordOptions) worktreecmd.TaskTrace {
	t.Helper()
	res, err := worktreecmd.TaskRecord(context.Background(), dir, o)
	if err != nil || !res.OK || len(res.Tasks) != 1 {
		t.Fatalf("TaskRecord: err=%v res=%+v", err, res)
	}
	return res.Tasks[0]
}

// Regression (lets-11zwo): a task in progress whose worktree is gone and which left
// no released marker must be reported, not ignored.
func TestRecord_OrphanWithoutMarkerIsReported(t *testing.T) {
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	installAdapter(t, repo, "beads", beadsConvention)
	runIn(t, repo, "git", "branch", "lets-orph-gone-work")
	mustWrite(t, taskFile(repo, "lets-orph-gone-work"), "task: lets-orph\n", 0o600)

	tr := recordOne(t, repo, worktreecmd.RecordOptions{Tasks: []string{"lets-orph"}})
	if !tr.Orphan || len(tr.Worktrees) != 0 || tr.Marker || tr.Record.State != worktreecmd.RecordMissing {
		t.Fatalf("orphan not reported: %+v", tr)
	}
	if len(tr.Branches) != 1 || tr.Branches[0] != "lets-orph-gone-work" || len(tr.TaskState) != 1 {
		t.Errorf("traces: %+v", tr)
	}
}

func TestRecord_NotOrphan(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-live1-work")
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-live1"}); err != nil {
		t.Fatal(err)
	}
	if tr := recordOne(t, repo, worktreecmd.RecordOptions{Tasks: []string{"lets-live1"}}); tr.Orphan || len(tr.Worktrees) != 1 {
		t.Errorf("a worktree holds it: %+v", tr)
	}
	if tr := recordOne(t, repo, worktreecmd.RecordOptions{Tasks: []string{"lets-none9"}}); tr.Orphan {
		t.Errorf("no local trace is never an orphan: %+v", tr)
	}
	runIn(t, repo, "git", "branch", "lets-mark1-work")
	mustWrite(t, filepath.Join(repo, ".lets", "cache", "released-lets-mark1"), "x\n", 0o600)
	if tr := recordOne(t, repo, worktreecmd.RecordOptions{Tasks: []string{"lets-mark1"}}); tr.Orphan || !tr.Marker {
		t.Errorf("a marker already surfaces it: %+v", tr)
	}
}

func TestRecord_PresentAtWorktreeHead(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-snap1-work")
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-snap1"}); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitOutput(t, wt, "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(repo, ".lets", "sessions", "2026-09-22-1200-lets-snap1-snapshot.md"), "### Record\n- head: "+head+"\n", 0o644)
	if tr := recordOne(t, repo, worktreecmd.RecordOptions{Tasks: []string{"lets-snap1"}}); tr.Record.State != worktreecmd.RecordPresent {
		t.Errorf("record = %+v", tr.Record)
	}
	runIn(t, wt, "git", "commit", "--allow-empty", "-m", "after")
	if tr := recordOne(t, repo, worktreecmd.RecordOptions{Tasks: []string{"lets-snap1"}, Ref: "lets-snap1-work"}); tr.Record.State != worktreecmd.RecordStale {
		t.Errorf("--ref past the snapshot: %+v", tr.Record)
	}
}

func TestRecord_UndeclaredConventionSkipsBranchesLoudly(t *testing.T) {
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	installAdapter(t, repo, "beads", "links: `.beads/.env` (0600).") // no id: - undeclared
	runIn(t, repo, "git", "branch", "feature/lets-und1-work")
	mustWrite(t, taskFile(repo, "feature/lets-und1-work"), "task: lets-und1\n", 0o600)
	res, err := worktreecmd.TaskRecord(context.Background(), repo, worktreecmd.RecordOptions{Tasks: []string{"lets-und1"}})
	if err != nil || len(res.Tasks) != 1 {
		t.Fatalf("TaskRecord: %v", err)
	}
	tr := res.Tasks[0]
	if len(tr.Branches) != 0 || !tr.Orphan || !stepsContain(res.Steps, "branch traces skipped") {
		t.Errorf("undeclared: branches must be skipped with a warning, the task-state trace still counts: %+v steps=%+v", tr, res.Steps)
	}
}

func TestRecord_Usage(t *testing.T) {
	repo := initRepo(t)
	for _, o := range []worktreecmd.RecordOptions{{}, {Tasks: []string{"-x"}}, {Tasks: []string{"a-1", "b-2"}, Ref: "main"}} {
		if _, err := worktreecmd.TaskRecord(context.Background(), repo, o); err == nil {
			t.Errorf("%+v must be refused", o)
		}
	}
}

// An inventory that failed half-way must not publish orphan=false (round 2, finding 3).
func TestRecord_UnreadableTraceFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 000 file")
	}
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	installAdapter(t, repo, "beads", beadsConvention)
	f := taskFile(repo, "lets-unr1-work")
	mustWrite(t, f, "task: lets-unr1\n", 0o600)
	if err := os.Chmod(f, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f, 0o600) })
	res, err := worktreecmd.TaskRecord(context.Background(), repo, worktreecmd.RecordOptions{Tasks: []string{"lets-unr1"}})
	if err == nil || res.OK || res.Error == nil || res.Error.Kind != "trace_unreadable" {
		t.Fatalf("an unreadable task-state file must fail the record: err=%v res=%+v", err, res)
	}
}

func TestRecord_UnreadableMarkerFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 000 dir")
	}
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	installAdapter(t, repo, "beads", beadsConvention)
	runIn(t, repo, "git", "branch", "lets-mk1-work")
	cache := filepath.Join(repo, ".lets", "cache")
	mustMkdir(t, cache)
	if err := os.Chmod(cache, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cache, 0o755) })
	res, err := worktreecmd.TaskRecord(context.Background(), repo, worktreecmd.RecordOptions{Tasks: []string{"lets-mk1"}})
	if err == nil || res.OK || len(res.Tasks) != 0 || res.Error == nil || res.Error.Kind != "trace_unreadable" {
		t.Fatalf("an unreadable marker must publish no rows: err=%v res=%+v", err, res)
	}
}

func TestRecord_ForEachRefFailureFails(t *testing.T) {
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	installAdapter(t, repo, "beads", beadsConvention)
	t.Cleanup(worktreecmd.SetForEachRef(func(context.Context, string) ([]byte, error) { return nil, errors.New("boom") }))
	res, err := worktreecmd.TaskRecord(context.Background(), repo, worktreecmd.RecordOptions{Tasks: []string{"lets-any1"}})
	if err == nil || res.OK || res.Error == nil || res.Error.Kind != "git_failed" {
		t.Fatalf("a failed branch scan must fail the record: err=%v res=%+v", err, res)
	}
}
