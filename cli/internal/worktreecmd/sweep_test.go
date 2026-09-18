//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// mergedUpstreamBranch creates branch with one commit, pushes it to origin/main, and
// moves origin/main one commit further (so the branch tip is not the merge tip).
func mergedUpstreamBranch(t *testing.T, repo, branch string) {
	t.Helper()
	commitOnBranch(t, repo, branch, "origin/main", branch)
	runIn(t, repo, "git", "push", "-q", "origin", branch+":main")
	commitOnBranch(t, repo, "tmp-after-"+filepath.Base(branch), branch, "after")
	runIn(t, repo, "git", "push", "-q", "origin", "tmp-after-"+filepath.Base(branch)+":main")
	runIn(t, repo, "git", "branch", "-D", "tmp-after-"+filepath.Base(branch))
	runIn(t, repo, "git", "fetch", "-q", "origin")
}

func TestSweep_ConventionShapesOnly(t *testing.T) {
	repo := initRepoWithOrigin(t)
	mustMkdir(t, filepath.Join(repo, ".lets"))
	installAdapter(t, repo, "beads", beadsConvention)
	mergedUpstreamBranch(t, repo, "feature/lets-abc-done")
	mergedUpstreamBranch(t, repo, "feature/notatask")
	commitOnBranch(t, repo, "worktree-lets-xyz-wip", "origin/main", "wip")

	res, err := worktreecmd.Sweep(context.Background(), repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Merged, []string{"feature/lets-abc-done"}) || !slices.Equal(res.Unmerged, []string{"worktree-lets-xyz-wip"}) {
		t.Errorf("merged=%v unmerged=%v", res.Merged, res.Unmerged)
	}
	if out, _ := exec.Command("git", "-C", repo, "branch", "--list", "feature/lets-abc-done").Output(); len(out) == 0 {
		t.Error("dry run must not delete")
	}
	res, err = worktreecmd.Sweep(context.Background(), repo, true)
	if err != nil || !slices.Equal(res.Deleted, []string{"feature/lets-abc-done"}) {
		t.Fatalf("apply: err=%v deleted=%v", err, res.Deleted)
	}
	if out, _ := exec.Command("git", "-C", repo, "branch", "--list", "worktree-lets-xyz-wip").Output(); len(out) == 0 {
		t.Error("an unmerged branch must never be deleted")
	}
}

func TestSweep_LeadingIDTemplateNeverWidens(t *testing.T) {
	repo := initRepoWithOrigin(t)
	mustMkdir(t, filepath.Join(repo, ".lets"))
	installAdapter(t, repo, "beads", beadsConvention)
	if err := os.WriteFile(filepath.Join(repo, ".claude", "rules", "tracker-beads.board.md"),
		[]byte("## Worktree\n\nid: `PWA-[0-9]+`.\nbranch: `{id}-{slug}`.\nworktree-branch: `{id}-{slug}`.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mergedUpstreamBranch(t, repo, "release/1.0")
	mergedUpstreamBranch(t, repo, "PWA-12-fix")

	res, err := worktreecmd.Sweep(context.Background(), repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(res.Merged, "release/1.0") || !slices.Contains(res.Merged, "PWA-12-fix") {
		t.Errorf("merged = %v: release/1.0 must never be a candidate, PWA-12-fix must", res.Merged)
	}
}

func TestRemove_AlreadyGoneUsesConventionCandidate(t *testing.T) {
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets"))
	installAdapter(t, repo, "beads", beadsConvention)
	runIn(t, repo, "git", "branch", "feature/lets-abc-fix-login")
	// Orca named the worktree lets-abc-fix-login; its branch is the convention's feature/ shape
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "lets-abc-fix-login", DeleteBranch: true})
	if err != nil || !res.Removed.AlreadyGone || res.Removed.Branch != "feature/lets-abc-fix-login" {
		t.Fatalf("err=%v removed=%+v", err, res.Removed)
	}
}
