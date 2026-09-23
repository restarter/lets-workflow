//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestRelease_RecordsDirtyUnpushedAndRemovesTaskState(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-rel1-thing")
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-rel1"}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(wt, "wip.txt"), "dirty", 0o644)
	res, err := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if err != nil || !res.OK {
		t.Fatalf("Release never refuses: err=%v", err)
	}
	r := res.Released
	if r.Task != "lets-rel1" || !r.Dirty || !r.Unpushed {
		t.Errorf("released = %+v", r)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".lets", "cache", "released-lets-rel1"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^lets-rel1\|lets-rel1-thing\|\d{4}-\d{2}-\d{2}T[^|]+\|dirty=true\|unpushed=true\|snapshot=missing\n$`).Match(data) {
		t.Errorf("marker line = %q", data)
	}
	if fi, _ := os.Stat(filepath.Join(repo, ".lets", "cache", "released-lets-rel1")); fi.Mode().Perm() != 0o600 {
		t.Errorf("marker mode %04o, want 0600", fi.Mode().Perm())
	}
	if _, err := os.Stat(taskFile(repo, "lets-rel1-thing")); !os.IsNotExist(err) {
		t.Error("task-state file must be removed")
	}
	if !stepsContain(res.Steps, "about to discard") {
		t.Errorf("dirty/unpushed must be named in a warning: %+v", res.Steps)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Error("release must never delete the checkout")
	}
}

func TestRelease_NoTaskNoMarker(t *testing.T) {
	repo, wt := adoptRepo(t, "no-task-here")
	res, err := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if err != nil || !res.OK || res.Released.Marker != "" || !stepsContain(res.Steps, "no released marker") {
		t.Fatalf("err=%v res=%+v", err, res.Released)
	}
	if m, _ := filepath.Glob(filepath.Join(repo, ".lets", "cache", "released-*")); len(m) != 0 {
		t.Errorf("markers %v", m)
	}
}

func TestRelease_MainCheckoutRefused(t *testing.T) {
	repo, _ := adoptRepo(t, "rel-main")
	if _, err := worktreecmd.Release(context.Background(), repo, worktreecmd.ReleaseOptions{}); worktreecmd.ExitCode(err) != worktreecmd.ExitNotLinkedWorktree {
		t.Errorf("err = %v, want exit 23", err)
	}
}

func TestRelease_UnreadableTaskStateIsKept(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 000 file")
	}
	repo, wt := adoptRepo(t, "lets-keep1-thing")
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-keep1"}); err != nil {
		t.Fatal(err)
	}
	f := taskFile(repo, "lets-keep1-thing")
	if err := os.Chmod(f, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f, 0o600) })
	res, _ := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if _, err := os.Stat(f); err != nil {
		t.Fatal("an unreadable task-state file must be kept")
	}
	if res.Released.Kept != worktreecmd.KeepUnreadable || !strings.Contains(res.Alarm(), "unreadable") {
		t.Errorf("an unreadable file is kept and says why: %+v alarm=%q", res.Released, res.Alarm())
	}
}

func TestRelease_InvalidTaskIDIsKept(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-bad1-thing")
	mustWrite(t, taskFile(repo, "lets-bad1-thing"), "task: -evil\n", 0o600)
	res, _ := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if _, err := os.Stat(taskFile(repo, "lets-bad1-thing")); err != nil {
		t.Fatal("an invalid id must not delete the evidence")
	}
	if res.Released.Marker != "" || res.Released.Kept != worktreecmd.KeepInvalidID || !strings.Contains(res.Alarm(), "invalid_id") {
		t.Errorf("released = %+v alarm=%q", res.Released, res.Alarm())
	}
}

func TestRelease_PresentRecordIsQuiet(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-ok1-thing")
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-ok1"}); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitOutput(t, wt, "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(repo, ".lets", "sessions", "2026-09-22-1200-lets-ok1-snapshot.md"), "- head: "+head+"\n", 0o644)
	res, err := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if err != nil || res.Released.Snapshot != worktreecmd.RecordPresent || res.Alarm() != "" {
		t.Fatalf("released = %+v alarm=%q", res.Released, res.Alarm())
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".lets", "cache", "released-lets-ok1"))
	if !strings.HasSuffix(string(data), "|snapshot=present\n") {
		t.Errorf("marker = %q", data)
	}
}

// A closed task's file (done.md cleanup kept only session:) is removed without a
// marker and without an alarm - alarming here would fire on every ordinary archive.
func TestRelease_TasklessFileRemovedQuietly(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-done1-thing")
	mustWrite(t, taskFile(repo, "lets-done1-thing"), "session: 0123456789012345678901234567890123456789 aaaaaaaa-0000-4000-8000-000000000001\norc: MAIN\n", 0o600)
	res, err := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if err != nil || res.Released.Marker != "" || res.Released.Kept != "" || res.Alarm() != "" {
		t.Fatalf("taskless release: err=%v %+v alarm=%q", err, res.Released, res.Alarm())
	}
	if _, err := os.Stat(taskFile(repo, "lets-done1-thing")); !os.IsNotExist(err) {
		t.Error("a file naming no task is removed")
	}
}

// A writer that changes the file after the marker was written keeps its revision.
func TestRelease_ChangedDuringReleaseIsKept(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-race1-thing")
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-race1"}); err != nil {
		t.Fatal(err)
	}
	f := taskFile(repo, "lets-race1-thing")
	t.Cleanup(worktreecmd.SetBeforeTaskStateRemove(func() { mustWrite(t, f, "task: lets-race2\n", 0o600) }))
	res, _ := worktreecmd.Release(context.Background(), wt, worktreecmd.ReleaseOptions{})
	if res.Released.Marker == "" || res.Released.Kept != worktreecmd.KeepChanged {
		t.Fatalf("released = %+v", res.Released)
	}
	if data, _ := os.ReadFile(f); string(data) != "task: lets-race2\n" {
		t.Errorf("the newer revision must survive: %q", data)
	}
}
