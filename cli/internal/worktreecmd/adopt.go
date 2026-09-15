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
	"slices"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// AdoptOptions configures Adopt.
type AdoptOptions struct {
	Task         string        // explicit task id (--task); supersedes a derived one
	ReplaceTask  bool          // --replace-task: a human at a terminal overriding a conflicting .task (never passed by LETS markdown)
	LinksOnly    bool          // stop after the link step
	PluginRoot   string        // --plugin-root, else CLAUDE_PLUGIN_ROOT
	LockDeadline time.Duration // zero blocks (the CLI); the SessionStart self-heal passes 3s
}

// Adopt makes a worktree someone else created (Orca, a teammate, `git worktree add`)
// a valid LETS worktree: `.lets` and the tracker adapter's declared store links,
// then the task-state file. Locked and idempotent - the orca.yaml setup hook and the
// SessionStart self-heal may both run it. Never calls a tracker or Orca, and never
// deletes a directory.
func Adopt(ctx context.Context, dir string, o AdoptOptions) (*AdoptResult, error) {
	res := &AdoptResult{
		Envelope:   Envelope{SchemaVersion: SchemaVersion, Subcommand: "adopt", Steps: []Step{}},
		StoreLinks: []StoreLink{},
	}
	add := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }
	fail := func(e *Error) (*AdoptResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}

	if o.Task != "" && !taskid.Valid(o.Task) {
		return fail(&Error{Code: ExitUsage, Kind: "task_invalid", Message: fmt.Sprintf("--task %q is not a task id", o.Task)})
	}
	inWt, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	if real, err := filepath.EvalSymlinks(mainRoot); err == nil {
		mainRoot = real
	}
	res.ProjectRoot, res.MainRoot = mainRoot, mainRoot
	if !inWt {
		return fail(&Error{Code: ExitNotLinkedWorktree, Kind: "not_linked_worktree",
			Message:     "this is the main checkout, not a linked worktree",
			Remediation: "run adopt inside the worktree (or pass --dir <worktree>)"})
	}
	wtRoot := gitutil.ProjectRoot(dir, 2*time.Second)
	if wtRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "could not resolve the worktree root"})
	}
	if real, err := filepath.EvalSymlinks(wtRoot); err == nil {
		wtRoot = real
	}
	mainLets := filepath.Join(mainRoot, ".lets")
	if fi, err := os.Stat(mainLets); err != nil || !fi.IsDir() {
		return fail(&Error{Code: ExitSymlinkSourceMissing, Kind: "main_lets_missing",
			Message:     mainLets + " does not exist - this project was never initialized",
			Remediation: "run `lets init` (or /lets:init) in the main checkout first"})
	}

	unlock, err := lockAdopt(mainLets, o.LockDeadline)
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return fail(e)
		}
		return fail(&Error{Code: ExitFilesystem, Kind: "adopt_lock_failed", Message: err.Error(), Cause: err})
	}
	defer unlock()

	home, _ := os.UserHomeDir()
	env := letsconfig.ResolvedEnv(mainRoot, home, func(r string) string { return gitutil.DefaultBranch(r, 2*time.Second) })
	tracker := env["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	pluginRoot, perr := initcmd.DetectPluginRoot(o.PluginRoot)
	if perr != nil {
		pluginRoot = ""
	}
	adapter, reason := trackeradapter.Load(mainRoot, tracker, pluginRoot)
	if reason != "" {
		add(StepWarn, fmt.Sprintf("%s: tracker adapter %q - run /lets:update so it declares its ## Worktree links", reason, tracker))
	}

	out, err := linkShared(ctx, mainRoot, wtRoot, adapter.Links, modeAdopt)
	res.Steps = append(res.Steps, out.Steps...)
	res.MovedAside = out.MovedAside
	if out.StoreLinks != nil {
		res.StoreLinks = out.StoreLinks
	}
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return fail(e)
		}
		return fail(&Error{Code: ExitFilesystem, Kind: "adopt_link_failed", Message: err.Error(), Cause: err})
	}
	excludes := []string{".lets", ".lets.pre-adopt*"}
	for _, sl := range res.StoreLinks {
		if sl.Linked {
			excludes = append(excludes, sl.Path)
		}
	}
	if err := ensureWorktreeExcludes(ctx, mainRoot, excludes); err != nil {
		add(StepWarn, fmt.Sprintf("could not update info/exclude (%s): %v", strings.Join(excludes, ", "), err))
	}
	for _, f := range []struct{ name, kind string }{
		{"tracker-" + tracker + ".md", "adapter_not_in_checkout"},
		{"tracker-" + tracker + ".board.md", "board_not_in_checkout"},
	} {
		mainCopy := filepath.Join(mainRoot, ".claude", "rules", f.name)
		wtCopy := filepath.Join(wtRoot, ".claude", "rules", f.name)
		if _, err := os.Stat(mainCopy); err == nil {
			if _, err := os.Stat(wtCopy); err != nil {
				add(StepWarn, fmt.Sprintf("%s: %s is in the main checkout but not in this worktree's branch, so this worktree's sessions do not load it (commit .claude/rules or restart)", f.kind, f.name))
			}
		}
	}

	branch := currentBranchOf(ctx, wtRoot)
	res.Worktree = &WorktreeInfo{
		Name: filepath.Base(wtRoot), Path: wtRoot, Branch: branch, Kind: "other",
		LetsSymlinked: out.LetsLinked, StoreLinks: res.StoreLinks, StoreLinked: allLinked(res.StoreLinks),
	}
	res.Worktree.BeadsSymlinked = res.Worktree.StoreLinked
	if o.LinksOnly {
		res.OK = true
		return res, nil
	}

	task, e := adoptTask(ctx, mainRoot, wtRoot, branch, tracker, pluginRoot, o, add)
	if e != nil {
		return fail(e)
	}
	res.Task = task
	res.OK = true
	return res, nil
}

// lockAdopt takes <main>/.lets/locks/adopt.lock (blocking, or until deadline).
func lockAdopt(mainLets string, deadline time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Join(mainLets, "locks"), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(mainLets, "locks", "adopt.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if deadline == 0 {
		err = fsutil.LockFile(f)
	} else {
		err = fsutil.TryLockFile(f, time.Now().Add(deadline))
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, fsutil.ErrLockBusy) {
			return nil, &Error{Code: ExitGeneric, Kind: "adopt_lock_busy",
				Message:     "another adopt is running (" + f.Name() + ")",
				Remediation: "/lets:start retries"}
		}
		return nil, err
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

func currentBranchOf(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func headOf(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// adoptTask resolves and records the worktree's task. First hit wins, every id
// through taskid.Valid: --task, the existing .task file, the convention on the
// branch name, the convention on the directory name. A name-derived id is an
// unconfirmed candidate (`origin:`) when it came from an accept: shape or the
// directory; a created shape (branch: / worktree-branch:) needs no marker.
func adoptTask(ctx context.Context, mainRoot, wtRoot, branch, tracker, pluginRoot string, o AdoptOptions, add func(status, msg string)) (*TaskInfo, *Error) {
	slug, ok := taskstate.Slug(branch)
	if !ok {
		add(StepWarn, "detached_head: no branch, so no task-state file is written")
		return nil, nil
	}
	letsDir := filepath.Join(mainRoot, ".lets")
	existing, rerr := taskstate.Read(letsDir, slug)
	hasFile := rerr == nil
	if rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		add(StepWarn, fmt.Sprintf("could not read %s: %v", taskstate.Path(letsDir, slug), rerr))
	}
	if hasFile && existing.Task != "" && !taskid.Valid(existing.Task) {
		add(StepWarn, "the task-state file names an invalid id; ignoring it")
		existing.Task = ""
	}

	var info *TaskInfo
	switch {
	case o.Task != "":
		info = &TaskInfo{ID: o.Task, Source: "argument"}
	case hasFile && existing.Task != "":
		info = &TaskInfo{ID: existing.Task, Source: "task_file", Origin: existing.Origin}
	default:
		conv, _ := trackeradapter.LoadConvention(mainRoot, tracker, pluginRoot)
		if !conv.Declared {
			add(StepSkip, "convention_undeclared: no task id derived from the branch name")
			return nil, nil
		}
		if id, tmpl, ok := conv.ParseBranch(branch, trackeradapter.CreatedAndAccepted); ok && taskid.Valid(id) {
			info = &TaskInfo{ID: id, Source: "branch"}
			if slices.Contains(conv.Accept, tmpl) {
				info.Origin = "branch"
			}
		} else if id, _, ok := conv.ParseBranch(filepath.Base(wtRoot), trackeradapter.CreatedAndAccepted); ok && taskid.Valid(id) {
			info = &TaskInfo{ID: id, Source: "dir", Origin: "dir"}
		} else {
			add(StepWarn, fmt.Sprintf("task_unresolved: %q matches no task branch shape; no task-state written", branch))
			return nil, nil
		}
	}

	if hasFile && existing.Task != "" && existing.Task != info.ID {
		guess := existing.Origin == "branch" || existing.Origin == "dir"
		if !o.ReplaceTask && !(guess && info.Source == "argument") {
			return nil, &Error{Code: ExitTaskFileConflict, Kind: "task_file_conflict",
				Message:     fmt.Sprintf("%s names task %s, not %s", taskstate.Path(letsDir, slug), existing.Task, info.ID),
				Remediation: "claim the task you mean with /lets:start <id>, or pass --replace-task at a terminal"}
		}
	}

	head := headOf(ctx, wtRoot)
	start := head
	if hasFile && existing.Task == info.ID && existing.Start != "" &&
		exec.CommandContext(ctx, "git", "-C", wtRoot, "merge-base", "--is-ancestor", existing.Start, "HEAD").Run() == nil {
		start = existing.Start
	}
	set := map[string]string{"task": info.ID}
	if start != "" {
		set["start"] = start
	}
	switch {
	case info.Source == "argument":
		set["origin"] = "" // an explicit id is confirmed
	case info.Source == "task_file":
		// keep whatever origin the file already carries
	default:
		set["origin"] = info.Origin
	}
	wait := 5 * time.Second
	if o.LockDeadline > 0 {
		wait = o.LockDeadline // the SessionStart self-heal bounds both locks
	}
	if _, err := taskstate.MergeWrite(letsDir, slug, taskstate.WriteOpts{Set: set, Create: true, Deadline: time.Now().Add(wait)}); err != nil {
		if errors.Is(err, taskstate.ErrLockBusy) {
			return nil, &Error{Code: ExitTaskStateLockBusy, Kind: "task_state_lock_busy", Message: err.Error(), Cause: err}
		}
		return nil, &Error{Code: ExitFilesystem, Kind: "task_state_write_failed", Message: err.Error(), Cause: err}
	}
	if info.Source == "argument" {
		info.Origin = ""
	}
	msg := fmt.Sprintf("task %s recorded (source: %s)", info.ID, info.Source)
	if info.Origin != "" {
		msg += fmt.Sprintf("; derived from the %s name - /lets:start confirms it", info.Origin)
	}
	add(StepOK, msg)
	return info, nil
}
