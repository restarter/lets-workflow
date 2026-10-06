//go:build unix

package worktreecmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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
	isolateEnv(t)
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

// isolateEnv keeps the developer's ~/.lets/.env and $CLAUDE_PLUGIN_ROOT out of a
// test that judges the tracker convention.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
}

// writeTracker selects tracker in root's .lets/.env and writes its adapter and
// board under root/.claude/rules ("" = no file).
func writeTracker(t *testing.T, root, tracker, adapter, board string) {
	t.Helper()
	rules := filepath.Join(root, ".claude", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".lets", ".env"), []byte("LETS_TRACKER="+tracker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"tracker-" + tracker + ".md": adapter, "tracker-" + tracker + ".board.md": board} {
		p := filepath.Join(rules, name)
		if body == "" {
			_ = os.Remove(p)
			continue
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const planfixAdapter = "# adapter\n\n## Capabilities\n\nnothing here.\n"

func warnsWith(steps []Step, prefix string) int {
	n := 0
	for _, s := range steps {
		if s.Status == StepWarn && strings.HasPrefix(s.Message, prefix) {
			n++
		}
	}
	return n
}

// titleFile writes "Fix login" to a fresh title file.
func titleFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "title.txt")
	if err := os.WriteFile(p, []byte("Fix login"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestBranchName_BoardTemplateWithoutID is lets-puvic's acceptance test: a board's
// branch: renders although nothing declares id:, and the caller is told so.
func TestBranchName_BoardTemplateWithoutID(t *testing.T) {
	isolateEnv(t)
	repo, _ := recordRepo(t)
	writeTracker(t, repo, "planfix-mcp", planfixAdapter, "# board\n\n## Worktree\n\nbranch: `feature/pwa-{id}`.\n")
	title := titleFile(t)

	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "49514", TitleFile: title})
	if err != nil || !res.OK {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if res.Branch != "feature/pwa-49514" || res.Source != "board" || res.Template != "feature/pwa-{id}" {
		t.Errorf("branch=%q source=%q template=%q", res.Branch, res.Source, res.Template)
	}
	if !slices.Contains(res.Reasons, "convention_templates_without_id") {
		t.Errorf("reasons = %v", res.Reasons)
	}
	if n := warnsWith(res.Steps, "convention_templates_without_id: "); n != 1 {
		t.Errorf("want one templates-without-id warn, got %d: %+v", n, res.Steps)
	}

	res, err = BranchName(context.Background(), repo, BranchNameOptions{Task: "49514", TitleFile: title, Worktree: true})
	if err != nil || !res.OK {
		t.Fatalf("worktree: err=%v res=%+v", err, res)
	}
	if res.Branch != "worktree-49514-fix-login" || res.Source != "default" {
		t.Errorf("worktree: branch=%q source=%q", res.Branch, res.Source)
	}
	if n := warnsWith(res.Steps, "convention_templates_without_id: "); n != 1 {
		t.Errorf("worktree: want one templates-without-id warn, got %d: %+v", n, res.Steps)
	}
}

func TestBranchName_InvalidDeclarationRefuses(t *testing.T) {
	isolateEnv(t)
	repo, _ := recordRepo(t)
	writeTracker(t, repo, "planfix-mcp", planfixAdapter, "# board\n\n## Worktree\n\nbranch: `feature/pwa-{id}`\n")

	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "49514", TitleFile: titleFile(t)})
	wantKind(t, err, ExitUsage, "convention_declaration_invalid")
	if res.OK || res.Error == nil || !strings.Contains(res.Error.Message, ".claude/rules/tracker-planfix-mcp.board.md") || res.Branch != "" {
		t.Errorf("res = %+v err = %+v", res, res.Error)
	}
	if n := warnsWith(res.Steps, "convention_declaration_invalid: "); n != 0 {
		t.Errorf("the refusal is the one voice, got %d warn steps: %+v", n, res.Steps)
	}
}

func TestBranchName_TrackerNameInvalidWarns(t *testing.T) {
	isolateEnv(t)
	repo, _ := recordRepo(t)
	writeTracker(t, repo, "Planfix-MCP", "", "")

	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "49514", TitleFile: titleFile(t)})
	if err != nil || !res.OK || res.Branch != "feature/49514-fix-login" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if n := warnsWith(res.Steps, "tracker_name_invalid: "); n != 1 {
		t.Errorf("want one tracker_name_invalid warn, got %d: %+v", n, res.Steps)
	}
}

func TestBranchName_AdapterMissingWarns(t *testing.T) {
	isolateEnv(t)
	repo, _ := recordRepo(t)
	writeTracker(t, repo, "planfix-mcp", "", "")

	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "49514", TitleFile: titleFile(t)})
	if err != nil || !res.OK || res.Branch != "feature/49514-fix-login" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if n := warnsWith(res.Steps, "adapter_missing: "); n != 1 {
		t.Errorf("want one adapter_missing warn, got %d: %+v", n, res.Steps)
	}
	for _, s := range res.Steps {
		if strings.HasPrefix(s.Message, "adapter_missing: ") && !strings.Contains(s.Message, ".claude/rules/tracker-planfix-mcp.md") {
			t.Errorf("warn must name the adapter file: %q", s.Message)
		}
	}
}

func TestBranchName_DeclaredConventionIsQuiet(t *testing.T) {
	repo, title := dirRepo(t)
	res, err := BranchName(context.Background(), repo, BranchNameOptions{Task: "lets-a1", TitleFile: title})
	if err != nil || !res.OK {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if n := warnsWith(res.Steps, ""); n != 0 {
		t.Errorf("a declared convention must not warn: %+v", res.Steps)
	}
}

func TestBranchName_BoardOnlyInWorktreeWarns(t *testing.T) {
	isolateEnv(t)
	repo, _ := recordRepo(t)
	writeTracker(t, repo, "planfix-mcp", planfixAdapter, "")
	parent, _ := filepath.EvalSymlinks(t.TempDir())
	wt := filepath.Join(parent, "wt")
	gitOut(t, repo, "worktree", "add", "-q", "-b", "wt-branch", wt)
	rules := filepath.Join(wt, ".claude", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "tracker-planfix-mcp.board.md"), []byte("# board\n\n## Worktree\n\nbranch: `feature/pwa-{id}`.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := BranchName(context.Background(), wt, BranchNameOptions{Task: "49514", TitleFile: titleFile(t)})
	if err != nil || !res.OK || res.Branch != "feature/49514-fix-login" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if n := warnsWith(res.Steps, "board_not_in_main_checkout: "); n != 1 {
		t.Errorf("want one board_not_in_main_checkout warn, got %d: %+v", n, res.Steps)
	}
}

// TestSweep_TemplatesWithoutIDWarns: sweep names a template without id: only when
// its legacy prefixes cannot see the branches it creates.
func TestSweep_TemplatesWithoutIDWarns(t *testing.T) {
	isolateEnv(t)
	repo, _ := recordRepo(t)
	ctx := context.Background()
	const prefix = "convention_templates_without_id: "

	writeTracker(t, repo, "planfix-mcp", planfixAdapter, "## Worktree\n\nbranch: `task/{id}`.\n")
	if c := loadConvention(repo); c.Branch != "task/{id}" || c.Declared {
		t.Fatalf("fixture must load the board template undeclared: %+v", c)
	}
	res, _ := Sweep(ctx, repo, false)
	if n := warnsWith(res.Steps, prefix); n != 1 {
		t.Errorf("task/{id}: want one warn, got %d: %+v", n, res.Steps)
	}

	writeTracker(t, repo, "planfix-mcp", planfixAdapter, "## Worktree\n\nbranch: `feature/pwa-{id}`.\n")
	if c := loadConvention(repo); c.Branch != "feature/pwa-{id}" {
		t.Fatalf("fixture must load the board template: %+v", c)
	}
	res, _ = Sweep(ctx, repo, false)
	if n := warnsWith(res.Steps, prefix); n != 0 {
		t.Errorf("feature/pwa-{id} is covered by feature/: %+v", res.Steps)
	}

	writeTracker(t, repo, "planfix-mcp", "# adapter\n\n## Worktree\n\nlinks: nothing.\n", "")
	res, _ = Sweep(ctx, repo, false)
	if n := warnsWith(res.Steps, prefix); n != 0 {
		t.Errorf("no template declared: %+v", res.Steps)
	}
}
