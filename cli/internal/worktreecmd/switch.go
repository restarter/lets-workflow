//go:build unix

package worktreecmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/memberscmd"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// SwitchOptions configures Switch. Task is the task the team worktree moves to;
// Branch names its branch explicitly (default: the task's existing local branch,
// else a new one rendered by BranchName in the created shape, TitleFile giving its
// slug). Park commits the old branch's changes as `wip(<id>): park`; every NEW
// path must be named in Include. Session / Guard are the caller's session id and
// the SessionStart hook's guard, built by the cli layer.
type SwitchOptions struct {
	Task, Branch, TitleFile string
	Park                    bool
	Include                 []string
	FetchTimeout            time.Duration // zero = 20s
	Session                 string
	Guard                   taskstate.SessionGuard
}

// ParkInfo is the park commit Switch made on the branch it left.
type ParkInfo struct {
	Branch string   `json:"branch"`
	Sha    string   `json:"sha"`
	Team   string   `json:"team"`
	Files  []string `json:"files"`
}

// NewPath is a path a park would need an --include for; Source is untracked | staged.
type NewPath struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// SwitchResult is `lets worktree switch`. Unpark is unparked | parked_pushed |
// parked_unverified | parked_kept when the target carried a `park:` key.
type SwitchResult struct {
	Envelope
	Team       string    `json:"team,omitempty"`
	From       string    `json:"from,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	Task       string    `json:"task,omitempty"`
	Created    bool      `json:"created"`
	Base       string    `json:"base,omitempty"`
	BaseStale  bool      `json:"base_stale,omitempty"`
	Park       *ParkInfo `json:"park,omitempty"`
	Unpark     string    `json:"unpark,omitempty"`
	UnparkHead string    `json:"unpark_head,omitempty"`
	NewPaths   []NewPath `json:"new_paths,omitempty"`
}

// Unpark outcomes.
const (
	Unparked         = "unparked"
	ParkedPushed     = "parked_pushed"
	ParkedUnverified = "parked_unverified"
	ParkedKept       = "parked_kept"
)

const defaultFetchTimeout = 20 * time.Second

// Seams for tests: the remote check before an unpark, and the unpark's key delete.
var (
	remotesContainAny = gitutil.RemotesContainAny
	unparkClear       = func(letsDir, slug string) error {
		_, err := taskstate.MergeWrite(letsDir, slug, taskstate.WriteOpts{Set: map[string]string{"park": "", "park_team": ""}, Deadline: time.Now().Add(5 * time.Second)})
		return err
	}
)

var parkSubjectRe = regexp.MustCompile(`^wip(\([^)]*\))?: park$`)

// Switch moves a standing team's worktree to another task's branch. It refuses
// before touching anything: not a team worktree (29), an unmerged index
// (index_unmerged), an operation in progress (operation_in_progress), a detached
// HEAD (detached_head), a live writer working here (32), the merge-branch as target
// (33), a dirty tree without --park (14), a new branch with no origin/<merge> (31),
// a session: line another session holds. A park inventories every new path first
// and refuses any without --include (30) with the index and HEAD untouched; only
// then does it commit. A new branch is cut from origin/<merge> (never the local
// merge-branch) with --no-track. Arriving on a parked branch, the park commit is
// undone with `reset --soft HEAD~1` only when it is HEAD, its subject matches and
// no remote has it. Nothing here stashes.
func Switch(ctx context.Context, dir string, o SwitchOptions) (*SwitchResult, error) {
	res := &SwitchResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "switch", Steps: []Step{}}, Task: o.Task}
	fail := func(e *Error) (*SwitchResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	step := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }
	if !taskid.Valid(o.Task) {
		return fail(&Error{Code: ExitUsage, Kind: "task_invalid", Message: fmt.Sprintf("--task %q is not a task id", o.Task)})
	}
	root := gitutil.ProjectRoot(dir, 2*time.Second)
	if root == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = root
	_, mainRoot := gitutil.DetectInsideWorktreeAt(root)
	letsDir := filepath.Join(mainRoot, ".lets")
	git := func(args ...string) (string, error) {
		out, err := gitRawIn(ctx, root, args...)
		return strings.TrimSpace(string(out)), err
	}
	gitErr := func(what string, err error) *Error {
		return &Error{Code: ExitGitFailed, Kind: "git_failed", Message: what + ": " + err.Error(), Cause: err}
	}

	// 29: only a standing team's worktree switches tasks.
	gitDir, err := git("rev-parse", "--absolute-git-dir")
	if err != nil {
		return fail(gitErr("rev-parse --absolute-git-dir", err))
	}
	team, ok, warns, terr := teamfile.FindByWorktree(filepath.Join(letsDir, "teams"), root, gitDir)
	for _, w := range warns {
		step(StepWarn, w)
	}
	switch {
	case terr != nil:
		return fail(ErrNotTeamWorktree(": " + terr.Error()))
	case !ok:
		return fail(ErrNotTeamWorktree(""))
	}
	res.Team = team

	// An unmerged index is refused outright: a park would commit conflict markers.
	unmerged, err := git("ls-files", "-u")
	if err != nil {
		return fail(gitErr("ls-files -u", err))
	}
	if unmerged != "" {
		return fail(&Error{Code: ExitDirtyWorktree, Kind: "index_unmerged", Message: "the index has unmerged paths; nothing was touched", Remediation: "finish or abort the merge / rebase / cherry-pick first"})
	}
	// An operation in progress would be concluded by the park commit (and a later
	// unpark's reset --soft would drop a merge's second parent): refuse it.
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		p, err := git("rev-parse", "--git-path", marker)
		if err != nil {
			return fail(gitErr("rev-parse --git-path", err))
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if _, err := os.Lstat(p); err == nil {
			return fail(&Error{Code: ExitDirtyWorktree, Kind: "operation_in_progress", Message: marker + " exists - a merge, cherry-pick, revert or rebase is in progress; nothing was touched", Remediation: "finish or abort it first"})
		}
	}
	// A detached HEAD has no branch to park on and no task-state file.
	if currentBranchOf(ctx, root) == "" {
		return fail(&Error{Code: ExitDirtyWorktree, Kind: "detached_head", Message: "HEAD is detached; nothing was touched", Remediation: "switch this worktree to a branch first"})
	}

	// 32: a member working in this tree would see it switch under it.
	if who, e := liveMembersIn(mainRoot, root); e != nil {
		return fail(e)
	} else if who != "" {
		return fail(ErrMembersLive(who))
	}

	from := currentBranchOf(ctx, root)
	res.From = from
	merge := mergeBranch(mainRoot)
	target, created, e := switchTarget(ctx, root, mainRoot, o)
	if e != nil {
		return fail(e)
	}
	res.Branch, res.Created = target, created
	switch target {
	case merge:
		return fail(ErrTargetIsMergeBranch(target))
	case from:
		return fail(&Error{Code: ExitUsage, Kind: "already_on_target", Message: "this worktree is already on " + target})
	}

	dirty, err := git("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fail(gitErr("status", err))
	}
	if dirty != "" && !o.Park {
		return fail(&Error{Code: ExitDirtyWorktree, Kind: "dirty_worktree", Message: "the worktree has changes:\n" + dirty, Remediation: "commit them, or pass --park (with --include for every new path)"})
	}

	base := ""
	if created {
		base, e = remoteBase(ctx, root, gitDir, merge, o.FetchTimeout, res)
		if e != nil {
			return fail(e)
		}
		res.Base = base
	}

	targetSlug, _ := taskstate.Slug(target)
	if o.Session != "" {
		cur, rerr := taskstate.Read(letsDir, targetSlug)
		if err := taskstate.MaySetSession(cur, rerr == nil, o.Session, taskstate.OpRefresh, o.Guard); err != nil {
			var held *taskstate.HeldError
			if errors.As(err, &held) {
				return fail(sessionHeld(target, held))
			}
		}
	}

	if dirty != "" {
		fromState, _ := taskstate.Read(letsDir, slugOf(from))
		park, e := parkChanges(ctx, root, letsDir, from, fromState.Task, team, o.Include, res)
		if e != nil {
			return fail(e)
		}
		res.Park = park
		step(StepOK, fmt.Sprintf("parked %s as %s", from, park.Sha))
	}
	fromState, _ := taskstate.Read(letsDir, slugOf(from))

	if created {
		if _, err := git("switch", "--no-track", "-c", target, base); err != nil {
			return fail(gitErr("switch -c", err))
		}
		step(StepOK, fmt.Sprintf("cut %s from origin/%s (%s)", target, merge, base))
	} else {
		if _, err := git("switch", target); err != nil {
			return fail(gitErr("switch", err))
		}
		step(StepOK, "switched to "+target)
	}
	head, _ := git("rev-parse", "HEAD")

	// One MergeWrite on the target's file: task, start (a new branch starts at its
	// cut; an existing one keeps its own), orc carried, session through the guard.
	_, err = taskstate.MergeWrite(letsDir, targetSlug, taskstate.WriteOpts{
		Create:   true,
		Deadline: time.Now().Add(5 * time.Second),
		Derive: func(cur taskstate.State, exists bool) (map[string]string, error) {
			set := map[string]string{"task": o.Task}
			switch {
			case created:
				set["start"] = base
			case cur.Start == "" || cur.Task != o.Task:
				if mb, err := git("merge-base", "HEAD", "origin/"+merge); err == nil {
					set["start"] = mb
				}
			}
			if cur.Orc == "" && fromState.Orc != "" {
				set["orc"] = fromState.Orc
			}
			if o.Session != "" {
				switch err := taskstate.MaySetSession(cur, exists, o.Session, taskstate.OpRefresh, o.Guard); {
				case err == nil:
					set["session"] = head + " " + o.Session
				case !errors.Is(err, taskstate.ErrSessionUnchanged):
					return nil, err
				}
			}
			return set, nil
		},
	})
	var held *taskstate.HeldError
	switch {
	case errors.As(err, &held):
		e := sessionHeld(target, held)
		e.Message = fmt.Sprintf("switched to %s, task-state not written: the session: line is held (%s) by %s", target, held.Reason, held.Holder)
		return fail(e)
	case err != nil:
		return fail(&Error{Code: ExitFilesystem, Kind: "task_state_failed", Message: "switched to " + target + ", but its task-state was not written: " + err.Error(), Cause: err})
	}
	step(StepOK, "task-state of "+target+" names "+o.Task)

	if !created {
		if e := unpark(ctx, root, letsDir, targetSlug, head, res); e != nil {
			return fail(e)
		}
	}
	res.OK = true
	return res, nil
}

func sessionHeld(target string, held *taskstate.HeldError) *Error {
	return &Error{Code: ExitGeneric, Kind: "session_held", Message: fmt.Sprintf("the session: line of %s is held (%s) by %s", target, held.Reason, held.Holder), Remediation: "switch from the team's lead session"}
}

func slugOf(branch string) string {
	s, _ := taskstate.Slug(branch)
	return s
}

// switchTarget resolves the branch: --branch, else the task's one existing local
// branch in a created or accepted shape, else a new created-shape name.
func switchTarget(ctx context.Context, root, mainRoot string, o SwitchOptions) (string, bool, *Error) {
	exists := func(b string) bool { return revParse(ctx, root, "refs/heads/"+b) != "" }
	if o.Branch != "" {
		if exec.CommandContext(ctx, "git", "check-ref-format", "--branch", o.Branch).Run() != nil {
			return "", false, &Error{Code: ExitUsage, Kind: "branch_name_invalid", Message: fmt.Sprintf("%q is not a valid branch name", o.Branch)}
		}
		return o.Branch, !exists(o.Branch), nil
	}
	if byTask, _, err := branchesByTask(ctx, mainRoot); err == nil {
		switch b := byTask[o.Task]; len(b) {
		case 0:
		case 1:
			return b[0], false, nil
		default:
			return "", false, &Error{Code: ExitUsage, Kind: "branch_ambiguous", Message: fmt.Sprintf("task %s has several branches: %s", o.Task, strings.Join(b, ", ")), Remediation: "pass --branch"}
		}
	}
	bn, err := BranchName(ctx, root, BranchNameOptions{Task: o.Task, TitleFile: o.TitleFile})
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return "", false, e
		}
		return "", false, &Error{Code: ExitUsage, Kind: "branch_render_failed", Message: err.Error(), Cause: err}
	}
	return bn.Branch, !exists(bn.Branch), nil
}

// remoteBase fetches origin/<merge> under the timeout and returns its sha. A failed
// fetch falls back to the existing origin/<merge> with a staleness warning (the age
// of FETCH_HEAD); no origin/<merge> at all is 31 - the local merge-branch is never
// a base.
func remoteBase(ctx context.Context, root, gitDir, merge string, timeout time.Duration, res *SwitchResult) (string, *Error) {
	if timeout <= 0 {
		timeout = defaultFetchTimeout
	}
	fctx, cancel := context.WithTimeout(ctx, timeout)
	_, ferr := gitRawIn(fctx, root, "fetch", "--no-tags", "origin", merge)
	cancel()
	base := revParse(ctx, root, "refs/remotes/origin/"+merge)
	if base == "" {
		return "", ErrNoRemoteBase(merge)
	}
	if ferr != nil {
		res.BaseStale = true
		age := "unknown"
		if fi, err := os.Stat(filepath.Join(gitDir, "FETCH_HEAD")); err == nil {
			age = time.Since(fi.ModTime()).Round(time.Minute).String()
		} else if fi, err := os.Stat(filepath.Join(commonDirOf(ctx, root), "FETCH_HEAD")); err == nil {
			age = time.Since(fi.ModTime()).Round(time.Minute).String()
		}
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: fmt.Sprintf("fetch of origin/%s failed (%v); cutting from the local copy of origin/%s, last fetched %s ago", merge, ferr, merge, age)})
	}
	return base, nil
}

func commonDirOf(ctx context.Context, root string) string {
	out, _ := gitRawIn(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return strings.TrimSpace(string(out))
}

// parkChanges inventories every NEW path first - untracked (not ignored) and staged
// additions / copies / rename targets against HEAD, force-added ones included - and
// refuses any without an --include (30) before the index is touched. Then `git add
// -u` + the includes, one `wip(<id>): park` commit, and `park:` / `park_team:` on
// the old branch's task-state in one write.
func parkChanges(ctx context.Context, root, letsDir, from, oldID, team string, include []string, res *SwitchResult) (*ParkInfo, *Error) {
	listZ := func(args ...string) ([]string, error) {
		out, err := gitRawIn(ctx, root, args...)
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, p := range bytes.Split(out, []byte{0}) {
			if len(p) > 0 {
				paths = append(paths, string(p))
			}
		}
		return paths, nil
	}
	untracked, err := listZ("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, &Error{Code: ExitGitFailed, Kind: "git_failed", Message: "ls-files --others: " + err.Error(), Cause: err}
	}
	staged, err := listZ("diff", "--cached", "--diff-filter=ACR", "--name-only", "-z", "HEAD")
	if err != nil {
		return nil, &Error{Code: ExitGitFailed, Kind: "git_failed", Message: "diff --cached: " + err.Error(), Cause: err}
	}
	source := map[string]string{}
	for _, p := range untracked {
		source[p] = "untracked"
	}
	for _, p := range staged {
		if _, ok := source[p]; !ok {
			source[p] = "staged"
		}
	}
	included := map[string]bool{}
	for _, inc := range include {
		p := inc
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				rel = p
			}
			p = rel
		}
		p = filepath.ToSlash(filepath.Clean(p))
		_, statErr := os.Lstat(filepath.Join(root, p))
		if p == ".." || strings.HasPrefix(p, "../") || filepath.IsAbs(p) || statErr != nil || source[p] == "" {
			return nil, &Error{Code: ExitUsage, Kind: "include_invalid", Message: fmt.Sprintf("--include %q is not an existing new path inside this worktree", inc)}
		}
		included[p] = true
	}
	var gaps []NewPath
	for p, src := range source {
		if !included[p] {
			gaps = append(gaps, NewPath{Path: p, Source: src})
		}
	}
	if len(gaps) > 0 {
		sort.Slice(gaps, func(i, j int) bool { return gaps[i].Path < gaps[j].Path })
		res.NewPaths = gaps
		names := make([]string, len(gaps))
		for i, g := range gaps {
			names[i] = g.Path + " (" + g.Source + ")"
		}
		return nil, ErrUntrackedPresent(strings.Join(names, ", "))
	}

	if _, err := gitRawIn(ctx, root, "add", "-u"); err != nil {
		return nil, &Error{Code: ExitGitFailed, Kind: "git_failed", Message: "add -u: " + err.Error(), Cause: err}
	}
	var incs []string
	for p := range included {
		incs = append(incs, p)
	}
	sort.Strings(incs)
	if len(incs) > 0 {
		if _, err := gitRawIn(ctx, root, append([]string{"--literal-pathspecs", "add", "--"}, incs...)...); err != nil {
			return nil, &Error{Code: ExitGitFailed, Kind: "git_failed", Message: "add the includes: " + err.Error(), Cause: err}
		}
	}
	subject := "wip: park"
	if oldID != "" {
		subject = "wip(" + oldID + "): park"
	}
	if _, err := gitRawIn(ctx, root, "-c", "commit.gpgSign=false", "commit", "--no-verify", "-q", "-m", subject); err != nil {
		return nil, &Error{Code: ExitGitFailed, Kind: "git_failed", Message: "park commit: " + err.Error(), Cause: err}
	}
	sha := revParse(ctx, root, "HEAD")
	filesOut, _ := gitRawIn(ctx, root, "show", "--name-only", "--format=", sha)
	info := &ParkInfo{Branch: from, Sha: sha, Team: team, Files: strings.Fields(string(filesOut))}
	if _, err := taskstate.MergeWrite(letsDir, slugOf(from), taskstate.WriteOpts{
		Set: map[string]string{"park": sha, "park_team": team}, Create: true, Deadline: time.Now().Add(5 * time.Second),
	}); err != nil {
		return info, &Error{Code: ExitFilesystem, Kind: "park_state_failed", Message: "parked " + from + " as " + sha + ", but its task-state was not written: " + err.Error(), Remediation: "add `park: " + sha + "` and `park_team: " + team + "` to " + taskstate.Path(letsDir, slugOf(from)), Cause: err}
	}
	return info, nil
}

// unpark undoes the park commit of the branch just switched to - only when it is
// HEAD, its subject is a park's and no remote has it - then deletes park: and
// park_team: in one write. A pushed or unverified park stays, keys included.
func unpark(ctx context.Context, root, letsDir, slug, head string, res *SwitchResult) *Error {
	st, err := taskstate.Read(letsDir, slug)
	if err != nil || st.Park == "" {
		return nil
	}
	subject, _ := gitRawIn(ctx, root, "log", "-1", "--format=%s", "HEAD")
	if head != st.Park || !parkSubjectRe.MatchString(strings.TrimSpace(string(subject))) {
		res.Unpark = ParkedKept
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "park " + st.Park + " is not this branch's HEAD park commit; left as it is"})
		return nil
	}
	state, remote, rhead, rerr := remotesContainAny(ctx, root, head, 0)
	switch state {
	case gitutil.RemotePushed:
		res.Unpark, res.UnparkHead = ParkedPushed, remote+"/"+rhead
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "park " + head + " is on " + remote + "/" + rhead + "; it stays a commit"})
		return nil
	case gitutil.RemoteUnverified:
		res.Unpark = ParkedUnverified
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: fmt.Sprintf("cannot prove park %s unpushed (%v); it stays a commit", head, rerr)})
		return nil
	}
	if _, err := gitRawIn(ctx, root, "reset", "--soft", "HEAD~1"); err != nil {
		return &Error{Code: ExitGitFailed, Kind: "git_failed", Message: "reset --soft: " + err.Error(), Cause: err}
	}
	res.Unpark = Unparked
	if err := unparkClear(letsDir, slug); err != nil {
		return &Error{Code: ExitFilesystem, Kind: "unpark_state_partial", Message: fmt.Sprintf("the park was undone (reset --soft), but park: and park_team: are still in %s: %v", taskstate.Path(letsDir, slug), err), Remediation: "remove the park: and park_team: lines from " + taskstate.Path(letsDir, slug), Cause: err}
	}
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: "unparked " + head + " (its changes are staged)"})
	return nil
}

// liveMembersIn names every WRITER in this tree - an implementer without isolation
// whose caller_toplevel is this worktree - that is live, rotated or unknown. The
// standing team's read-only members (architect, skeptic, explorer, ...) and an
// isolated implementer never block. A registry that cannot be read refuses (fail
// closed), naming the file; dismissing its members unblocks.
func liveMembersIn(mainRoot, toplevel string) (string, *Error) {
	files, _ := filepath.Glob(filepath.Join(mainRoot, ".lets", "execution", "members-*.json"))
	top := resolvedPath(toplevel)
	var who []string
	for _, f := range files {
		scope := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "members-"), ".json")
		st, err := memberscmd.Status(memberscmd.Options{Root: mainRoot, Scope: scope}, "")
		if err != nil {
			return "", &Error{Code: ExitMembersLive, Kind: "members_live", Message: "members registry " + f + " cannot be judged: " + err.Error(), Remediation: "fix the file, or dismiss its members (lets members dismiss --scope " + scope + " --all)"}
		}
		for _, m := range st.Members {
			if resolvedPath(m.CallerToplevel) != top || m.Role != "lets:implementer" || m.Isolation != "" {
				continue
			}
			if m.Status == memberscmd.StatusLive || m.Status == memberscmd.StatusRotated || m.Status == memberscmd.StatusUnknown {
				who = append(who, fmt.Sprintf("%s/%s (%s)", scope, m.Name, m.Status))
			}
		}
	}
	return strings.Join(who, ", "), nil
}

func resolvedPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// gitRawIn runs git in dir; the error carries stderr.
func gitRawIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
