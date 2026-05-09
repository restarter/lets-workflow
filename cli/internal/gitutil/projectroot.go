// Package gitutil provides shared git-related helpers used across cli/.
//
// Split out so that hook/sessionstart, initcmd, and statusline don't each
// own a slightly-different copy of "find the git toplevel" with subtly
// different fallback semantics.
package gitutil

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// ProjectRoot returns the git repository toplevel.
//
//   - dir == "" → uses the caller's cwd (omits `git -C`).
//   - dir != "" → runs `git -C dir rev-parse --show-toplevel`.
//   - timeout == 0 → no timeout (use sparingly; recommended for one-shot CLI).
//   - timeout > 0 → context-bounded; recommended on hot paths (hooks, statusline).
//
// Returns "" on any failure (git not installed, not a git repo, timeout, etc.).
//
// Deliberately matches the bash `git rev-parse --show-toplevel 2>/dev/null`
// contract: silent on failure, no os.Getwd() fallback. Callers that need a
// fallback should make it explicit at the call site - silently injecting
// arbitrary cwd as "project root" can mislead downstream commands that trust
// the value.
func ProjectRoot(dir string, timeout time.Duration) string {
	args := []string{"rev-parse", "--show-toplevel"}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}

	var cmd *exec.Cmd
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "git", args...)
	} else {
		cmd = exec.Command("git", args...)
	}

	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Branch returns the current git branch name in dir (or cwd if dir == "").
// Returns "" on failure (detached HEAD, not a repo, etc.). Same silent-failure
// semantics as ProjectRoot — callers decide how to render absence.
func Branch(dir string, timeout time.Duration) string {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}

	var cmd *exec.Cmd
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "git", args...)
	} else {
		cmd = exec.Command("git", args...)
	}

	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		// Detached HEAD - not a useful branch name; surface as empty.
		return ""
	}
	return branch
}
