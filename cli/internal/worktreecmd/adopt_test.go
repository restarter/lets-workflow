//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

const beadsConvention = "links: `.beads/.env` (0600).\n" +
	"id: `[a-z][a-z0-9]*-[a-z0-9]+(\\.[0-9]+)?`.\n" +
	"branch: `feature/{id}-{slug}`.\n" +
	"worktree-branch: `worktree-{id}-{slug}`.\n" +
	"accept: `{id}-{slug}`."

// adoptRepo returns an initialized main checkout (.lets, beads adapter, .beads/.env)
// and an external linked worktree on branch.
func adoptRepo(t *testing.T, branch string) (repo, wt string) {
	t.Helper()
	repo = initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	mustWrite(t, filepath.Join(repo, ".beads", ".env"), "X=1", 0o600)
	installAdapter(t, repo, "beads", beadsConvention)
	wt = filepath.Join(realTempDir(t), filepath.Base(branch))
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", branch, wt)
	return repo, wt
}

func taskFile(repo, branch string) string {
	return filepath.Join(repo, ".lets", "sessions", ".task-"+strings.ReplaceAll(branch, "/", "-"))
}

func readTask(t *testing.T, repo, branch string) string {
	t.Helper()
	data, err := os.ReadFile(taskFile(repo, branch))
	if err != nil {
		t.Fatalf("task file: %v", err)
	}
	return string(data)
}

func TestAdopt_MovesCacheOnlyLetsAndLinks(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-1")
	mustWrite(t, filepath.Join(wt, ".lets", "cache", "usage"), "u", 0o644)
	mustWrite(t, filepath.Join(wt, ".lets", "cache", "task-status"), "s", 0o644)
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("Adopt: %v", err)
	}
	if res.MovedAside != filepath.Join(wt, ".lets.pre-adopt") {
		t.Errorf("moved_aside = %q", res.MovedAside)
	}
	if data, _ := os.ReadFile(filepath.Join(res.MovedAside, "cache", "usage")); string(data) != "u" {
		t.Error("moved contents lost")
	}
	if !resolvesTo(t, filepath.Join(wt, ".lets"), filepath.Join(repo, ".lets")) {
		t.Error(".lets not linked")
	}
	if target, _ := os.Readlink(filepath.Join(wt, ".beads", ".env")); !filepath.IsAbs(target) {
		t.Errorf("an external worktree gets an absolute store link, got %q", target)
	}
	if res.MainRoot != repo {
		t.Errorf("main_root = %q, want %q", res.MainRoot, repo)
	}
	if out, _ := exec.Command("git", "-C", wt, "status", "--porcelain").Output(); len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("worktree must stay clean after adopt:\n%s", out)
	}
	// with .lets.pre-adopt taken, the next move goes to -2
	_ = os.Remove(filepath.Join(wt, ".lets"))
	mustWrite(t, filepath.Join(wt, ".lets", "cache", "usage"), "u2", 0o644)
	if res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true}); err != nil || res.MovedAside != filepath.Join(wt, ".lets.pre-adopt-2") {
		t.Errorf("second move: %v %q", err, res.MovedAside)
	}
}

func TestAdopt_RealLetsDataRefused(t *testing.T) {
	_, wt := adoptRepo(t, "ext-2")
	mustWrite(t, filepath.Join(wt, ".lets", "sessions", "x.md"), "notes", 0o644)
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitLetsDirConflict || res.OK || !strings.Contains(res.Error.Remediation, "mv ") {
		t.Fatalf("err=%v res=%+v", err, res.Error)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, ".lets", "sessions", "x.md")); string(data) != "notes" {
		t.Error("data touched")
	}
}

func TestAdopt_AdapterStates(t *testing.T) {
	// none adapter links nothing
	repo, wt := adoptRepo(t, "ext-3")
	mustWrite(t, filepath.Join(repo, ".lets", ".env"), "LETS_TRACKER=none\n", 0o644)
	installAdapter(t, repo, "none", "links: nothing.\nid: nothing.")
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{})
	if err != nil || len(res.StoreLinks) != 0 || res.Task != nil {
		t.Errorf("none: err=%v links=%+v task=%+v", err, res.StoreLinks, res.Task)
	}
	if _, err := os.Lstat(filepath.Join(wt, ".beads", ".env")); !os.IsNotExist(err) {
		t.Error("none adapter linked .beads/.env")
	}

	// an adapter without ## Worktree: warn store_links_undeclared, ok
	repo2, wt2 := adoptRepo(t, "ext-4")
	mustWrite(t, filepath.Join(repo2, ".claude", "rules", "tracker-beads.md"), "# old adapter\n", 0o644)
	res, err = worktreecmd.Adopt(context.Background(), wt2, worktreecmd.AdoptOptions{})
	if err != nil || !res.OK || !stepsContain(res.Steps, "store_links_undeclared") {
		t.Errorf("undeclared: err=%v steps=%+v", err, res.Steps)
	}
}

func stepsContain(steps []worktreecmd.Step, sub string) bool {
	for _, s := range steps {
		if strings.Contains(s.Message, sub) {
			return true
		}
	}
	return false
}

func TestAdopt_ConcurrentAdoptsSerialize(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-5")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("adopt %d: %v", i, err)
		}
	}
	if !resolvesTo(t, filepath.Join(wt, ".lets"), filepath.Join(repo, ".lets")) {
		t.Error("one symlink expected")
	}
}

func TestAdopt_MainCheckoutRefused(t *testing.T) {
	repo, _ := adoptRepo(t, "ext-6")
	_, err := worktreecmd.Adopt(context.Background(), repo, worktreecmd.AdoptOptions{})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitNotLinkedWorktree {
		t.Errorf("err = %v, want exit 23", err)
	}
}

func TestAdoptTask_ExplicitIdempotentAndConflicts(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-7")
	ctx := context.Background()
	if _, err := worktreecmd.Adopt(ctx, wt, worktreecmd.AdoptOptions{Task: "lets-abc"}); err != nil {
		t.Fatal(err)
	}
	first := readTask(t, repo, "ext-7")
	if !strings.Contains(first, "task: lets-abc") || !strings.Contains(first, "start: ") {
		t.Fatalf("task file: %q", first)
	}
	runIn(t, wt, "git", "commit", "-q", "--allow-empty", "-m", "work")
	if _, err := worktreecmd.Adopt(ctx, wt, worktreecmd.AdoptOptions{Task: "lets-abc"}); err != nil {
		t.Fatal(err)
	}
	if got := readTask(t, repo, "ext-7"); got != first {
		t.Errorf("rerun must keep start:\n%s\nwant\n%s", got, first)
	}
	// a different explicit id over an explicit file: exit 25
	if _, err := worktreecmd.Adopt(ctx, wt, worktreecmd.AdoptOptions{Task: "lets-other"}); worktreecmd.ExitCode(err) != worktreecmd.ExitTaskFileConflict {
		t.Errorf("err = %v, want exit 25", err)
	}
	// an invalid id: exit 2
	if _, err := worktreecmd.Adopt(ctx, wt, worktreecmd.AdoptOptions{Task: "-rf"}); worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("err = %v, want exit 2", err)
	}
}

func TestAdoptTask_DerivedFromBranchAndSuperseded(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-ip06f-peer-messaging-orca")
	ctx := context.Background()
	mustWrite(t, taskFile(repo, "lets-ip06f-peer-messaging-orca"), "orc: MAIN-LETS\n", 0o644)
	res, err := worktreecmd.Adopt(ctx, wt, worktreecmd.AdoptOptions{})
	if err != nil || res.Task == nil || res.Task.ID != "lets-ip06f" || res.Task.Origin != "branch" {
		t.Fatalf("err=%v task=%+v", err, res.Task)
	}
	got := readTask(t, repo, "lets-ip06f-peer-messaging-orca")
	if !strings.Contains(got, "task: lets-ip06f") || !strings.Contains(got, "origin: branch") || !strings.Contains(got, "orc: MAIN-LETS") {
		t.Errorf("task file:\n%s", got)
	}
	// an explicit id supersedes the guess and clears origin:
	res, err = worktreecmd.Adopt(ctx, wt, worktreecmd.AdoptOptions{Task: "lets-real"})
	if err != nil || res.Task.Origin != "" {
		t.Fatalf("supersede: err=%v task=%+v", err, res.Task)
	}
	got = readTask(t, repo, "lets-ip06f-peer-messaging-orca")
	if !strings.Contains(got, "task: lets-real") || strings.Contains(got, "origin:") || !strings.Contains(got, "orc: MAIN-LETS") {
		t.Errorf("superseded file:\n%s", got)
	}
}

func TestAdoptTask_BoardConventionAndUndeclared(t *testing.T) {
	repo, wt := adoptRepo(t, "feature/PWA-45122-fix-login")
	mustWrite(t, filepath.Join(repo, ".claude", "rules", "tracker-beads.board.md"), "# board\n\n## Worktree\n\nid: `PWA-[0-9]+`.\nbranch: `feature/{id}-{slug}`.\n", 0o644)
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{})
	if err != nil || res.Task == nil || res.Task.ID != "PWA-45122" || res.Task.Origin != "" {
		t.Fatalf("board: err=%v task=%+v", err, res.Task)
	}

	repo2, wt2 := adoptRepo(t, "lets-abc-something")
	installAdapter(t, repo2, "beads", "links: `.beads/.env` (0600).") // no id: - undeclared
	res, err = worktreecmd.Adopt(context.Background(), wt2, worktreecmd.AdoptOptions{})
	if err != nil || res.Task != nil {
		t.Errorf("undeclared: err=%v task=%+v", err, res.Task)
	}
	if _, err := os.Stat(taskFile(repo2, "lets-abc-something")); !os.IsNotExist(err) {
		t.Error("undeclared convention must write no task file")
	}
}

func TestAdoptTask_DetachedHeadWritesNothing(t *testing.T) {
	repo, wt := adoptRepo(t, "lets-abc-detached")
	runIn(t, wt, "git", "checkout", "-q", "--detach", "HEAD")
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-abc"})
	if err != nil || !stepsContain(res.Steps, "detached_head") {
		t.Fatalf("err=%v steps=%+v", err, res.Steps)
	}
	matches, _ := filepath.Glob(filepath.Join(repo, ".lets", "sessions", ".task-*"))
	if len(matches) != 0 {
		t.Errorf("detached HEAD wrote %v", matches)
	}
}

// teamAdoptRepo is adoptRepo with the worktree directory named apart from its
// branch. tracker "fake" runs the shared partial-adapter fixture from a temporary
// plugin root, with a board whose id grammar accepts `_` - a custom grammar under
// which even `team_backend-api` parses as an id, so the team_ prefix must win first.
func teamAdoptRepo(t *testing.T, tracker, branch, dir string) (repo, wt, pluginRoot string) {
	t.Helper()
	repo = initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	switch tracker {
	case "beads":
		mustWrite(t, filepath.Join(repo, ".beads", ".env"), "X=1", 0o600)
		installAdapter(t, repo, "beads", beadsConvention)
	case "fake":
		fake, err := os.ReadFile(filepath.Join("..", "initcmd", "testdata", "tracker-fake.md"))
		if err != nil {
			t.Fatal(err)
		}
		pluginRoot = realTempDir(t)
		mustWrite(t, filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), "{}", 0o644)
		mustWrite(t, filepath.Join(pluginRoot, "rules", "tracker-fake.md"), string(fake), 0o644)
		mustWrite(t, filepath.Join(repo, ".lets", ".env"), "LETS_TRACKER=fake\n", 0o644)
		mustWrite(t, filepath.Join(repo, ".fake", "token.json"), "{}", 0o600)
		mustWrite(t, filepath.Join(repo, ".claude", "rules", "tracker-fake.board.md"),
			"# board\n\n## Worktree\n\nid: `[a-z_-]+`.\naccept: `{id}`.\n", 0o644)
	}
	wt = filepath.Join(realTempDir(t), dir)
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", branch, wt)
	return repo, wt, pluginRoot
}

func requireNoTaskFile(t *testing.T, label, repo, branch string) {
	t.Helper()
	if data, err := os.ReadFile(taskFile(repo, branch)); !os.IsNotExist(err) {
		t.Errorf("%s: a team worktree must write no task-state file, got err=%v:\n%s", label, err, data)
	}
}

// A team worktree (area backend-api) derives no task id from a team_ branch or
// directory, under beads and under the fake adapter's custom grammar. The control
// row proves the hand-named `snake-backend-api` DOES derive an id without the
// prefix (beads: id snake-backend + slug api - the adopt trap), so each mixed row
// would record a task if the prefix check were missing; under fake the Go-created
// row would too.
func TestAdopt_TeamPrefixDerivesNoTask(t *testing.T) {
	for tracker, controlID := range map[string]string{"beads": "snake-backend", "fake": "snake-backend-api"} {
		_, wt, pr := teamAdoptRepo(t, tracker, "snake-backend-api", "snake-backend-api")
		res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{PluginRoot: pr})
		if err != nil || res.Task == nil || res.Task.ID != controlID {
			t.Fatalf("%s control: want id %s, err=%v task=%+v steps=%+v", tracker, controlID, err, res.Task, res.Steps)
		}
		for _, c := range []struct{ branch, dir string }{
			{"team_backend-api", "team_backend-api"},  // Go-created: dir and branch
			{"snake-backend-api", "team_backend-api"}, // dir only
			{"team_backend-api", "snake-backend-api"}, // branch only
		} {
			label := tracker + " " + c.branch + " in " + c.dir
			repo, wt, pr := teamAdoptRepo(t, tracker, c.branch, c.dir)
			res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{PluginRoot: pr})
			if err != nil || !res.OK || res.Task != nil || !stepsContain(res.Steps, "team_worktree") {
				t.Errorf("%s: err=%v task=%+v steps=%+v", label, err, res.Task, res.Steps)
			}
			requireNoTaskFile(t, label, repo, c.branch)
		}
	}
}

// A team file that claims the worktree derives no id; a team file that claims it
// but is not clean (stale git_dir, name mismatch) derives none either and warns
// naming the file - fail-safe, never a fall-through to the convention. An explicit
// --task still records: the skip is on derivation only.
func TestAdopt_TeamFileErrorDerivesNoTask(t *testing.T) {
	for _, c := range []struct{ label, team, file, gitDir, step string }{
		{"hit", "snake", "snake.md", "", "team_worktree"},
		{"stale git_dir", "snake", "snake.md", "/nowhere/.git/worktrees/gone", "team_file_error"},
		{"name mismatch", "frog", "snake.md", "", "team_file_error"},
	} {
		repo, wt, _ := teamAdoptRepo(t, "beads", "snake-backend-api", "snake-backend-api")
		gitDir := c.gitDir
		if gitDir == "" {
			gitDir = strings.TrimSpace(gitOutput(t, wt, "rev-parse", "--absolute-git-dir"))
		}
		mustWrite(t, filepath.Join(repo, ".lets", "teams", c.file),
			"---\nteam: \""+c.team+"\"\nworktree: \""+wt+"\"\ngit_dir: \""+gitDir+"\"\n---\n", 0o644)
		res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{})
		if err != nil || !res.OK || res.Task != nil || !stepsContain(res.Steps, c.step) {
			t.Errorf("%s: err=%v task=%+v steps=%+v", c.label, err, res.Task, res.Steps)
		}
		if c.step == "team_file_error" && !stepsContain(res.Steps, c.file) {
			t.Errorf("%s: the warn step must name %s: %+v", c.label, c.file, res.Steps)
		}
		requireNoTaskFile(t, c.label, repo, "snake-backend-api")
		if c.label == "hit" {
			res, err = worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{Task: "lets-abc"})
			if err != nil || res.Task == nil || res.Task.ID != "lets-abc" {
				t.Errorf("explicit --task in a team worktree: err=%v task=%+v", err, res.Task)
			}
		}
	}
}

// One broken team file that claims another worktree does not affect this one: the
// convention still derives the id, and the file is named in a warn step.
func TestAdopt_UnrelatedMalformedIgnored(t *testing.T) {
	repo, wt, _ := teamAdoptRepo(t, "beads", "snake-backend-api", "snake-backend-api")
	mustWrite(t, filepath.Join(repo, ".lets", "teams", "broken.md"), "---\nteam: \"frog\nworktree: /elsewhere\n", 0o644)
	mustWrite(t, filepath.Join(repo, ".lets", "teams", "notes.md"), "# notes, not a team file\n", 0o644)
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{})
	if err != nil || res.Task == nil || res.Task.ID != "snake-backend" {
		t.Fatalf("err=%v task=%+v steps=%+v", err, res.Task, res.Steps)
	}
	if !stepsContain(res.Steps, "broken.md") || stepsContain(res.Steps, "notes.md") || stepsContain(res.Steps, "team_") {
		t.Errorf("steps=%+v, want one warn naming broken.md and no team skip", res.Steps)
	}
	if got := readTask(t, repo, "snake-backend-api"); !strings.Contains(got, "task: snake-backend") {
		t.Errorf("task file:\n%s", got)
	}
}
