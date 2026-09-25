package teamfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// realDir is a t.TempDir with symlinks resolved (macOS /var -> /private/var), so a
// test that wants a symlink makes one on purpose.
func realDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// teamBody is a hand-made team file: plain values, as an owner might write one.
func teamBody(team, worktree, gitDir string) string {
	s := "---\nteam: " + team + "\nworktree: " + worktree + "\n"
	if gitDir != "" {
		s += "git_dir: " + gitDir + "\n"
	}
	return s + "---\n\n# Team file\n"
}

func allValues(worktree, gitDir string) teamfile.Values {
	return teamfile.Values{
		"team": "snake", "area": "backend-api", "repo": "lets-workflow", "worktree": worktree,
		"git_dir": gitDir, "base": "origin/main", "created": "2026-09-26T10:00:00Z",
		"lead_name": "snake-lead", "agent_command": "claude", "orca_agent": "claude-agent-teams",
	}
}

func TestRender_QuotesValues(t *testing.T) {
	tmpl := []byte("---\nteam: {{team}}\narea: {{area}}\n---\nbody\n")
	out, err := teamfile.Render(tmpl, teamfile.Values{"team": "snake", "area": `a: "b" # c \ d`})
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nteam: \"snake\"\narea: \"a: \\\"b\\\" # c \\\\ d\"\n---\nbody\n"
	if string(out) != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}

	// Round trip: a worktree path with YAML-special characters comes back intact.
	root := realDir(t)
	wt := filepath.Join(root, `wt "x": #y 'z'`)
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = teamfile.Render([]byte("---\nteam: {{team}}\nworktree: {{worktree}}\n---\n"), teamfile.Values{"team": "snake", "worktree": wt})
	if err != nil {
		t.Fatal(err)
	}
	teams := filepath.Join(root, "teams")
	writeFile(t, filepath.Join(teams, "snake.md"), string(out))
	name, ok, warns, err := teamfile.FindByWorktree(teams, wt, "")
	if err != nil || !ok || name != "snake" || len(warns) != 0 {
		t.Errorf("round trip: name=%q ok=%v warns=%v err=%v", name, ok, warns, err)
	}
}

func TestRender_RefusesLeftoverPlaceholder(t *testing.T) {
	for label, tc := range map[string]struct {
		tmpl string
		v    teamfile.Values
	}{
		"unknown key":      {"team: {{team}}\narea: {{area}}\n", teamfile.Values{"team": "snake"}},
		"value carries {{": {"team: {{team}}\n", teamfile.Values{"team": "{{area}}"}},
		"malformed":        {"team: {{ team }}\n", teamfile.Values{"team": "snake"}},
	} {
		out, err := teamfile.Render([]byte(tc.tmpl), tc.v)
		if !errors.Is(err, teamfile.ErrPlaceholder) || out != nil {
			t.Errorf("%s: out=%q err=%v, want ErrPlaceholder and no output", label, out, err)
		}
	}
}

func TestRender_RefusesControlChars(t *testing.T) {
	for _, v := range []string{"a\nb", "a\rb", "a\x1bb", "a\x00b", "a\x7fb", "a\u0085b", "a\tb"} {
		out, err := teamfile.Render([]byte("team: {{team}}\n"), teamfile.Values{"team": v})
		if !errors.Is(err, teamfile.ErrControlChar) || out != nil {
			t.Errorf("%q: out=%q err=%v, want ErrControlChar", v, out, err)
		}
	}
}

// The shipped template renders with exactly the documented keys, keeps its
// Workspace section, and the result is found by FindByWorktree.
func TestRender_Template(t *testing.T) {
	tmpl, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "templates", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	root := realDir(t)
	wt := filepath.Join(root, "team_snake")
	gitDir := filepath.Join(root, "repo", ".git", "worktrees", "team_snake")
	for _, d := range []string{wt, gitDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	v := allValues(wt, gitDir)
	out, err := teamfile.Render(tmpl, v)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"team: \"snake\"\n", "lead_name: \"snake-lead\"\n", "orca_agent: \"claude-agent-teams\"\n",
		"## 2. Workspace", "`DOCKER_PREFIX`", "`COMPOSE_PROJECT_NAME`", "<first>-<last>", "`.lets/hooks/team-setup`",
		"Resume artefacts:", "Continuity = files only",
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("rendered template lacks %q", s)
		}
	}
	// Every key is required: dropping any one leaves a placeholder.
	for k := range v {
		less := teamfile.Values{}
		for k2, v2 := range v {
			if k2 != k {
				less[k2] = v2
			}
		}
		if _, err := teamfile.Render(tmpl, less); !errors.Is(err, teamfile.ErrPlaceholder) {
			t.Errorf("template without %q: err=%v, want ErrPlaceholder", k, err)
		}
	}
	teams := filepath.Join(root, "teams")
	writeFile(t, filepath.Join(teams, "snake.md"), string(out))
	name, ok, warns, err := teamfile.FindByWorktree(teams, wt, gitDir)
	if err != nil || !ok || name != "snake" || len(warns) != 0 {
		t.Errorf("rendered template: name=%q ok=%v warns=%v err=%v", name, ok, warns, err)
	}
}

func TestValidName(t *testing.T) {
	for _, s := range []string{"snake", "frog-2", "a", "0", strings.Repeat("a", 40), "backend-api", "runner", "run"} {
		if !teamfile.ValidName(s) {
			t.Errorf("%q must be valid", s)
		}
	}
	for _, s := range []string{"", "Snake", "team_snake", "a b", "a/b", "../x", "a.b", strings.Repeat("a", 41), "run-73671c", "run-", "x\n"} {
		if teamfile.ValidName(s) {
			t.Errorf("%q must be invalid", s)
		}
	}
}

func TestFindByWorktree_Symlink(t *testing.T) {
	root := realDir(t)
	wt := filepath.Join(root, "team_snake")
	link := filepath.Join(root, "link")
	gitDir := filepath.Join(root, "gitdir")
	gitLink := filepath.Join(root, "gitlink")
	for _, d := range []string{wt, gitDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(wt, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitDir, gitLink); err != nil {
		t.Fatal(err)
	}
	teams := filepath.Join(root, "teams")

	// The file records the symlink; the caller asks with the real path.
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", link, gitLink))
	if name, ok, _, err := teamfile.FindByWorktree(teams, wt, gitDir); err != nil || !ok || name != "snake" {
		t.Errorf("file=link, query=real: name=%q ok=%v err=%v", name, ok, err)
	}
	// The file records the real path; the caller asks through the symlink.
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", wt, gitDir))
	if name, ok, _, err := teamfile.FindByWorktree(teams, link, gitLink); err != nil || !ok || name != "snake" {
		t.Errorf("file=real, query=link: name=%q ok=%v err=%v", name, ok, err)
	}
	// A relative caller git dir (`.git` in a main checkout) is taken from toplevel.
	if err := os.MkdirAll(filepath.Join(wt, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", wt, filepath.Join(wt, ".git")))
	if name, ok, _, err := teamfile.FindByWorktree(teams, link, ".git"); err != nil || !ok || name != "snake" {
		t.Errorf("relative git dir: name=%q ok=%v err=%v", name, ok, err)
	}
}

func TestFindByWorktree_Ambiguous(t *testing.T) {
	root := realDir(t)
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	teams := filepath.Join(root, "teams")
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", wt, ""))
	writeFile(t, filepath.Join(teams, "frog.md"), teamBody("frog", wt, ""))
	name, ok, _, err := teamfile.FindByWorktree(teams, wt, "")
	if !errors.Is(err, teamfile.ErrAmbiguousTeam) || ok || name != "" {
		t.Fatalf("name=%q ok=%v err=%v, want ErrAmbiguousTeam", name, ok, err)
	}
	for _, f := range []string{"snake.md", "frog.md"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("error %q does not name %s", err, f)
		}
	}
	// A malformed second claimer is still a second claimer.
	writeFile(t, filepath.Join(teams, "frog.md"), teamBody("toad", wt, ""))
	if _, _, _, err := teamfile.FindByWorktree(teams, wt, ""); !errors.Is(err, teamfile.ErrAmbiguousTeam) {
		t.Errorf("malformed second claimer: err=%v, want ErrAmbiguousTeam", err)
	}
}

func TestFindByWorktree_StaleGitDir(t *testing.T) {
	root := realDir(t)
	wt := filepath.Join(root, "wt")
	oldGit := filepath.Join(root, "git-old")
	newGit := filepath.Join(root, "git-new")
	for _, d := range []string{wt, oldGit, newGit} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	teams := filepath.Join(root, "teams")
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", wt, oldGit))
	name, ok, _, err := teamfile.FindByWorktree(teams, wt, newGit)
	if !errors.Is(err, teamfile.ErrStaleTeam) || ok || name != "" {
		t.Fatalf("name=%q ok=%v err=%v, want ErrStaleTeam", name, ok, err)
	}
	if !strings.Contains(err.Error(), "snake.md") {
		t.Errorf("error %q does not name the file", err)
	}
	// An unknown caller git dir cannot prove the path was not reused.
	if _, _, _, err := teamfile.FindByWorktree(teams, wt, ""); !errors.Is(err, teamfile.ErrStaleTeam) {
		t.Errorf("empty caller git dir: err=%v, want ErrStaleTeam", err)
	}
	// A hand-made file without git_dir skips the check.
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", wt, ""))
	if name, ok, _, err := teamfile.FindByWorktree(teams, wt, newGit); err != nil || !ok || name != "snake" {
		t.Errorf("no git_dir: name=%q ok=%v err=%v", name, ok, err)
	}
}

func TestFindByWorktree_NameMismatch(t *testing.T) {
	root := realDir(t)
	wt := filepath.Join(root, "wt")
	other := filepath.Join(root, "other")
	for _, d := range []string{wt, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	teams := filepath.Join(root, "teams")

	// Claims this worktree: fail-safe error naming the file, no team.
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("frog", wt, ""))
	name, ok, _, err := teamfile.FindByWorktree(teams, wt, "")
	if !errors.Is(err, teamfile.ErrMalformedTeam) || ok || name != "" || !strings.Contains(err.Error(), "snake.md") {
		t.Errorf("claiming mismatch: name=%q ok=%v err=%v, want ErrMalformedTeam naming snake.md", name, ok, err)
	}
	// An invalid filename that claims this worktree is malformed too.
	if err := os.Remove(filepath.Join(teams, "snake.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teams, "run-1.md"), teamBody("run-1", wt, ""))
	if _, _, _, err := teamfile.FindByWorktree(teams, wt, ""); !errors.Is(err, teamfile.ErrMalformedTeam) {
		t.Errorf("reserved name claiming: err=%v, want ErrMalformedTeam", err)
	}
	if err := os.Remove(filepath.Join(teams, "run-1.md")); err != nil {
		t.Fatal(err)
	}

	// Names another worktree: skipped with a warning; this worktree's team is found.
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("frog", other, ""))
	writeFile(t, filepath.Join(teams, "lion.md"), teamBody("lion", wt, ""))
	name, ok, warns, err := teamfile.FindByWorktree(teams, wt, "")
	if err != nil || !ok || name != "lion" {
		t.Errorf("other worktree mismatch: name=%q ok=%v err=%v, want lion", name, ok, err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "snake.md") {
		t.Errorf("warnings=%v, want one naming snake.md", warns)
	}
}

// One broken file in the shared .lets/teams cannot affect another worktree; a
// markdown file without frontmatter (notes, briefs) is not a team file at all.
func TestFindByWorktree_UnrelatedMalformedSkipped(t *testing.T) {
	root := realDir(t)
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	teams := filepath.Join(root, "teams")
	writeFile(t, filepath.Join(teams, "notes.md"), "# notes\n\nteam: snake\nworktree: "+wt+"\n")
	writeFile(t, filepath.Join(teams, "open.md"), "---\nteam: open\nworktree: /elsewhere\n")
	writeFile(t, filepath.Join(teams, "quote.md"), "---\nteam: \"quote\nworktree: /elsewhere\n---\n")
	writeFile(t, filepath.Join(teams, "dup.md"), "---\nteam: dup\nworktree: /a\nworktree: /b\n---\n")
	writeFile(t, filepath.Join(teams, "snake.md.tmp-abc"), teamBody("snake", wt, ""))
	writeFile(t, filepath.Join(teams, ".hidden.md"), teamBody("hidden", wt, ""))

	name, ok, warns, err := teamfile.FindByWorktree(teams, wt, "")
	if err != nil || ok || name != "" {
		t.Errorf("name=%q ok=%v err=%v, want no team and no error", name, ok, err)
	}
	joined := strings.Join(warns, "\n")
	for _, f := range []string{"open.md", "quote.md", "dup.md"} {
		if !strings.Contains(joined, f) {
			t.Errorf("warnings %v do not name %s", warns, f)
		}
	}
	if len(warns) != 3 {
		t.Errorf("warnings=%v, want exactly 3 (notes, temp and dot files are not team files)", warns)
	}

	// The good file still wins next to them.
	writeFile(t, filepath.Join(teams, "snake.md"), teamBody("snake", wt, ""))
	if name, ok, _, err := teamfile.FindByWorktree(teams, wt, ""); err != nil || !ok || name != "snake" {
		t.Errorf("with a good file: name=%q ok=%v err=%v", name, ok, err)
	}
}

func TestFindByWorktree_MissingDir(t *testing.T) {
	root := realDir(t)
	name, ok, warns, err := teamfile.FindByWorktree(filepath.Join(root, "nope"), root, "")
	if err != nil || ok || name != "" || warns != nil {
		t.Errorf("name=%q ok=%v warns=%v err=%v, want nothing", name, ok, warns, err)
	}
}
