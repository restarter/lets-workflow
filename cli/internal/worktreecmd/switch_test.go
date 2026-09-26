//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/memberscmd"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// teamFixture is a main checkout pushed to a bare origin, the permissive tracker
// convention, and a linked team worktree `team_snake` claimed by the team file
// snake.md. a.txt is tracked.
type teamFixture struct {
	repo, wt, letsDir, origin string
}

func newTeamFixture(t *testing.T) teamFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	repo, letsDir := recordRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", "a.txt")
	gitOut(t, repo, "commit", "-q", "-m", "a")
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitOut(t, repo, "init", "-q", "--bare", "-b", "main", origin)
	gitOut(t, repo, "remote", "add", "origin", origin)
	gitOut(t, repo, "push", "-q", "-u", "origin", "main")
	rules := filepath.Join(repo, ".claude", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "tracker-beads.md"), []byte("# Tracker adapter: beads\n\n## Worktree\n\n"+permissiveConvention+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, _ := filepath.EvalSymlinks(t.TempDir())
	wt = filepath.Join(wt, "team_snake")
	gitOut(t, repo, "worktree", "add", "-q", "-b", "team_snake", wt)
	f := teamFixture{repo: repo, wt: wt, letsDir: letsDir, origin: origin}
	f.writeTeamFile(t, "snake")
	return f
}

func (f teamFixture) writeTeamFile(t *testing.T, team string) {
	t.Helper()
	gd := gitOut(t, f.wt, "rev-parse", "--absolute-git-dir")
	body := fmt.Sprintf("---\nteam: %q\nworktree: %q\ngit_dir: %q\n---\n", team, f.wt, gd)
	if err := os.MkdirAll(filepath.Join(f.letsDir, "teams"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.letsDir, "teams", team+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f teamFixture) sw(o SwitchOptions) (*SwitchResult, error) {
	return Switch(context.Background(), f.wt, o)
}

func (f teamFixture) state(t *testing.T, branch string) taskstate.State {
	t.Helper()
	st, _ := taskstate.Read(f.letsDir, slugOf(branch))
	return st
}

func (f teamFixture) write(t *testing.T, name, body string) {
	t.Helper()
	p := filepath.Join(f.wt, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// onTask puts the team worktree on task id's new branch and returns that branch.
func (f teamFixture) onTask(t *testing.T, id string) string {
	t.Helper()
	res, err := f.sw(SwitchOptions{Task: id})
	if err != nil || !res.OK || !res.Created {
		t.Fatalf("switch to %s: %+v, %v", id, res, err)
	}
	return res.Branch
}

// indexSnap is what a refused switch must leave alone: the index, HEAD, the tree.
func indexSnap(t *testing.T, dir string) string {
	t.Helper()
	return gitOut(t, dir, "ls-files", "-s") + "|" + gitOut(t, dir, "diff", "--cached", "--name-only") + "|" + gitOut(t, dir, "rev-parse", "HEAD") + "|" + gitOut(t, dir, "status", "--porcelain", "--untracked-files=all")
}

func wantKind(t *testing.T, err error, code int, kind string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code || e.Kind != kind {
		t.Fatalf("err = %v, want exit %d kind %s", err, code, kind)
	}
}

func TestSwitch_NotTeamWorktree(t *testing.T) {
	f := newTeamFixture(t)
	if err := os.Remove(filepath.Join(f.letsDir, "teams", "snake.md")); err != nil {
		t.Fatal(err)
	}
	_, err := f.sw(SwitchOptions{Task: "lets-a1"})
	wantKind(t, err, ExitNotTeamWorktree, "not_team_worktree")
}

func TestSwitch_TargetIsMergeBranch(t *testing.T) {
	f := newTeamFixture(t)
	_, err := f.sw(SwitchOptions{Task: "lets-a1", Branch: "main"})
	wantKind(t, err, ExitTargetIsMergeBranch, "target_is_merge_branch")
}

// fakeLead registers a live lead session for the members registry and returns
// Options for scope in this fixture's team worktree.
func fakeLead(t *testing.T, f teamFixture, scope string) memberscmd.Options {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	sid := "aaaaaaaa-0000-4000-8000-000000000001"
	if err := os.WriteFile(filepath.Join(home, "sessions", "4242.json"), []byte(`{"name":"snake-lead","sessionId":"`+sid+`","cwd":"/x","peerProtocol":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return home }
	ccregistry.ProcAlive = func(pid int) bool { return pid == 4242 }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
	oldSleep := memberscmd.Sleep
	memberscmd.Sleep = func(time.Duration) {}
	t.Cleanup(func() { memberscmd.Sleep = oldSleep })
	return memberscmd.Options{Root: f.repo, CallerToplevel: f.wt, Session: sid, Scope: scope}
}

func TestSwitch_MembersLive(t *testing.T) {
	f := newTeamFixture(t)
	o := fakeLead(t, f, "run-abc123")
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "impl-abc123", Role: "implementer"}); err != nil {
		t.Fatal(err)
	}
	before := indexSnap(t, f.wt)
	_, err := f.sw(SwitchOptions{Task: "lets-a1"})
	wantKind(t, err, ExitMembersLive, "members_live")
	if !strings.Contains(err.Error(), "run-abc123/impl-abc123") {
		t.Errorf("the refusal must name the member: %v", err)
	}
	if indexSnap(t, f.wt) != before {
		t.Error("a refused switch touched the tree")
	}
}

func TestSwitch_DirtyWithoutPark(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	f.write(t, "a.txt", "changed\n")
	_, err := f.sw(SwitchOptions{Task: "lets-b2"})
	wantKind(t, err, ExitDirtyWorktree, "dirty_worktree")
}

func TestSwitch_UnmergedIndexRefused(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	gitOut(t, f.wt, "switch", "-q", "-c", "side")
	f.write(t, "a.txt", "side\n")
	gitOut(t, f.wt, "commit", "-q", "-am", "side")
	gitOut(t, f.wt, "switch", "-q", "-")
	f.write(t, "a.txt", "ours\n")
	gitOut(t, f.wt, "commit", "-q", "-am", "ours")
	if err := exec.Command("git", "-C", f.wt, "merge", "-q", "side").Run(); err == nil {
		t.Fatal("the merge was expected to conflict")
	}
	before := indexSnap(t, f.wt)
	_, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true, Include: []string{"a.txt"}})
	wantKind(t, err, ExitDirtyWorktree, "index_unmerged")
	if indexSnap(t, f.wt) != before || gitOut(t, f.wt, "ls-files", "-u") == "" {
		t.Error("the unmerged index was touched")
	}
}

func TestSwitch_ParkTrackedOnly(t *testing.T) {
	f := newTeamFixture(t)
	a1 := f.onTask(t, "lets-a1")
	f.write(t, "a.txt", "work in progress\n")
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	if err != nil || !res.OK || res.Park == nil {
		t.Fatalf("park: %+v, %v", res, err)
	}
	if subj := gitOut(t, f.repo, "log", "-1", "--format=%s", a1); subj != "wip(lets-a1): park" {
		t.Errorf("park subject = %q", subj)
	}
	if res.Park.Sha != gitOut(t, f.repo, "rev-parse", a1) || strings.Join(res.Park.Files, ",") != "a.txt" {
		t.Errorf("park = %+v", res.Park)
	}
	if gitOut(t, f.wt, "status", "--porcelain") != "" || gitOut(t, f.wt, "branch", "--show-current") != res.Branch {
		t.Error("after a park the tree must be clean on the new branch")
	}
}

func TestSwitch_ParkRecordsTeam(t *testing.T) {
	f := newTeamFixture(t)
	a1 := f.onTask(t, "lets-a1")
	f.write(t, "a.txt", "wip\n")
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	if err != nil {
		t.Fatal(err)
	}
	if st := f.state(t, a1); st.Park != res.Park.Sha || st.ParkTeam != "snake" || st.Task != "lets-a1" {
		t.Errorf("old branch task-state = %+v", st)
	}
}

func TestSwitch_UntrackedPresent(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	f.write(t, "a.txt", "tracked change\n")
	f.write(t, "notes/new.txt", "new\n")
	before := indexSnap(t, f.wt)
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	wantKind(t, err, ExitUntrackedPresent, "untracked_present")
	if len(res.NewPaths) != 1 || res.NewPaths[0] != (NewPath{Path: "notes/new.txt", Source: "untracked"}) {
		t.Errorf("new paths = %+v", res.NewPaths)
	}
	if indexSnap(t, f.wt) != before {
		t.Error("a refused park touched the index or HEAD")
	}
}

// Codex r1: a refused switch leaves the index byte-identical.
func TestSwitch_RefusalLeavesIndex(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	f.write(t, "a.txt", "staged change\n")
	gitOut(t, f.wt, "add", "a.txt")
	f.write(t, "loose.txt", "x\n")
	tree := gitOut(t, f.wt, "write-tree")
	cached := gitOut(t, f.wt, "diff", "--cached", "--name-only")
	if _, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true}); err == nil {
		t.Fatal("expected a refusal")
	}
	if gitOut(t, f.wt, "write-tree") != tree || gitOut(t, f.wt, "diff", "--cached", "--name-only") != cached {
		t.Error("the index changed across a refused switch")
	}
}

func TestSwitch_IncludeParks(t *testing.T) {
	f := newTeamFixture(t)
	a1 := f.onTask(t, "lets-a1")
	f.write(t, "notes/new.txt", "new\n")
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true, Include: []string{"notes/new.txt"}})
	if err != nil || !res.OK {
		t.Fatalf("park with include: %+v, %v", res, err)
	}
	if !strings.Contains(gitOut(t, f.repo, "show", "--name-only", "--format=", a1), "notes/new.txt") {
		t.Error("the included path is not in the park commit")
	}
	f.write(t, "loose.txt", "x\n")
	if _, err := f.sw(SwitchOptions{Task: "lets-c3", Park: true, Include: []string{"../outside.txt"}}); err == nil {
		t.Error("an include outside the worktree must be refused")
	}
}

func TestSwitch_StagedNewFileNeedsInclude(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	f.write(t, "staged.txt", "s\n")
	gitOut(t, f.wt, "add", "staged.txt")
	tree, head := gitOut(t, f.wt, "write-tree"), gitOut(t, f.wt, "rev-parse", "HEAD")
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	wantKind(t, err, ExitUntrackedPresent, "untracked_present")
	if len(res.NewPaths) != 1 || res.NewPaths[0] != (NewPath{Path: "staged.txt", Source: "staged"}) {
		t.Errorf("new paths = %+v", res.NewPaths)
	}
	if gitOut(t, f.wt, "write-tree") != tree || gitOut(t, f.wt, "rev-parse", "HEAD") != head {
		t.Error("the index or HEAD changed")
	}
}

func TestSwitch_StagedNewFileIncludedParks(t *testing.T) {
	f := newTeamFixture(t)
	a1 := f.onTask(t, "lets-a1")
	f.write(t, "staged.txt", "s\n")
	gitOut(t, f.wt, "add", "staged.txt")
	if _, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true, Include: []string{"staged.txt"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gitOut(t, f.repo, "show", "--name-only", "--format=", a1), "staged.txt") {
		t.Error("the staged new file is not in the park commit")
	}
}

func TestSwitch_StagedRenameTargetNeedsInclude(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	gitOut(t, f.wt, "mv", "a.txt", "b.txt")
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	wantKind(t, err, ExitUntrackedPresent, "untracked_present")
	if len(res.NewPaths) != 1 || res.NewPaths[0].Path != "b.txt" || res.NewPaths[0].Source != "staged" {
		t.Errorf("new paths = %+v", res.NewPaths)
	}
}

func TestSwitch_NoRemoteBase(t *testing.T) {
	f := newTeamFixture(t)
	gitOut(t, f.repo, "remote", "remove", "origin")
	_, err := f.sw(SwitchOptions{Task: "lets-a1"})
	wantKind(t, err, ExitNoRemoteBase, "no_remote_base")
	if gitOut(t, f.wt, "branch", "--show-current") != "team_snake" {
		t.Error("a refused switch moved the worktree")
	}
}

func TestSwitch_StaleFetchWarning(t *testing.T) {
	f := newTeamFixture(t)
	gitOut(t, f.repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	res, err := f.sw(SwitchOptions{Task: "lets-a1"})
	if err != nil || !res.BaseStale || res.Base != gitOut(t, f.repo, "rev-parse", "origin/main") {
		t.Fatalf("stale base: %+v, %v", res, err)
	}
	warned := false
	for _, s := range res.Steps {
		warned = warned || (s.Status == StepWarn && strings.Contains(s.Message, "last fetched"))
	}
	if !warned {
		t.Errorf("no staleness warning: %+v", res.Steps)
	}
}

func TestSwitch_TaskStateCarry(t *testing.T) {
	f := newTeamFixture(t)
	a1 := f.onTask(t, "lets-a1")
	if _, err := taskstate.MergeWrite(f.letsDir, slugOf(a1), taskstate.WriteOpts{Set: map[string]string{"orc": "main-orc"}}); err != nil {
		t.Fatal(err)
	}
	res, err := f.sw(SwitchOptions{Task: "lets-b2"})
	if err != nil {
		t.Fatal(err)
	}
	st := f.state(t, res.Branch)
	if st.Task != "lets-b2" || st.Start != gitOut(t, f.repo, "rev-parse", "origin/main") || st.Orc != "main-orc" {
		t.Errorf("new branch task-state = %+v", st)
	}
	// an existing branch keeps its own start
	if _, err := taskstate.MergeWrite(f.letsDir, slugOf(a1), taskstate.WriteOpts{Set: map[string]string{"start": "abcdef1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sw(SwitchOptions{Task: "lets-a1"}); err != nil {
		t.Fatal(err)
	}
	if st := f.state(t, a1); st.Start != "abcdef1" {
		t.Errorf("an existing branch lost its start: %+v", st)
	}
}

func TestSwitch_SessionHeldStops(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	f.write(t, "a.txt", "wip\n")
	before := indexSnap(t, f.wt)
	g := taskstate.SessionGuard{Lead: func(string) (bool, string) { return false, "snake-lead (session 11111111)" }}
	_, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true, Session: "bbbbbbbb-0000-4000-8000-000000000002", Guard: g})
	wantKind(t, err, ExitGeneric, "session_held")
	if !strings.Contains(err.Error(), "snake-lead") {
		t.Errorf("the refusal must name the holder: %v", err)
	}
	if indexSnap(t, f.wt) != before {
		t.Error("a held switch parked or moved anything")
	}
}

// parkAndLeave parks lets-a1 with a tracked change and moves to lets-b2.
func parkAndLeave(t *testing.T, f teamFixture) (a1, parkSha, pre string) {
	t.Helper()
	a1 = f.onTask(t, "lets-a1")
	pre = gitOut(t, f.wt, "rev-parse", "HEAD")
	f.write(t, "a.txt", "wip\n")
	res, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	if err != nil {
		t.Fatal(err)
	}
	return a1, res.Park.Sha, pre
}

func TestSwitch_Unpark(t *testing.T) {
	f := newTeamFixture(t)
	a1, _, pre := parkAndLeave(t, f)
	res, err := f.sw(SwitchOptions{Task: "lets-a1"})
	if err != nil || res.Unpark != Unparked {
		t.Fatalf("unpark: %+v, %v", res, err)
	}
	if gitOut(t, f.wt, "rev-parse", "HEAD") != pre || gitOut(t, f.wt, "diff", "--cached", "--name-only") != "a.txt" {
		t.Error("the park commit must be undone with its changes staged")
	}
	_ = a1
}

func TestSwitch_UnparkClearsParkKeys(t *testing.T) {
	f := newTeamFixture(t)
	a1, _, _ := parkAndLeave(t, f)
	if _, err := f.sw(SwitchOptions{Task: "lets-a1"}); err != nil {
		t.Fatal(err)
	}
	if st := f.state(t, a1); st.Park != "" || st.ParkTeam != "" || st.Task != "lets-a1" {
		t.Errorf("park keys not cleared: %+v", st)
	}
}

func TestSwitch_ParkedPushed(t *testing.T) {
	f := newTeamFixture(t)
	a1, sha, _ := parkAndLeave(t, f)
	gitOut(t, f.repo, "push", "-q", f.origin, a1+":refs/heads/shared")
	res, err := f.sw(SwitchOptions{Task: "lets-a1"})
	if err != nil || res.Unpark != ParkedPushed || res.UnparkHead != "origin/shared" {
		t.Fatalf("pushed park: %+v, %v", res, err)
	}
	if gitOut(t, f.wt, "rev-parse", "HEAD") != sha || f.state(t, a1).Park != sha {
		t.Error("a pushed park must stay, keys included")
	}
}

func TestSwitch_UnparkUnverifiedKept(t *testing.T) {
	f := newTeamFixture(t)
	a1, sha, _ := parkAndLeave(t, f)
	orig := remotesContainAny
	remotesContainAny = func(context.Context, string, string, time.Duration) (string, string, string, error) {
		return gitutil.RemoteUnverified, "", "", errors.New("timeout")
	}
	t.Cleanup(func() { remotesContainAny = orig })
	res, err := f.sw(SwitchOptions{Task: "lets-a1"})
	if err != nil || res.Unpark != ParkedUnverified {
		t.Fatalf("unverified: %+v, %v", res, err)
	}
	if gitOut(t, f.wt, "rev-parse", "HEAD") != sha || f.state(t, a1).Park != sha || f.state(t, a1).ParkTeam != "snake" {
		t.Error("an unverified park must stay, keys included")
	}
}

func TestSwitch_UnparkStatePartialNamed(t *testing.T) {
	f := newTeamFixture(t)
	a1, _, pre := parkAndLeave(t, f)
	orig := unparkClear
	unparkClear = func(string, string) error { return errors.New("disk full") }
	t.Cleanup(func() { unparkClear = orig })
	res, err := f.sw(SwitchOptions{Task: "lets-a1"})
	wantKind(t, err, ExitFilesystem, "unpark_state_partial")
	if res.OK || !strings.Contains(err.Error(), taskstate.Path(f.letsDir, slugOf(a1))) || !strings.Contains(res.Error.Remediation, "park_team:") {
		t.Errorf("the partial state must be named: %+v, %v", res.Error, err)
	}
	if gitOut(t, f.wt, "rev-parse", "HEAD") != pre {
		t.Error("the reset is not undone when the key delete fails")
	}
}

// Owner decision: only a live WRITER in this tree blocks - the team's read-only
// members and an isolated implementer never do.
func TestSwitch_ReadOnlyAndIsolatedMembersDoNotBlock(t *testing.T) {
	f := newTeamFixture(t)
	o := fakeLead(t, f, "run-abc123")
	for _, a := range []memberscmd.AddOptions{
		{Name: "architect", Role: "architect"},
		{Name: "skeptic", Role: "lets:skeptic"},
		{Name: "impl-abc123-a", Role: "implementer", Isolation: "worktree", WorktreePath: filepath.Join(t.TempDir(), "agent"), AgentID: "a1b2c3"},
	} {
		if _, err := memberscmd.Add(o, a); err != nil {
			t.Fatal(err)
		}
	}
	if res, err := f.sw(SwitchOptions{Task: "lets-a1"}); err != nil || !res.OK {
		t.Fatalf("read-only and isolated members must not block: %+v, %v", res, err)
	}
}

func TestSwitch_OperationInProgress(t *testing.T) {
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		t.Run(marker, func(t *testing.T) {
			f := newTeamFixture(t)
			f.onTask(t, "lets-a1")
			if marker == "MERGE_HEAD" {
				// a real clean merge left open: no unmerged path, MERGE_HEAD present
				gitOut(t, f.wt, "switch", "-q", "-c", "side")
				f.write(t, "side.txt", "s\n")
				gitOut(t, f.wt, "add", "side.txt")
				gitOut(t, f.wt, "commit", "-q", "-m", "side")
				gitOut(t, f.wt, "switch", "-q", "-")
				gitOut(t, f.wt, "merge", "-q", "--no-ff", "--no-commit", "side")
			} else {
				p := gitOut(t, f.wt, "rev-parse", "--git-path", marker)
				if !filepath.IsAbs(p) {
					p = filepath.Join(f.wt, p)
				}
				var err error
				if strings.HasPrefix(marker, "rebase-") {
					err = os.MkdirAll(p, 0o755)
				} else {
					err = os.WriteFile(p, []byte(gitOut(t, f.wt, "rev-parse", "HEAD")+"\n"), 0o644)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			before := indexSnap(t, f.wt) + gitOut(t, f.repo, "branch", "--list")
			_, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true, Include: []string{"side.txt"}})
			wantKind(t, err, ExitDirtyWorktree, "operation_in_progress")
			if indexSnap(t, f.wt)+gitOut(t, f.repo, "branch", "--list") != before {
				t.Error("a refused switch touched the index, HEAD or the branches")
			}
		})
	}
}

func TestSwitch_DetachedHead(t *testing.T) {
	f := newTeamFixture(t)
	f.onTask(t, "lets-a1")
	gitOut(t, f.wt, "switch", "-q", "--detach")
	f.write(t, "a.txt", "wip\n")
	before := indexSnap(t, f.wt) + gitOut(t, f.repo, "branch", "--list")
	_, err := f.sw(SwitchOptions{Task: "lets-b2", Park: true})
	wantKind(t, err, ExitDirtyWorktree, "detached_head")
	if indexSnap(t, f.wt)+gitOut(t, f.repo, "branch", "--list") != before {
		t.Error("a refused switch touched the index, HEAD or the branches")
	}
	if _, err := os.Stat(taskstate.Path(f.letsDir, "")); err == nil {
		t.Error("a task-state file with an empty slug was written")
	}
}

// Codex: an ignored local file at a path the target branch tracks is never
// overwritten - the switch fails and the file, the index and HEAD stay as they were.
func TestSwitch_IgnoredFileNotOverwritten(t *testing.T) {
	f := newTeamFixture(t)
	a1 := f.onTask(t, "lets-a1")
	f.write(t, "config.env", "tracked on lets-a1\n")
	gitOut(t, f.wt, "add", "config.env")
	gitOut(t, f.wt, "commit", "-q", "-m", "track config.env")
	f.onTask(t, "lets-b2") // a new branch from origin/main, without config.env
	f.write(t, ".gitignore", "config.env\n")
	gitOut(t, f.wt, "add", ".gitignore")
	gitOut(t, f.wt, "commit", "-q", "-m", "ignore config.env")
	const local = "SECRET=local only\n"
	f.write(t, "config.env", local)
	if st := gitOut(t, f.wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("fixture: the ignored file must not show as dirty: %q", st)
	}
	before := indexSnap(t, f.wt)
	_, err := f.sw(SwitchOptions{Task: "lets-a1"})
	wantKind(t, err, ExitGitFailed, "git_failed")
	if b, _ := os.ReadFile(filepath.Join(f.wt, "config.env")); string(b) != local {
		t.Errorf("the ignored file was overwritten: %q", b)
	}
	if indexSnap(t, f.wt) != before || gitOut(t, f.wt, "branch", "--show-current") == a1 {
		t.Error("a refused switch changed the index, HEAD or the branch")
	}
}
