//go:build unix

package orcacmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// orcaTeam is a team worktree Orca created and lists, on team_snake at origin/main;
// its rm callback runs `git worktree remove` and drops the ps row.
func orcaTeam(t *testing.T) (teamRepo, *fakeTeamOrca, string) {
	t.Helper()
	r := newTeamRepo(t)
	f := &fakeTeamOrca{}
	p := r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", true)
	gitT(t, p, "branch", "--set-upstream-to=origin/main")
	f.rm = func([]string) (string, bool) {
		gitT(t, r.repo, "worktree", "remove", p)
		f.rows = nil
		return `{"ok":true}`, false
	}
	return r, r.fake(t, f), p
}

func teamRemove(r teamRepo) (*TeamRemoveResult, error) {
	return TeamRemove(context.Background(), TeamRemoveOptions{Repo: r.repo, Name: "team_snake"})
}

func TestTeamRemove_Argv(t *testing.T) {
	r, f, p := orcaTeam(t)
	res, err := teamRemove(r)
	if err != nil || res.Team.State != TeamRemoved {
		t.Fatalf("remove: %+v, %v", res.Team, err)
	}
	want := []string{"worktree", "rm", "--worktree", "path:" + p, "--run-hooks", "--json"}
	for _, c := range *f.calls {
		if len(c.args) > 1 && c.args[1] == "rm" {
			if !slices.Equal(c.args, want) {
				t.Errorf("argv = %q\nwant  %q", c.args, want)
			}
			if slices.Contains(c.args, "--force") || slices.Contains(c.args, "--allow-failed-archive-hook") {
				t.Error("team-remove is never forced and never waives the archive hook")
			}
		}
	}
}

func TestTeamRemove_RefusesNonTeamName(t *testing.T) {
	r, f, _ := orcaTeam(t)
	_, err := TeamRemove(context.Background(), TeamRemoveOptions{Repo: r.repo, Name: "snake"})
	wantExit(t, err, ExitUsage, "name_invalid")
	if len(*f.calls) != 0 {
		t.Error("a refused name reached Orca")
	}
}

func TestTeamRemove_OrcaAbsentNotAttempted(t *testing.T) {
	r, f, p := orcaTeam(t)
	lookOrca = func() (string, bool) { return "", false }
	res, err := teamRemove(r)
	if err != nil || res.Team.State != TeamRemoveNotAttempted || res.Team.Attempted || f.count("worktree rm") != 0 {
		t.Errorf("orca absent: %+v, %v", res.Team, err)
	}
	if _, serr := os.Stat(p); serr != nil {
		t.Error("Go never removes an Orca worktree")
	}
}

func TestTeamRemove_NotListed(t *testing.T) {
	r, f, _ := orcaTeam(t)
	f.rows = nil
	_, err := teamRemove(r)
	wantExit(t, err, ExitNotListed, "not_listed")
}

func TestTeamRemove_DirtyRefusedNoRm(t *testing.T) {
	r, f, p := orcaTeam(t)
	if err := os.WriteFile(filepath.Join(p, "tracked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, p, "add", "tracked.txt")
	_, err := teamRemove(r)
	wantExit(t, err, ExitDirtyWorktree, "dirty_worktree")
	if f.count("worktree rm") != 0 {
		t.Error("a dirty worktree reached rm")
	}
}

func TestTeamRemove_UntrackedRefusedNoRm(t *testing.T) {
	r, f, p := orcaTeam(t)
	if err := os.WriteFile(filepath.Join(p, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := teamRemove(r)
	wantExit(t, err, ExitDirtyWorktree, "dirty_worktree")
	if !strings.Contains(err.Error(), "new.txt") || f.count("worktree rm") != 0 {
		t.Errorf("the untracked path must be named and no rm run: %v", err)
	}
}

func TestTeamRemove_UnpushedRefusedNoRm(t *testing.T) {
	r, f, p := orcaTeam(t)
	gitT(t, p, "-c", "user.email=t@e", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "local work")
	_, err := teamRemove(r)
	wantExit(t, err, ExitUnpushedCommits, "unpushed_commits")
	if !strings.Contains(err.Error(), "not_pushed") || f.count("worktree rm") != 0 {
		t.Errorf("the state must be named and no rm run: %v", err)
	}
}

func TestTeamRemove_UnverifiedRefusedNoRm(t *testing.T) {
	r, f, _ := orcaTeam(t)
	orig := remoteCheck
	remoteCheck = func(context.Context, string, string, time.Duration) (string, string, string, error) {
		return gitutil.RemoteUnverified, "", "", nil
	}
	t.Cleanup(func() { remoteCheck = orig })
	_, err := teamRemove(r)
	wantExit(t, err, ExitUnpushedCommits, "unpushed_commits")
	if !strings.Contains(err.Error(), "unverified") || f.count("worktree rm") != 0 {
		t.Errorf("unverified is refused like unpushed: %v", err)
	}
}

func TestTeamRemove_ArchiveHookFailedSurfaced(t *testing.T) {
	r, f, p := orcaTeam(t)
	f.rm = func([]string) (string, bool) {
		return `{"ok":false,"error":{"code":"worktree_archive_hook_failed","message":"hook exited 1: port still bound"}}`, true
	}
	res, err := teamRemove(r)
	wantExit(t, err, ExitArchiveHookFailed, "archive_hook_failed")
	if !strings.Contains(err.Error(), "port still bound") || f.count("worktree rm") != 1 || res.Team.State != TeamArchiveHookFailed {
		t.Errorf("the hook output is shown and rm is not retried: %v (rm calls %d)", err, f.count("worktree rm"))
	}
	if _, serr := os.Stat(p); serr != nil {
		t.Error("the worktree stays after a failed archive hook")
	}
}

func TestTeamRemove_RemovedConfirmedByGitAndPs(t *testing.T) {
	r, _, p := orcaTeam(t)
	res, err := teamRemove(r)
	if err != nil || res.Team.State != TeamRemoved || !res.Team.Attempted {
		t.Fatalf("remove: %+v, %v", res.Team, err)
	}
	if strings.Contains(gitT(t, r.repo, "worktree", "list", "--porcelain"), p) {
		t.Error("git still lists the removed worktree")
	}
}

func TestTeamRemove_StillListedAmbiguous(t *testing.T) {
	r, f, _ := orcaTeam(t)
	f.rm = func([]string) (string, bool) { return `{"ok":true}`, false } // claims success, removes nothing
	res, err := teamRemove(r)
	wantExit(t, err, ExitRemoveAmbiguous, "remove_ambiguous")
	if !strings.Contains(err.Error(), "git still lists") || !strings.Contains(err.Error(), "Orca still lists") || res.Team.State != TeamRemoveAmbiguous || f.count("worktree rm") != 1 {
		t.Errorf("ambiguous names both sides, no retry: %v", err)
	}
}
