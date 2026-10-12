//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// TaskStateOptions configures TaskStateSet. Every field maps to one flag of
// `lets worktree task-state set`; an empty string means "not given".
type TaskStateOptions struct {
	Task, Start, SessionSHA, SessionID, Orc  string
	ClearOrigin, ClearOrc, ClearTask, Create bool
	Wait                                     time.Duration // lock deadline (0 = default 5s)

	// The merge-branch `orchestrator:` records. Orchestrator (or SelfOrchestrator:
	// this session's registry name) names the record to create or patch; a nil
	// OrchestratorScope / Standing leaves that field as recorded (the CLI sets a
	// pointer only for a flag that was given). ClearOrchestrator removes a record.
	Orchestrator      string
	SelfOrchestrator  bool
	OrchestratorScope *string
	Standing          *string // "on" | "off"
	ClearOrchestrator string
	// Session is the caller's session id ($CLAUDE_CODE_SESSION_ID, set by the cli
	// layer), read only by SelfOrchestrator.
	Session string
	// Guard judges a --session-id write under the task lock: a live foreign holder
	// (HeldLive, HeldRotatedForeign) refuses it. A zero guard writes as before.
	Guard taskstate.SessionGuard
	// Proof is recorded as session_proof beside the session: line; nil deletes the
	// line (the caller proves only its own process).
	Proof *taskstate.Proof
}

// orchestratorOps reports whether o touches an `orchestrator:` record.
func (o TaskStateOptions) orchestratorOps() bool {
	return o.Orchestrator != "" || o.SelfOrchestrator || o.OrchestratorScope != nil || o.Standing != nil || o.ClearOrchestrator != ""
}

// orchestratorOnly reports whether o touches orchestrator records and nothing of
// HEAD's own task-state.
func (o TaskStateOptions) orchestratorOnly() bool {
	return o.orchestratorOps() && o.Task == "" && o.Start == "" && o.SessionSHA == "" && o.SessionID == "" && o.Orc == "" &&
		!o.ClearOrigin && !o.ClearOrc && !o.ClearTask
}

// selfName is the caller's registry name, or a refusal: orchestrator_needs_name (no
// usable name), orchestrator_not_own_process (the claimed session does not run this
// command), orchestrator_name_not_unique (another live session carries the name -
// checked only when unique is true: a pure --standing off only tightens, so a shared
// name may still revoke).
func selfName(sid string, unique bool) (string, *Error) {
	if !ccregistry.ValidSession(sid) {
		return "", &Error{Code: ExitUsage, Kind: "usage", Message: "--self-orchestrator needs this session's id ($CLAUDE_CODE_SESSION_ID)",
			Remediation: "run inside a Claude Code session"}
	}
	remedy := "name this session with /rename <name>, then run /lets:start --main again"
	snap := ccregistry.Read(ccregistry.HomeDir())
	e, ok := snap.Find(sid)
	switch {
	case !ok:
		return "", &Error{Code: ExitGeneric, Kind: "orchestrator_needs_name", Message: "this session is not in the Claude Code session registry, so it has no name to record", Remediation: remedy}
	case !e.NameOK && e.Name != "":
		return "", &Error{Code: ExitGeneric, Kind: "orchestrator_needs_name", Message: fmt.Sprintf("this session has no usable name (%q fails the name grammar)", e.Name), Remediation: remedy}
	case !e.NameOK:
		return "", &Error{Code: ExitGeneric, Kind: "orchestrator_needs_name", Message: "this session has no name to record as an orchestrator", Remediation: remedy}
	}
	// The session id is only a claim ($CLAUDE_CODE_SESSION_ID can be set by any
	// subprocess, a nested Claude session included): the record is written only when
	// the NEAREST Claude Code process above this command is that session. Any doubt -
	// an unidentified Claude process in between, an unreadable hop, no session at all
	// - refuses.
	caller, err := snap.CallerSession()
	if err != nil || caller.SessionID != sid {
		msg := fmt.Sprintf("another Claude Code session (%s, pid %d) runs this command, not session %s", caller.SessionID, caller.Pid, sid)
		if err != nil {
			msg = fmt.Sprintf("could not prove session %s runs this command: %v", sid, err)
		}
		return "", &Error{Code: ExitUsage, Kind: "orchestrator_not_own_process", Message: msg, Cause: err,
			Remediation: "run it from the orchestrator's own Claude Code session (its Bash tool), not from another process"}
	}
	if caller.Name != e.Name { // one session id under two processes (claude -r): the caller's own name counts
		if !caller.NameOK {
			return "", &Error{Code: ExitGeneric, Kind: "orchestrator_needs_name", Message: "this session's process has no usable name to record", Remediation: remedy}
		}
		e = caller
	}
	if !unique {
		return e.Name, nil
	}
	// After the process proof: only the session's own process learns whether its name
	// is shared. A record is keyed by name, so a second live holder would write the
	// first one's grant - the same rule as the read side, where a shared name carries
	// no self_record.
	holders := 0
	for _, x := range snap.Entries {
		if x.NameOK && x.Name == e.Name {
			holders++
		}
	}
	if holders > 1 {
		return "", &Error{Code: ExitUsage, Kind: "orchestrator_name_not_unique",
			Message:     fmt.Sprintf("%d live sessions are named %s - a record is keyed by name, so it cannot be written for one of them", holders, e.Name),
			Remediation: "rename this session with /rename <name>, then run it again"}
	}
	return e.Name, nil
}

// taskStateTarget resolves the task-state file for dir's current HEAD branch. It
// always acts on HEAD: markdown never types a branch name into this command. The
// one exception is TaskStateSet's orchestrator records, which go to the
// merge-branch file whatever HEAD is.
func taskStateTarget(ctx context.Context, dir string) (root, branch, letsDir, slug string, e *Error) {
	root = gitutil.ProjectRoot(dir, 2*time.Second)
	if root == "" {
		return "", "", "", "", &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	branch = currentBranchOf(ctx, root)
	s, ok := taskstate.Slug(branch)
	if !ok {
		return root, "", "", "", &Error{Code: ExitUsage, Kind: "detached_head", Message: "detached HEAD has no task-state file"}
	}
	return root, branch, filepath.Join(root, ".lets"), s, nil
}

func infoFrom(path string, st taskstate.State) *TaskStateInfo {
	return &TaskStateInfo{Path: path, Task: st.Task, Start: st.Start, Session: st.Session, Origin: st.Origin, Orc: st.Orc}
}

// TaskStateShow reads the task-state file for dir's HEAD branch.
func TaskStateShow(ctx context.Context, dir string) (*TaskStateResult, error) {
	res := &TaskStateResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "task-state", Steps: []Step{}}}
	root, _, letsDir, slug, e := taskStateTarget(ctx, dir)
	res.ProjectRoot = root
	if e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	path := taskstate.Path(letsDir, slug)
	st, err := taskstate.Read(letsDir, slug)
	switch {
	case errors.Is(err, os.ErrNotExist):
		res.TaskState = &TaskStateInfo{Path: path, Reason: "file_absent"}
	case err != nil:
		e := &Error{Code: ExitFilesystem, Kind: "task_state_read_failed", Message: err.Error(), Cause: err}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	default:
		res.TaskState = infoFrom(path, st)
	}
	res.OK = true
	return res, nil
}

// TaskStateSet is the ONE markdown-facing writer of the task-state file. Two guards
// make every caller safe by construction: an `orc:` binding is never written on the
// merge-branch, and a --task that names a different task than the file does is
// refused unless --start comes with it (take-task always passes both), so a writer
// can never pair one task's id with another task's start. A --session-id write is
// guarded too: a live foreign holder of the session: line refuses the whole call
// (exit 35 session_held, nothing written); unknown, dead, no-proof and unchanged
// holders write - the "/lets:start in this chat rewrites the session" remedy.
func TaskStateSet(ctx context.Context, dir string, o TaskStateOptions) (*TaskStateResult, error) {
	res := &TaskStateResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "task-state", Steps: []Step{}}}
	fail := func(e *Error) (*TaskStateResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	root, branch, letsDir, slug, e := taskStateTarget(ctx, dir)
	res.ProjectRoot = root
	// Orchestrator records alone need no HEAD branch: they go to the merge-branch file,
	// so a detached HEAD refuses only a call that also writes this branch's own keys.
	recordsOnDetached := e != nil && e.Kind == "detached_head" && o.orchestratorOnly()
	if e != nil && !recordsOnDetached {
		return fail(e)
	}
	path := ""
	if slug != "" {
		path = taskstate.Path(letsDir, slug)
	}

	set := map[string]string{}
	if (o.SessionSHA == "") != (o.SessionID == "") {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--session-sha and --session-id go together"})
	}
	if o.Task != "" {
		set["task"] = o.Task
	}
	if o.Start != "" {
		set["start"] = o.Start
	}
	if o.SessionSHA != "" {
		set["session"] = o.SessionSHA + " " + o.SessionID
	}
	if o.Orc != "" {
		set["orc"] = o.Orc
	}
	if o.ClearOrigin {
		set["origin"] = ""
	}
	if o.ClearOrc {
		if o.Orc != "" {
			return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--orc and --clear-orc conflict"})
		}
		set["orc"] = ""
	}
	if o.ClearTask {
		if o.Task != "" || o.Start != "" {
			return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--clear-task conflicts with --task / --start"})
		}
		set["task"], set["start"], set["origin"] = "", "", ""
	}
	for k, v := range set {
		if err := taskstate.Validate(k, v); err != nil {
			return fail(&Error{Code: ExitUsage, Kind: "invalid_value", Message: err.Error(), Cause: err})
		}
	}
	orch := o.orchestratorOps()
	if orch {
		if e := orchestratorUsage(o); e != nil {
			return fail(e)
		}
	}
	if len(set) == 0 && !orch {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "nothing to set"})
	}

	if o.Orc != "" || orch {
		// The merge-branch as the writer resolves it, from the main checkout (a worktree
		// shares its .lets): orchestrator records always live in ITS task-state, whatever
		// HEAD is - the orc skill's `standing` verb runs from a worktree too.
		mainRoot := root
		if _, m := gitutil.DetectInsideWorktreeAt(root); m != "" {
			mainRoot = m
		}
		home, _ := os.UserHomeDir()
		merge := letsconfig.ResolvedEnv(mainRoot, home, func(r string) string { return gitutil.DefaultBranch(r, 2*time.Second) })["LETS_MERGE_BRANCH"]
		if orch {
			ms, ok := taskstate.Slug(merge)
			if !ok {
				return fail(&Error{Code: ExitGeneric, Kind: "merge_branch_unknown", Message: "the merge-branch could not be resolved"})
			}
			if branch != merge && len(set) > 0 {
				return fail(&Error{Code: ExitUsage, Kind: "usage",
					Message: fmt.Sprintf("orchestrator records go to the merge-branch (%s) task-state; this branch's keys cannot be written in the same call from %s", merge, branch)})
			}
			letsDir, slug = filepath.Join(mainRoot, ".lets"), ms
			path = taskstate.Path(letsDir, slug)
		}
		if o.Orc != "" && branch == merge {
			res.TaskState = &TaskStateInfo{Path: path, Reason: "orc_on_merge_branch"}
			res.OK = true
			return res, nil
		}
	}
	info := &TaskStateInfo{Path: path}
	res.TaskState = info
	var patch *taskstate.OrchestratorPatch
	if o.Orchestrator != "" || o.SelfOrchestrator {
		name := o.Orchestrator
		if o.SelfOrchestrator {
			// a pure --standing off only tightens: a shared name may still revoke
			revokeOnly := o.Standing != nil && *o.Standing == "off" && o.OrchestratorScope == nil
			n, e := selfName(o.Session, !revokeOnly)
			if e != nil {
				return fail(e)
			}
			name = n
		}
		patch = &taskstate.OrchestratorPatch{Name: name, Scope: o.OrchestratorScope, Standing: o.Standing}
	}

	wait := o.Wait
	if wait == 0 {
		wait = 5 * time.Second
	}
	// The mismatch guard and the rebound report read the file under the write's lock:
	// checked before it, a writer recording task B/start B in between would pair this
	// --task with B's start.
	var current taskstate.State
	var existed bool
	errMismatch := errors.New("task_mismatch")
	derive := func(cur taskstate.State, exists bool) (map[string]string, error) {
		current, existed = cur, exists
		if exists && o.Task != "" && cur.Task != "" && cur.Task != o.Task && o.Start == "" {
			return nil, errMismatch
		}
		if o.SessionSHA == "" {
			return nil, nil
		}
		if o.Guard.Liveness != nil {
			err := taskstate.MaySetSession(cur, exists, o.SessionID, taskstate.OpRefresh, o.Guard)
			var held *taskstate.HeldError
			if errors.As(err, &held) && (held.Reason == taskstate.HeldLive || held.Reason == taskstate.HeldRotatedForeign) {
				return nil, ErrSessionHeld(held, branch) // the file is untouched
			}
			// nil, unchanged, unknown, no proof: write as before
		}
		return taskstate.SessionValues(o.SessionSHA, o.SessionID, o.Proof), nil
	}
	st, err := taskstate.MergeWrite(letsDir, slug, taskstate.WriteOpts{Set: set, Create: o.Create || patch != nil, Deadline: time.Now().Add(wait), Derive: derive,
		PatchOrchestrator: patch, ClearOrchestrator: o.ClearOrchestrator})
	var we *Error
	switch {
	case errors.Is(err, taskstate.ErrInvalidValue):
		return fail(&Error{Code: ExitUsage, Kind: "invalid_value", Message: err.Error(), Cause: err})
	case errors.Is(err, errMismatch):
		info.Reason = "task_mismatch"
		fillFrom(info, current)
		res.OK = true
		return res, nil
	case errors.Is(err, taskstate.ErrFileAbsent):
		info.Reason = "file_absent"
		res.OK = true
		return res, nil
	case errors.Is(err, taskstate.ErrLockBusy):
		return fail(&Error{Code: ExitTaskStateLockBusy, Kind: "task_state_lock_busy",
			Message: fmt.Sprintf("%s is locked by another writer", path), Remediation: "retry, or raise --wait", Cause: err})
	case errors.As(err, &we):
		return fail(we)
	case err != nil:
		return fail(&Error{Code: ExitFilesystem, Kind: "task_state_write_failed", Message: err.Error(), Cause: err})
	}
	info.Written = true
	fillFrom(info, st)
	if o.Orc != "" && existed && current.Orc != "" && current.Orc != o.Orc {
		info.Rebound = &Rebound{From: current.Orc}
	}
	res.OK = true
	return res, nil
}

// orchestratorUsage refuses the orchestrator flag combinations that cannot mean one
// write, and a standing grant written for a name other than the caller's own.
func orchestratorUsage(o TaskStateOptions) *Error {
	usage := func(msg string) *Error { return &Error{Code: ExitUsage, Kind: "usage", Message: msg} }
	named := o.Orchestrator != "" || o.SelfOrchestrator
	switch {
	case o.Orchestrator != "" && o.SelfOrchestrator:
		return usage("--orchestrator and --self-orchestrator conflict")
	case o.Standing != nil && *o.Standing == "on" && !o.SelfOrchestrator:
		// a grant relaxes the orchestrator's own send gate: only its own session
		// writes it; --standing off and --clear-orchestrator only tighten
		return usage("--standing on is written only by the orchestrator itself (--self-orchestrator)")
	case o.ClearOrchestrator != "" && (named || o.OrchestratorScope != nil || o.Standing != nil):
		return usage("--clear-orchestrator conflicts with --orchestrator / --self-orchestrator / --orchestrator-scope / --standing")
	case !named && o.ClearOrchestrator == "":
		return usage("--orchestrator-scope and --standing need --orchestrator or --self-orchestrator")
	case o.Orc != "":
		return usage("--orc binds a worker branch; orchestrator records live on the merge-branch")
	case o.Task != "" || o.Start != "":
		// a task_mismatch would skip the whole write, the record with it
		return usage("--task / --start cannot be combined with orchestrator records")
	}
	return nil
}

func fillFrom(info *TaskStateInfo, st taskstate.State) {
	info.Task, info.Start, info.Session, info.Origin, info.Orc = st.Task, st.Start, st.Session, st.Origin, st.Orc
}
