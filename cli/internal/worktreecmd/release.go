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
// dirty or unpushed work would protect nothing: it records what it saw, removes the
// task-state file, and leaves a released-<id> marker that `/lets:start --main` reads
// to offer reopening a task still in progress. Never calls a tracker, never touches
// role files.
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

	letsDir := filepath.Join(mainRoot, ".lets")
	slug, hasSlug := taskstate.Slug(branch)
	if hasSlug {
		if st, err := taskstate.Read(letsDir, slug); err == nil && taskid.Valid(st.Task) {
			info.Task = st.Task
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			add(StepWarn, fmt.Sprintf("could not read the task-state file: %v", err))
		}
		if err := taskstate.Remove(letsDir, slug, time.Now().Add(5*time.Second)); err != nil {
			add(StepWarn, fmt.Sprintf("could not remove the task-state file: %v", err))
		} else {
			add(StepOK, "removed task-state file")
		}
	} else {
		add(StepWarn, "detached HEAD: no task-state file to release")
	}

	if info.Task == "" {
		add(StepWarn, "no task recorded for this worktree; no released marker written")
	} else {
		marker := filepath.Join(letsDir, "cache", "released-"+info.Task)
		line := fmt.Sprintf("%s|%s|%s|dirty=%t|unpushed=%t\n", info.Task, info.Branch, time.Now().UTC().Format(time.RFC3339), info.Dirty, info.Unpushed)
		if err := writeMarker(marker, line); err != nil {
			add(StepWarn, fmt.Sprintf("could not write %s: %v", marker, err))
		} else {
			info.Marker = marker
			add(StepOK, "released marker written: "+marker)
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
