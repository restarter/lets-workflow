//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
}

// taskStateTarget resolves the task-state file for dir's current HEAD branch. It
// always acts on HEAD: markdown never types a branch name into this command.
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
// can never pair one task's id with another task's start.
func TaskStateSet(ctx context.Context, dir string, o TaskStateOptions) (*TaskStateResult, error) {
	res := &TaskStateResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "task-state", Steps: []Step{}}}
	fail := func(e *Error) (*TaskStateResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	root, branch, letsDir, slug, e := taskStateTarget(ctx, dir)
	res.ProjectRoot = root
	if e != nil {
		return fail(e)
	}
	path := taskstate.Path(letsDir, slug)

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
	if len(set) == 0 {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "nothing to set"})
	}

	info := &TaskStateInfo{Path: path}
	res.TaskState = info
	if o.Orc != "" {
		home, _ := os.UserHomeDir()
		merge := letsconfig.ResolvedEnv(root, home, func(r string) string { return gitutil.DefaultBranch(r, 2*time.Second) })["LETS_MERGE_BRANCH"]
		if branch == merge {
			info.Reason = "orc_on_merge_branch"
			res.OK = true
			return res, nil
		}
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
		return nil, nil
	}
	st, err := taskstate.MergeWrite(letsDir, slug, taskstate.WriteOpts{Set: set, Create: o.Create, Deadline: time.Now().Add(wait), Derive: derive})
	switch {
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

func fillFrom(info *TaskStateInfo, st taskstate.State) {
	info.Task, info.Start, info.Session, info.Origin, info.Orc = st.Task, st.Start, st.Session, st.Origin, st.Orc
}
