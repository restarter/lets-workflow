//go:build unix

package worktreecmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// permissiveConvention declares an id pattern wide enough for upper-case and
// multi-hyphen ids, so the dir rules - not the convention - decide each case.
const permissiveConvention = "id: `[A-Za-z0-9_][A-Za-z0-9._-]*`.\n" +
	"branch: `feature/{id}-{slug}`.\n" +
	"worktree-branch: `worktree-{id}-{slug}`."

// dirRepo is a main checkout with the permissive adapter installed and a title file.
func dirRepo(t *testing.T) (repo, title string) {
	t.Helper()
	repo, _ = recordRepo(t)
	rules := filepath.Join(repo, ".claude", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Tracker adapter: beads\n\n## Worktree\n\n" + permissiveConvention + "\n"
	if err := os.WriteFile(filepath.Join(rules, "tracker-beads.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	title = filepath.Join(t.TempDir(), "title.txt")
	if err := os.WriteFile(title, []byte("Fix login"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, title
}

func dirOf(t *testing.T, repo, title, id string) string {
	t.Helper()
	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: id, TitleFile: title, Worktree: true})
	if err != nil || !res.OK {
		t.Fatalf("BranchName(%q): err=%v res=%+v", id, err, res)
	}
	if res.Branch != "worktree-"+id+"-fix-login" {
		t.Errorf("branch = %q: the branch keeps the original id", res.Branch)
	}
	if len(res.Dir) > 64 || !nameRE.MatchString(res.Dir) {
		t.Errorf("dir %q is not a valid worktree name", res.Dir)
	}
	return res.Dir
}

// occupy adds a worktree at <repo>/.worktrees/<dir> on branch and, when task is
// set, writes that branch's task-state file.
func occupy(t *testing.T, repo, dir, branch, task string) {
	t.Helper()
	path := filepath.Join(repo, ".worktrees", dir)
	if branch == "" {
		gitOut(t, repo, "worktree", "add", "-q", "--detach", path)
		return
	}
	gitOut(t, repo, "worktree", "add", "-q", "-b", branch, path)
	if task == "" {
		return
	}
	sessions := filepath.Join(repo, ".lets", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sessions, ".task-"+strings.ReplaceAll(branch, "/", "-"))
	if err := os.WriteFile(file, []byte("task: "+task+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func forceDirHash(t *testing.T, suffix string) {
	t.Helper()
	old := dirHash
	dirHash = func(string) string { return suffix }
	t.Cleanup(func() { dirHash = old })
}

func TestBranchName_DirLowercaseShortUnchanged(t *testing.T) {
	repo, title := dirRepo(t)
	if got := dirOf(t, repo, title, "lets-abc12"); got != "lets-abc12-fix-login" {
		t.Errorf("dir = %q, want lets-abc12-fix-login", got)
	}
}

func TestBranchName_DirCaseCollisionDistinct(t *testing.T) {
	repo, title := dirRepo(t)
	upper, lower := dirOf(t, repo, title, "PROJ-42"), dirOf(t, repo, title, "proj-42")
	if upper == lower {
		t.Fatalf("PROJ-42 and proj-42 share the dir %q", upper)
	}
	if lower != "proj-42-fix-login" || !strings.HasPrefix(upper, "proj-42-fix-login-") {
		t.Errorf("dirs = %q, %q", upper, lower)
	}
}

func TestBranchName_DirPlantedPairDistinct(t *testing.T) {
	repo, title := dirRepo(t)
	a := "task-" + strings.Repeat("a", 65)
	one, two := a+"-3113", a+"-3814"
	// the pair is planted: equal on the first 6 hex, so a 6-hex suffix would collide
	if h1, h2 := dirHash(one), dirHash(two); h1[:6] != "365bd3" || h2[:6] != "365bd3" || h1 == h2 {
		t.Fatalf("planted pair broken: %s %s", h1, h2)
	}
	d1, d2 := dirOf(t, repo, title, one), dirOf(t, repo, title, two)
	if d1 == d2 {
		t.Errorf("both ids render the dir %q", d1)
	}
	if !strings.HasSuffix(d1, "-"+dirHash(one)) || len(dirHash(one)) != 12 {
		t.Errorf("dir %q must end in the 12-hex hash of the id", d1)
	}
}

func TestBranchName_DirInvalidFails(t *testing.T) {
	repo, title := dirRepo(t)
	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "_x", TitleFile: title, Worktree: true})
	if ExitCode(err) != ExitUsage || res.OK || res.Error == nil || res.Error.Kind != "dir_name_invalid" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if res.Dir != "" || res.Branch != "" {
		t.Errorf("a refused render carries no names: %+v", res)
	}
}

func TestBranchName_DirCollisionNamed(t *testing.T) {
	repo, title := dirRepo(t)
	forceDirHash(t, "000000000000")
	dir := dirOf(t, repo, title, "Proj-1")
	occupy(t, repo, dir, "worktree-Proj-1-fix-login", "Proj-1")
	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "PROJ-1", TitleFile: title, Worktree: true})
	if ExitCode(err) != ExitWorktreeExists || res.OK || res.Error == nil || res.Error.Kind != "dir_collision" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if !strings.Contains(res.Error.Message, `"Proj-1"`) || !strings.Contains(res.Error.Message, `"PROJ-1"`) {
		t.Errorf("the collision must name both ids: %q", res.Error.Message)
	}
	// no task-state: the occupant's id is read off its branch
	dir2 := dirOf(t, repo, title, "Proj-2")
	occupy(t, repo, dir2, "feature/Proj-2-x", "")
	res, err = BranchName(context.Background(), repo, BranchNameOptions{Task: "PROJ-2", TitleFile: title, Worktree: true})
	if ExitCode(err) != ExitWorktreeExists || res.Error == nil || res.Error.Kind != "dir_collision" || !strings.Contains(res.Error.Message, `"Proj-2"`) {
		t.Errorf("branch-read occupant: err=%v res=%+v", err, res)
	}
}

// plainWarns runs a render without --worktree and returns its warning steps; the
// plain render creates no dir, so a dir problem never fails it.
func plainWarns(t *testing.T, repo, title, id string) []string {
	t.Helper()
	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: id, TitleFile: title})
	if err != nil || !res.OK || res.Branch != "feature/"+id+"-fix-login" || res.Dir != "" {
		t.Fatalf("plain render of %q: err=%v res=%+v", id, err, res)
	}
	var warns []string
	for _, s := range res.Steps {
		if s.Status == StepWarn {
			warns = append(warns, s.Message)
		}
	}
	return warns
}

func TestBranchName_PlainDirInvalidWarns(t *testing.T) {
	repo, title := dirRepo(t)
	if w := plainWarns(t, repo, title, "_x"); len(w) != 1 || !strings.HasPrefix(w[0], "dir_name_invalid: ") {
		t.Errorf("warnings = %q", w)
	}
}

func TestBranchName_PlainDirCollisionWarns(t *testing.T) {
	repo, title := dirRepo(t)
	forceDirHash(t, "000000000000")
	dir := dirOf(t, repo, title, "Proj-3")
	occupy(t, repo, dir, "worktree-Proj-3-fix-login", "Proj-3")
	if w := plainWarns(t, repo, title, "PROJ-3"); len(w) != 1 || !strings.HasPrefix(w[0], "dir_collision: ") {
		t.Errorf("warnings = %q", w)
	}
}

func TestBranchName_SameIdExistingNotCollision(t *testing.T) {
	repo, title := dirRepo(t)
	dir := dirOf(t, repo, title, "PROJ-7")
	occupy(t, repo, dir, "worktree-PROJ-7-fix-login", "PROJ-7")
	if got := dirOf(t, repo, title, "PROJ-7"); got != dir {
		t.Errorf("dir moved: %q -> %q", dir, got)
	}
	// without a task-state file the id is read off the branch
	dir2 := dirOf(t, repo, title, "PROJ-8")
	occupy(t, repo, dir2, "worktree-PROJ-8-fix-login", "")
	if got := dirOf(t, repo, title, "PROJ-8"); got != dir2 {
		t.Errorf("dir moved: %q -> %q", dir2, got)
	}
}

func TestBranchName_UnreadableOccupantFallsToBackstop(t *testing.T) {
	repo, title := dirRepo(t)
	forceDirHash(t, "000000000000")
	dir := dirOf(t, repo, title, "PROJ-9")
	occupy(t, repo, dir, "", "")                            // detached HEAD
	if got := dirOf(t, repo, title, "Proj-9"); got != dir { // same stem, forced suffix
		t.Fatalf("dir = %q, want %q", got, dir)
	}
	// a branch outside the convention with no task-state: no readable id either
	dir2 := dirOf(t, repo, title, "PROJ-10")
	occupy(t, repo, dir2, "scratch", "")
	if got := dirOf(t, repo, title, "Proj-10"); got != dir2 {
		t.Fatalf("dir = %q, want %q", got, dir2)
	}
	// the create backstop then refuses the occupied path
	res, err := Create(context.Background(), repo, CreateOptions{Name: dir, Branch: "worktree-Proj-9-fix-login", Mode: BranchNewBranch})
	if err == nil || res.OK || res.Error == nil || res.Error.Kind != "worktree_path_exists" {
		t.Errorf("create on the occupied dir: err=%v res=%+v", err, res)
	}
}
