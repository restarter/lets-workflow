//go:build unix

package worktreecmd

import (
	"context"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// parkBranch makes a local branch and records a park on it for team ("" = none).
func parkBranch(t *testing.T, repo, letsDir, branch, task, team string) string {
	t.Helper()
	gitOut(t, repo, "branch", branch)
	sha := gitOut(t, repo, "rev-parse", branch)
	set := map[string]string{"task": task, "park": sha}
	if team != "" {
		set["park_team"] = team
	}
	if _, err := taskstate.MergeWrite(letsDir, slugOf(branch), taskstate.WriteOpts{Set: set, Create: true}); err != nil {
		t.Fatal(err)
	}
	return sha
}

func TestParked_TwoTeamsFiltered(t *testing.T) {
	repo, letsDir := recordRepo(t)
	sha := parkBranch(t, repo, letsDir, "feature/lets-a1-x", "lets-a1", "snake")
	parkBranch(t, repo, letsDir, "feature/lets-b2-x", "lets-b2", "frog")
	res, err := Parked(context.Background(), repo, "snake")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Owned) != 1 || res.Owned[0] != (ParkedEntry{Task: "lets-a1", Branch: "feature/lets-a1-x", ParkSha: sha, Tip: sha}) || len(res.Unowned) != 0 {
		t.Errorf("snake sees %+v / %+v - frog's park must not appear", res.Owned, res.Unowned)
	}
	res, _ = Parked(context.Background(), repo, "frog")
	if len(res.Owned) != 1 || res.Owned[0].Task != "lets-b2" {
		t.Errorf("frog sees %+v", res.Owned)
	}
}

func TestParked_UnownedSeparate(t *testing.T) {
	repo, letsDir := recordRepo(t)
	parkBranch(t, repo, letsDir, "feature/lets-a1-x", "lets-a1", "snake")
	parkBranch(t, repo, letsDir, "feature/lets-c3-x", "lets-c3", "")
	res, err := Parked(context.Background(), repo, "snake")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Owned) != 1 || len(res.Unowned) != 1 || res.Unowned[0].Task != "lets-c3" {
		t.Errorf("owned %+v, unowned %+v", res.Owned, res.Unowned)
	}
	if _, err := Parked(context.Background(), repo, "Bad Team"); err == nil {
		t.Error("an invalid --team must be refused")
	}
}

func TestParked_EmptyAfterUnpark(t *testing.T) {
	f := newTeamFixture(t)
	parkAndLeave(t, f)
	res, err := Parked(context.Background(), f.wt, "snake")
	if err != nil || len(res.Owned) != 1 {
		t.Fatalf("after the park: %+v, %v", res, err)
	}
	if _, err := f.sw(SwitchOptions{Task: "lets-a1"}); err != nil {
		t.Fatal(err)
	}
	res, err = Parked(context.Background(), f.wt, "snake")
	if err != nil || len(res.Owned) != 0 || len(res.Unowned) != 0 {
		t.Errorf("after the unpark: %+v, %v", res, err)
	}
}
