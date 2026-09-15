package sessionstart

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// RefreshSessionBoundary updates the `session:` line of the current branch's
// .task-<slug> state file to the current HEAD + sessionID, ONLY if the file
// already exists (it never creates one - that would litter un-claimed
// branches). Every other line is preserved verbatim, through taskstate (the one
// owner of the file; atomic, locked with a 1 s deadline). Best-effort: every failure returns nil so the hook
// never blocks or fails a Claude Code session over this side effect.
//
// Intended to run on a genuinely NEW session (SessionStart source=startup), so
// /lets:end has a fresh session boundary even when the user skipped
// /lets:start. It must NOT run on resume/compact (the same session continues -
// moving the boundary forward would drop earlier commits).
//
// Semantics: session: = THIS session's start. Setting it to HEAD on a new
// session is correct even if a prior session left commits - those belong to the
// prior session and are its /lets:end's job to report; a session that skips both
// /lets:start AND /lets:end loses its commits from the running bd log regardless
// of this refresh. The deeper "boundary = last bd comment" model (a continuous
// log rather than per-session) is intentionally out of scope here - see
// lets-mic6d (the /lets:end settlement redesign).
func RefreshSessionBoundary(projectRoot, sessionID string) error {
	if projectRoot == "" || sessionID == "" {
		return nil
	}
	slug, ok := taskstate.Slug(gitOut(projectRoot, "branch", "--show-current")) // detached HEAD -> skip
	if !ok {
		return nil
	}
	head := gitOut(projectRoot, "rev-parse", "HEAD")
	if head == "" {
		return nil
	}
	// taskstate owns the file: it replaces only session:, keeps every other line,
	// never creates a missing file, and validates the value (a malformed sid is
	// refused, not written). Every failure stays silent - see the doc comment above.
	_, _ = taskstate.MergeWrite(filepath.Join(projectRoot, ".lets"), slug, taskstate.WriteOpts{
		Set:      map[string]string{"session": head + " " + sessionID},
		Deadline: time.Now().Add(time.Second),
	})
	return nil
}

// gitOut runs `git -C <projectRoot> <args...>` with a short timeout and returns
// trimmed stdout, or "" on any error.
func gitOut(projectRoot string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", projectRoot}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
