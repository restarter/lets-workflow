//go:build unix

package ccregistry

import (
	"errors"
	"os"
)

// MaxAncestry bounds the parent walk of CallerDescendsFrom: a real chain from a
// Claude Code session to a Bash-run `lets` is a handful of hops.
const MaxAncestry = 64

// ParentOf returns the parent pid of pid; an error when pid cannot be read (it
// exited, or the platform source is unavailable). A package var so tests inject a
// fake process tree. Linux reads /proc/<pid>/stat; other unix runs /bin/ps by
// absolute path, never a PATH lookup a caller's environment could redirect.
var ParentOf = parentOf

// errAncestryTooDeep is the walk that reached MaxAncestry without an answer.
var errAncestryTooDeep = errors.New("process ancestry deeper than the walk bound")

// CallerDescendsFrom reports whether pid is an ancestor of this process: the parent
// chain from os.Getppid() is walked (at most MaxAncestry hops) until it reaches pid
// (true), init or a self-parented process (false). A registry entry's pid is the
// Claude Code process of that session, so this binds a claimed session id to the
// process that actually runs the caller - an environment variable alone
// ($CLAUDE_CODE_SESSION_ID) is a claim any subprocess can set. A live check only:
// nothing is stored. pid <= 0 is never an ancestor; a hop that cannot be read is an
// error, which callers treat as "not proven".
func CallerDescendsFrom(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	cur := os.Getppid()
	for i := 0; i < MaxAncestry; i++ {
		if cur == pid {
			return true, nil
		}
		if cur <= 1 {
			return false, nil
		}
		next, err := ParentOf(cur)
		if err != nil {
			return false, err
		}
		if next == cur {
			return false, nil
		}
		cur = next
	}
	return false, errAncestryTooDeep
}
