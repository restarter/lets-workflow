//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// ReleaseOptions configures Release. Release never refuses, so it has no force flag.
type ReleaseOptions struct{}

// Release runs when a worktree is about to be discarded by something else (Orca's
// archive hook). It deletes neither the checkout nor the branch, so refusing on
// dirty or unpushed work would protect nothing: it records what it saw, writes a
// released-<id> marker (with the session-record state) that `/lets:start --main`
// reads to offer reopening a task still in progress, and only then removes the
// task-state file - compare-and-delete, so a revision written after the read is kept.
// Never calls a tracker, never touches role files.
func Release(ctx context.Context, dir string, _ ReleaseOptions) (*ReleaseResult, error) {
	res := &ReleaseResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "release", Steps: []Step{}}}
	add := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }
	fail := func(e *Error) (*ReleaseResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}

	inWt, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = mainRoot
	if !inWt {
		return fail(&Error{Code: ExitNotLinkedWorktree, Kind: "not_linked_worktree", Message: "this is the main checkout, not a linked worktree"})
	}
	wtRoot := gitutil.ProjectRoot(dir, 2*time.Second)
	branch := currentBranchOf(ctx, wtRoot)
	info := &ReleasedInfo{Branch: stripControl(branch)}
	res.Released = info

	statusOut, _ := exec.CommandContext(ctx, "git", "-C", wtRoot, "status", "--porcelain").Output()
	info.Dirty = hasUserChanges(string(statusOut), loadStoreLinks(mainRoot, "", func(string, string) {}))
	out, err := exec.CommandContext(ctx, "git", "-C", wtRoot, "log", "@{u}..", "--oneline").CombinedOutput()
	info.Unpushed = err != nil || len(strings.TrimSpace(string(out))) > 0 // no upstream counts as unpushed

	// Order matters (lets-11zwo): read, then write the marker, and only then remove
	// the task-state file. A file that names a task goes only once that task's marker
	// is on disk; a read that failed, or an id that is not an id, keeps the file.
	letsDir := filepath.Join(mainRoot, ".lets")
	slug, hasSlug := taskstate.Slug(branch)
	removable := false
	if hasSlug {
		st, err := taskstate.Read(letsDir, slug)
		switch {
		case err == nil && taskid.Valid(st.Task):
			info.Task = st.Task
		case err == nil && st.Task == "":
			// Names no task: the normal state after /lets:done closed one (its cleanup
			// clears task:/start:/origin: and keeps session:). Nothing to reopen and
			// no marker to write - removing it is the old behaviour, kept on purpose.
			removable = true
		case err == nil:
			info.Kept = KeepInvalidID
			add(StepWarn, fmt.Sprintf("the task-state file names an invalid task id (%q) - kept", stripControl(st.Task)))
		case errors.Is(err, fs.ErrNotExist):
		default:
			info.Kept = KeepUnreadable
			add(StepWarn, fmt.Sprintf("could not read the task-state file: %v - kept", err))
		}
	} else {
		add(StepWarn, "detached HEAD: no task-state file to release")
	}

	if info.Task != "" {
		rec := recordOf(ctx, mainRoot, letsDir, info.Task, headOf(ctx, wtRoot))
		info.Record, info.Snapshot = &rec, rec.State
		marker := filepath.Join(letsDir, "cache", "released-"+info.Task)
		line := fmt.Sprintf("%s|%s|%s|dirty=%t|unpushed=%t|snapshot=%s\n", info.Task, info.Branch, time.Now().UTC().Format(time.RFC3339), info.Dirty, info.Unpushed, rec.State)
		if err := writeMarker(marker, line); err != nil {
			info.Kept = KeepNoMarker
			add(StepWarn, fmt.Sprintf("could not write %s: %v - task-state file kept", marker, err))
		} else {
			info.Marker = marker
			removable = true
			add(StepOK, "released marker written: "+marker)
		}
		if rec.State != RecordPresent {
			add(StepWarn, fmt.Sprintf("session record for %s: %s", info.Task, rec.State))
		}
	} else if hasSlug {
		add(StepWarn, "no task recorded for this worktree; no released marker written")
	}

	// Compare-and-delete, not read-then-delete: under the file's lock, remove it only
	// if it still names the task the marker was written for (or still names none).
	// A writer that got in between (a claim, a hook) keeps its revision.
	if hasSlug && removable {
		beforeTaskStateRemove()
		switch err := taskstate.RemoveIfTask(letsDir, slug, info.Task, time.Now().Add(5*time.Second)); {
		case err == nil:
			add(StepOK, "removed task-state file")
		case errors.Is(err, taskstate.ErrChanged):
			info.Kept = KeepChanged
			add(StepWarn, "the task-state file changed while releasing - kept; the marker covers only the state that was read")
		default:
			info.Kept = KeepRemoveFailed
			add(StepWarn, fmt.Sprintf("could not remove the task-state file: %v", err))
		}
	}
	if info.Dirty || info.Unpushed {
		var what []string
		if info.Dirty {
			what = append(what, "uncommitted changes")
		}
		if info.Unpushed {
			what = append(what, "unpushed commits")
		}
		add(StepWarn, "this worktree has "+strings.Join(what, " and ")+" - whatever archives it is about to discard them")
	}
	res.OK = true
	return res, nil
}

// beforeTaskStateRemove runs between the marker and the removal (a seam: a test
// writes the file here to prove the removal is conditional).
var beforeTaskStateRemove = func() {}

// Alarm names why this release must reach a person, or "" when it recorded all it
// should. A worktree vanishing without a record must be visible when it happens.
func (r *ReleaseResult) Alarm() string {
	i := r.Released
	if i == nil {
		return ""
	}
	var why []string
	if i.Task != "" && i.Snapshot != "" && i.Snapshot != RecordPresent {
		why = append(why, fmt.Sprintf("%s was archived without a current session record (snapshot=%s)", i.Task, i.Snapshot))
	}
	if i.Kept != "" {
		why = append(why, fmt.Sprintf("the task-state file of %s was kept (%s)", i.Branch, i.Kept))
	}
	return strings.Join(why, "; ")
}

// writeMarker writes a released marker atomically (0600).
func writeMarker(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// stripControl drops control characters and the marker's `|` separator.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '|' {
			return -1
		}
		return r
	}, s)
}
