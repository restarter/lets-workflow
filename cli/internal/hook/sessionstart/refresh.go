package sessionstart

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RefreshSessionBoundary updates the `session:` line of the current branch's
// .task-<slug> state file to the current HEAD + sessionID, ONLY if the file
// already exists (it never creates one - that would litter un-claimed
// branches). The `task:` and `start:` lines are preserved verbatim. The write
// is atomic (temp file + rename in the same dir, so it survives the .lets
// symlink in worktrees). Best-effort: every failure returns nil so the hook
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
	branch := gitOut(projectRoot, "branch", "--show-current") // empty on detached HEAD -> skip
	if branch == "" {
		return nil
	}
	slug := strings.ReplaceAll(branch, "/", "-")
	path := filepath.Join(projectRoot, ".lets", "sessions", ".task-"+slug)

	existing, err := os.ReadFile(path)
	if err != nil {
		return nil // refresh-if-exists: absent/unreadable -> nothing to do
	}
	head := gitOut(projectRoot, "rev-parse", "HEAD")
	if head == "" {
		return nil
	}

	// Keep every non-session line; replace (or append) a single session: line.
	var out []string
	sawSession := false
	sc := bufio.NewScanner(strings.NewReader(string(existing)))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "session: "):
			out = append(out, "session: "+head+" "+sessionID)
			sawSession = true
		case strings.TrimSpace(line) == "":
			// drop blank lines
		default:
			out = append(out, line)
		}
	}
	if !sawSession {
		out = append(out, "session: "+head+" "+sessionID)
	}
	return atomicWrite(path, strings.Join(out, "\n")+"\n")
}

// atomicWrite writes content to path via a same-dir temp file + rename.
// Best-effort: failures are swallowed (returns nil) - this is a non-critical
// refresh and must never break the hook.
func atomicWrite(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".task-*.tmp")
	if err != nil {
		return nil
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return nil
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return nil
	}
	_ = os.Rename(name, path)
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
