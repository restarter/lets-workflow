//go:build unix

package ccregistry

import (
	"errors"
	"fmt"
	"os"
)

// MaxAncestry bounds the parent walk of CallerSession: a real chain from a Claude
// Code session to a Bash-run `lets` is a handful of hops.
const MaxAncestry = 64

// ParentOf returns the parent pid of pid; an error when pid cannot be read (it
// exited, or the platform source is unavailable). A package var so tests inject a
// fake process tree. Linux reads /proc/<pid>/stat; other unix runs /bin/ps by
// absolute path, never a PATH lookup a caller's environment could redirect.
var ParentOf = parentOf

// ErrNoCallerSession is a walk that reached init (or a self-parented process)
// without passing a live Claude Code registry process: this command does not run
// under a Claude Code session.
var ErrNoCallerSession = errors.New("no Claude Code session among this process's ancestors")

// errAncestryTooDeep is the walk that reached MaxAncestry without an answer.
var errAncestryTooDeep = errors.New("process ancestry deeper than the walk bound")

// CallerSession is the Claude Code session that runs this process: the NEAREST
// ancestor (walking from os.Getppid(), at most MaxAncestry hops) that is a live
// registry process of s. Ordinary shells and wrappers in between are transparent; a
// Claude process is not - a session launched as a subprocess of another session is
// its own caller, never its parent's. A registry process that is not a usable
// Entries row (Unrecognized, Loose) stops the walk with an error: an intervening
// Claude process that cannot be identified fails closed. ErrNoCallerSession when no
// registry process is passed before init; an unreadable hop or the depth bound is an
// error too. A live check only: nothing is stored.
//
// $CLAUDE_CODE_SESSION_ID is a claim any subprocess can set; a caller proves it by
// comparing that id with CallerSession().SessionID.
func (s Snapshot) CallerSession() (Entry, error) {
	loose := map[int]bool{}
	for _, pid := range s.Loose {
		loose[pid] = true
	}
	cur := os.Getppid()
	for i := 0; i < MaxAncestry; i++ {
		if e, ok := s.FindPid(cur); ok {
			return e, nil
		}
		if label, ok := s.Unrecognized[cur]; ok {
			return Entry{}, fmt.Errorf("the nearest Claude Code process (pid %d) cannot be identified: %s", cur, label)
		}
		if loose[cur] {
			return Entry{}, fmt.Errorf("the nearest Claude Code process (pid %d) cannot be identified", cur)
		}
		if cur <= 1 {
			return Entry{}, ErrNoCallerSession
		}
		next, err := ParentOf(cur)
		if err != nil {
			return Entry{}, err
		}
		if next == cur {
			return Entry{}, ErrNoCallerSession
		}
		cur = next
	}
	return Entry{}, errAncestryTooDeep
}
