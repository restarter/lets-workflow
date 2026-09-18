package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyRows is the historical branch-name table; every row must keep its answer
// both through legacyTaskID and through taskIDFor under the declared beads adapter.
var legacyRows = map[string]string{
	"feature/lets-ds6bc-statusline-2-0":  "lets-ds6bc",
	"worktree-lets-ds6bc-statusline-2-0": "lets-ds6bc",
	"bug/lets-asdsad-asdasd":             "lets-asdsad",
	"fix/lets-abc-foo":                   "lets-abc",
	"bugfix-2/lets-abc-foo":              "lets-abc", // hyphenated prefix stripped
	"lets-hdrdr.3-subtask":               "lets-hdrdr.3",
	"main":                               "",
	"":                                   "",
}

func TestLegacyTaskID(t *testing.T) {
	for branch, want := range legacyRows {
		if got := legacyTaskID(branch); got != want {
			t.Errorf("legacyTaskID(%q)=%q, want %q", branch, got, want)
		}
	}
}

const beadsWorktreeSection = "# Tracker adapter: beads\n\n## Worktree\n\n" +
	"links: `.beads/.env` (0600).\n" +
	"id: `[a-z][a-z0-9]*-[a-z0-9]+(\\.[0-9]+)?`.\n" +
	"branch: `feature/{id}-{slug}`.\n" +
	"worktree-branch: `worktree-{id}-{slug}`.\n" +
	"accept: `{id}-{slug}`.\n"

const noneWorktreeSection = "# Tracker adapter: none\n\n## Worktree\n\n" +
	"links: nothing.\n" +
	"id: nothing.\n" +
	"branch: `feature/{id}-{slug}`.\n" +
	"worktree-branch: `worktree-{id}-{slug}`.\n"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// project builds a non-git project root with an optional tracker adapter, board
// file and .lets/.env; HOME is isolated so the user's ~/.lets/.env never leaks in.
func project(t *testing.T, env, adapterName, adapter, board string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if env != "" {
		writeFile(t, filepath.Join(root, ".lets", ".env"), env)
	}
	if adapter != "" {
		writeFile(t, filepath.Join(root, ".claude", "rules", "tracker-"+adapterName+".md"), adapter)
	}
	if board != "" {
		writeFile(t, filepath.Join(root, ".claude", "rules", "tracker-"+adapterName+".board.md"), board)
	}
	return root
}

func writeTask(t *testing.T, root, branch, task string) {
	t.Helper()
	slug := strings.ReplaceAll(branch, "/", "-")
	writeFile(t, filepath.Join(root, ".lets", "sessions", ".task-"+slug), "task: "+task+"\n")
}

func TestTaskIDFor_DeclaredBeadsKeepsLegacyRows(t *testing.T) {
	root := project(t, "LETS_TRACKER=beads\n", "beads", beadsWorktreeSection, "")
	for branch, want := range legacyRows {
		if got := taskIDFor(root, branch); got != want {
			t.Errorf("taskIDFor(%q)=%q, want %q", branch, got, want)
		}
	}
}

func TestTaskIDFor(t *testing.T) {
	cases := []struct {
		name, env, adapterName, adapter, board, branch, task, want string
	}{
		{name: "beads declared, created shape", env: "LETS_TRACKER=beads\n", adapterName: "beads", adapter: beadsWorktreeSection,
			branch: "feature/lets-abc-x", want: "lets-abc"},
		{name: "task file wins off the merge-branch", env: "LETS_TRACKER=beads\n", adapterName: "beads", adapter: beadsWorktreeSection,
			branch: "feature/lets-abc-x", task: "lets-zzz", want: "lets-zzz"},
		{name: "task file ignored on the merge-branch", env: "LETS_TRACKER=beads\nLETS_MERGE_BRANCH=trunk\n", adapterName: "beads", adapter: beadsWorktreeSection,
			branch: "trunk", task: "lets-zzz", want: ""},
		{name: "unsafe task file ignored", env: "LETS_TRACKER=beads\n", adapterName: "beads", adapter: beadsWorktreeSection,
			branch: "worktree-custom", task: "-rf", want: ""},
		// An accept: shape is adopt-only, but under beads the legacy fallback still
		// reads it (same shape as lets-hdrdr.3-subtask); a false positive costs one
		// bd show, never wrong data.
		{name: "beads accept shape via legacy fallback", env: "LETS_TRACKER=beads\n", adapterName: "beads", adapter: beadsWorktreeSection,
			branch: "lets-ip06f-peer-messaging-orca", want: "lets-ip06f"},
		{name: "board override", env: "LETS_TRACKER=planfix-mcp\n", adapterName: "planfix-mcp", adapter: noneWorktreeSection,
			board: "## Worktree\n\nid: `PWA-[0-9]+`.\n", branch: "feature/PWA-45122-fix", want: "PWA-45122"},
		{name: "board override, no match, non-beads", env: "LETS_TRACKER=planfix-mcp\n", adapterName: "planfix-mcp", adapter: noneWorktreeSection,
			board: "## Worktree\n\nid: `PWA-[0-9]+`.\n", branch: "fix/lets-abc-foo", want: ""},
		{name: "none adapter", env: "LETS_TRACKER=none\n", adapterName: "none", adapter: noneWorktreeSection,
			branch: "feature/lets-abc-x", want: ""},
		{name: "undeclared adapter keeps legacy", env: "LETS_TRACKER=beads\n", adapterName: "beads", adapter: "# Tracker adapter: beads\n",
			branch: "fix/lets-abc-foo", want: "lets-abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := project(t, c.env, c.adapterName, c.adapter, c.board)
			if c.task != "" {
				writeTask(t, root, c.branch, c.task)
			}
			if got := taskIDFor(root, c.branch); got != c.want {
				t.Errorf("taskIDFor(%q)=%q, want %q", c.branch, got, c.want)
			}
		})
	}
}

func TestRender_NonBeadsTrackerSpawnsNoTaskFetch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := newRepo(t, "main")
	gitRun(t, repo, "checkout", "-q", "-b", "feature/lets-abc-x")
	writeFile(t, filepath.Join(repo, ".lets", ".env"), "LETS_TRACKER=planfix-mcp\n")

	s := installSpies(t)
	renderAt(t, repo)
	if s.taskSpawn != 0 || s.placeholder != 0 {
		t.Errorf("planfix-mcp must spawn no bd task fetch, got %+v", *s)
	}

	writeFile(t, filepath.Join(repo, ".lets", ".env"), "LETS_TRACKER=beads\n")
	s = installSpies(t)
	renderAt(t, repo)
	if s.taskSpawn != 1 {
		t.Errorf("beads must spawn the task fetch, got %+v", *s)
	}
}
