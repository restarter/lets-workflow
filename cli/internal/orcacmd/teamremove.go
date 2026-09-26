//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// Team worktree removal states (TeamRemoveInfo.State).
const (
	TeamRemoveNotAttempted = "not_attempted" // Orca absent, not running, or its ps failed - Go never removes an Orca worktree
	TeamNotListed          = "not_listed"
	TeamRefused            = "refused"
	TeamRemoved            = "removed"
	TeamArchiveHookFailed  = "archive_hook_failed"
	TeamRemoveAmbiguous    = "ambiguous"
)

// TeamRemoveOptions configures TeamRemove; Name is `team_<callsign>`.
type TeamRemoveOptions struct {
	Repo, Name string
}

// TeamRemoveInfo is what TeamRemove did.
type TeamRemoveInfo struct {
	Attempted bool   `json:"attempted"`
	Path      string `json:"path,omitempty"`
	Branch    string `json:"branch,omitempty"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
}

// TeamRemoveResult is the team-remove envelope.
type TeamRemoveResult struct {
	Envelope
	Team *TeamRemoveInfo `json:"team,omitempty"`
}

// remoteCheck is the unpushed net (a seam for tests).
var remoteCheck = gitutil.RemotesContainAny

// TeamRemove removes an Orca-created team worktree THROUGH Orca: `orca worktree rm
// --run-hooks`, never forced and never waiving a failed archive hook. LETS's own nets
// run first, each a separate command in the worktree: no uncommitted or untracked
// change, and HEAD proven on a remote. Orca unavailable is not_attempted, exit 0 - the
// caller stops; Go never removes an Orca worktree. The outcome is classified from
// both sides (git's worktree list, Orca's ps) and never retried.
func TeamRemove(ctx context.Context, o TeamRemoveOptions) (*TeamRemoveResult, error) {
	res := &TeamRemoveResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "team-remove", Steps: []Step{}}}
	add := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }
	fail := func(e *Error) (*TeamRemoveResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if !validTeamName(o.Name) {
		return fail(&Error{Code: ExitUsage, Kind: "name_invalid", Message: fmt.Sprintf("--name %q is not a team worktree name (team_<callsign>)", o.Name)})
	}
	repo, e := resolveMainCheckout(o.Repo)
	if e != nil {
		return fail(e)
	}
	info := &TeamRemoveInfo{State: TeamRemoveNotAttempted}
	res.Team, res.OK = info, true
	notAttempted := func(reason string, f *Failure) (*TeamRemoveResult, error) {
		info.Reason = reason
		add(StepWarn, fmt.Sprintf("orca not used (%s) - nothing was removed; Go never removes an Orca worktree", f.Error()))
		return res, nil
	}
	c, f := NewClient()
	if f != nil {
		return notAttempted(f.Reason, f)
	}
	if _, f := c.Status(ctx); f != nil {
		return notAttempted(f.Reason, f)
	}
	rows, f := c.Ps(ctx)
	if f != nil {
		return notAttempted("ps_failed", f)
	}
	row, ok := psRow(rows, o.Name)
	if !ok {
		info.State = TeamNotListed
		return fail(&Error{Code: ExitNotListed, Kind: "not_listed", Message: fmt.Sprintf("Orca lists no worktree named %s - a Go-created team worktree goes through /lets:worktree remove", o.Name)})
	}
	info.Path, info.Branch = row.Path, strings.TrimPrefix(row.Branch, "refs/heads/")

	// Nets, each a separate command in the worktree.
	refuse := func(e *Error) (*TeamRemoveResult, error) {
		info.State, info.Reason = TeamRefused, e.Kind
		return fail(e)
	}
	status, err := gitRaw(ctx, info.Path, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return refuse(&Error{Code: ExitDirtyWorktree, Kind: "dirty_worktree", Message: "cannot read the worktree's status: " + err.Error()})
	}
	if s := strings.TrimSpace(status); s != "" {
		return refuse(&Error{Code: ExitDirtyWorktree, Kind: "dirty_worktree", Message: "the team worktree has changes:\n" + s, Remediation: "commit or remove them in the team session; nothing is forced"})
	}
	head := gitIn(ctx, info.Path, "rev-parse", "HEAD")
	state, _, _, rerr := remoteCheck(ctx, info.Path, head, 0)
	if state != gitutil.RemotePushed {
		msg := fmt.Sprintf("HEAD %s is %s", head, state)
		if rerr != nil {
			msg += " (" + rerr.Error() + ")"
		}
		return refuse(&Error{Code: ExitUnpushedCommits, Kind: "unpushed_commits", Message: msg, Remediation: "push the team branch, or finish its work in the team session; nothing is forced"})
	}

	info.Attempted = true
	rctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	_, rf := c.Run(rctx, "worktree rm", "worktree", "rm", "--worktree", "path:"+info.Path, "--run-hooks", "--json")
	cancel()
	if rf != nil && strings.Contains(rf.Detail, "worktree_archive_hook_failed") {
		info.State, info.Reason = TeamArchiveHookFailed, "archive_hook_failed"
		return fail(&Error{Code: ExitArchiveHookFailed, Kind: "archive_hook_failed", Message: "Orca's archive hook failed; the worktree stays: " + rf.Detail, Remediation: "fix the hook, then disband again; the hook is never waived"})
	}
	if rf != nil {
		add(StepWarn, "orca worktree rm reported: "+rf.Error()+" - classifying from git and Orca")
	}

	gitHas := false
	for _, p := range gitWorktreePaths(ctx, repo) {
		if fsutil.SameDir(p, info.Path) {
			gitHas = true
		}
	}
	rows, pf := c.Ps(ctx)
	orcaHas := pf != nil
	if pf == nil {
		_, orcaHas = psRow(rows, o.Name)
	}
	if !gitHas && !orcaHas {
		info.State = TeamRemoved
		add(StepOK, fmt.Sprintf("orca removed %s (%s); its archive hook ran", o.Name, info.Path))
		return res, nil
	}
	info.State, info.Reason = TeamRemoveAmbiguous, "remove_ambiguous"
	var still []string
	if gitHas {
		still = append(still, "git still lists "+info.Path)
	}
	switch {
	case pf != nil:
		still = append(still, "Orca's ps failed ("+pf.Error()+")")
	case orcaHas:
		still = append(still, "Orca still lists "+o.Name)
	}
	return fail(&Error{Code: ExitRemoveAmbiguous, Kind: "remove_ambiguous", Message: "after orca worktree rm: " + strings.Join(still, "; "), Remediation: "check `orca worktree ps` and `git worktree list`; nothing is retried"})
}

// gitRaw runs git in dir and returns its raw stdout.
func gitRaw(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}

// RenderTeamRemove writes a human-readable summary of a team-remove result.
func RenderTeamRemove(w io.Writer, env Envelope, t *TeamRemoveInfo) {
	if env.Error != nil {
		fmt.Fprintf(w, "Error: %s: %s\n", env.Error.Kind, env.Error.Message)
		return
	}
	if t == nil {
		return
	}
	if t.State == TeamRemoved {
		fmt.Fprintf(w, "Orca removed %s\n", t.Path)
		return
	}
	fmt.Fprintf(w, "Orca not used (%s) - nothing was removed\n", t.Reason)
}
