package statusline

import (
	"os"
	"path/filepath"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// taskIDFor resolves the task id the statusline shows, once per render. No git fork
// and no tracker call on this path (the renderer shows the task line only after the
// detached fetch confirms the id):
//
//  1. off the merge-branch, the task-state file's task: line - a worktree branch is
//     frozen at create time and may host several tasks in turn, so the file wins;
//  2. off the merge-branch, the adapter-declared convention on the branch name
//     (created shapes only; accept: shapes are adopt's);
//  3. when the convention is undeclared, or declared but yielding nothing while the
//     tracker is beads, the legacy heuristic - hand-named branches such as
//     `fix/lets-abc-foo` keep their task line.
//
// On the merge-branch the result is always empty: a stale .task-main must not surface
// as the current task.
func taskIDFor(projectRoot, branch string) string {
	if projectRoot == "" || branch == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	env := letsconfig.ResolvedEnv(projectRoot, home, nil) // nil: never fork git on a render
	if branch == env["LETS_MERGE_BRANCH"] {
		return ""
	}
	if slug, ok := taskstate.Slug(branch); ok {
		if st, err := taskstate.Read(filepath.Join(projectRoot, ".lets"), slug); err == nil && taskid.Valid(st.Task) {
			return st.Task
		}
	}
	tracker := env["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	mainRoot := projectRoot
	if m, ok := gitutil.LinkedWorktreeMain(projectRoot); ok {
		mainRoot = m
	}
	conv, _ := trackeradapter.LoadConvention(mainRoot, tracker, "")
	if conv.Declared {
		if id, _, ok := conv.ParseBranch(branch, trackeradapter.Created); ok && taskid.Valid(id) {
			return id
		}
		if tracker != "beads" {
			return ""
		}
	}
	if id := legacyTaskID(branch); taskid.Valid(id) {
		return id
	}
	return ""
}

// trackerFetchable reports whether the detached task fetch can serve this project:
// it runs `bd show`, so only the beads tracker (the default) gets a spawn.
func trackerFetchable(projectRoot string) bool {
	home, _ := os.UserHomeDir()
	t := letsconfig.ResolvedEnv(projectRoot, home, nil)["LETS_TRACKER"]
	return t == "" || t == "beads"
}
