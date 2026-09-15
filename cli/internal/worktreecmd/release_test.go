//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
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
	if !regexp.MustCompile(`^lets-rel1\|lets-rel1-thing\|\d{4}-\d{2}-\d{2}T[^|]+\|dirty=true\|unpushed=true\n$`).Match(data) {
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
