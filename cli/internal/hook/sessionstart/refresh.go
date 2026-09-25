package sessionstart

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// Outcome is what a guarded session: write did.
type Outcome int

const (
	OutcomeSkipped   Outcome = iota // nothing to act on: no file, no branch, no HEAD, bad input
	OutcomeWritten                  // the line now names this session
	OutcomeUnchanged                // it already did (or a carry had nothing to carry)
	OutcomeHeld                     // another session owns it, or its owner cannot be judged
	OutcomeNoProof                  // a carry without proof that the recorded id became this one
	OutcomeBusy                     // the task lock stayed busy past the deadline
)

// Allowed reports an outcome in which this session owns the line.
func (o Outcome) Allowed() bool { return o == OutcomeWritten || o == OutcomeUnchanged }

// sessionLockDeadline bounds the wait for the task lock: the SessionStart hook
// must never stall a session start. A timeout is held, never a blind write.
const sessionLockDeadline = time.Second

// RefreshSessionBoundary updates the `session:` line of the current branch's
// .task-<slug> state file to the current HEAD + sessionID, ONLY if the file
// already exists (it never creates one - that would litter un-claimed
// branches). Every other line is preserved verbatim, through taskstate (the one
// owner of the file; atomic, locked with a 1 s deadline). The guard g decides,
// under that lock, whether this session may write at all (taskstate.MaySetSession):
// a teammate pane in the same worktree runs this same hook and must never take the
// lead's boundary. A zero guard keeps the unguarded behaviour. Best-effort: nothing
// here fails the hook; a declined write returns a Notice message instead.
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
func RefreshSessionBoundary(projectRoot, sessionID string, g taskstate.SessionGuard) (Outcome, string) {
	if projectRoot == "" || sessionID == "" {
		return OutcomeSkipped, ""
	}
	slug, ok := taskstate.Slug(gitOut(projectRoot, "branch", "--show-current")) // detached HEAD -> skip
	if !ok {
		return OutcomeSkipped, ""
	}
	head := gitOut(projectRoot, "rev-parse", "HEAD")
	if head == "" {
		return OutcomeSkipped, ""
	}
	// taskstate owns the file: it replaces only session:, keeps every other line,
	// never creates a missing file, and validates the value (a malformed sid is
	// refused, not written).
	_, err := taskstate.MergeWrite(filepath.Join(projectRoot, ".lets"), slug, taskstate.WriteOpts{
		Deadline: time.Now().Add(sessionLockDeadline),
		Derive: func(cur taskstate.State, exists bool) (map[string]string, error) {
			if err := taskstate.MaySetSession(cur, exists, sessionID, taskstate.OpRefresh, g); err != nil {
				return nil, err
			}
			return map[string]string{"session": head + " " + sessionID}, nil
		},
	})
	return outcome(err, slug, taskstate.OpRefresh)
}

// CarrySession moves the recorded boundary to this session's new id after /clear,
// keeping its sha: /clear re-mints the id in the same process, and without the carry
// /lets:end would no longer recognize its own session. It writes ONLY on proof
// (g.Rotated shows the recorded id was re-minted into sessionID); no proof writes
// nothing and returns a warning naming the gap.
func CarrySession(projectRoot, sessionID string, g taskstate.SessionGuard) (Outcome, string) {
	if projectRoot == "" || sessionID == "" {
		return OutcomeSkipped, ""
	}
	slug, ok := taskstate.Slug(gitOut(projectRoot, "branch", "--show-current"))
	if !ok {
		return OutcomeSkipped, ""
	}
	_, err := taskstate.MergeWrite(filepath.Join(projectRoot, ".lets"), slug, taskstate.WriteOpts{
		Deadline: time.Now().Add(sessionLockDeadline),
		Derive: func(cur taskstate.State, exists bool) (map[string]string, error) {
			if err := taskstate.MaySetSession(cur, exists, sessionID, taskstate.OpCarry, g); err != nil {
				return nil, err
			}
			sha, _, _ := strings.Cut(cur.Session, " ")
			return map[string]string{"session": sha + " " + sessionID}, nil
		},
	})
	return outcome(err, slug, taskstate.OpCarry)
}

// outcome maps a MergeWrite result to an Outcome and its Notice message ("" when
// there is nothing to say). A declined write always says so: silence is how a
// foreign boundary would pass for this session's.
func outcome(err error, slug string, op taskstate.SessionOp) (Outcome, string) {
	file := ".task-" + slug
	label := "not refreshed"
	if op == taskstate.OpCarry {
		label = "not carried to this session after /clear"
	}
	var held *taskstate.HeldError
	switch {
	case err == nil:
		if op == taskstate.OpCarry {
			return OutcomeWritten, "The session boundary in " + file + " was carried to this session's new id after /clear."
		}
		return OutcomeWritten, ""
	case errors.Is(err, taskstate.ErrSessionUnchanged):
		return OutcomeUnchanged, ""
	case errors.Is(err, taskstate.ErrFileAbsent):
		return OutcomeSkipped, "" // never creates one
	case errors.Is(err, fsutil.ErrLockBusy):
		return OutcomeBusy, fmt.Sprintf("The session boundary in %s was %s: its lock stayed busy. If this chat owns the task, run /lets:start in that chat to rewrite the session.", file, label)
	case errors.As(err, &held):
		switch held.Reason {
		case taskstate.HeldNoProof:
			return OutcomeNoProof, fmt.Sprintf("The session boundary in %s (session %s) was %s: nothing proves that id was this chat's (no peer role file recorded it), so /lets:end will report an approximate range. Run /lets:start in this chat to rewrite the session.", file, short(held.Holder), label)
		case taskstate.HeldLead:
			return OutcomeHeld, fmt.Sprintf("The session boundary in %s was %s: this is a team worktree and its recorded lead is %s - a teammate never takes the lead's boundary. If this chat is the lead, run /lets:start in that chat to rewrite the session.", file, label, held.Holder)
		case taskstate.HeldUnknown:
			return OutcomeHeld, fmt.Sprintf("The session boundary in %s was %s: it names session %s, which the session registry cannot prove ended. If this chat owns the task, run /lets:start in that chat to rewrite the session.", file, label, short(held.Holder))
		}
		return OutcomeHeld, fmt.Sprintf("The session boundary in %s was %s: it is held by live session %s. If this chat owns the task, run /lets:start in that chat to rewrite the session.", file, label, short(held.Holder))
	}
	return OutcomeSkipped, "" // any other failure stays silent, as before the guard
}

func short(sid string) string {
	if len(sid) > 8 {
		return sid[:8]
	}
	return sid
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
