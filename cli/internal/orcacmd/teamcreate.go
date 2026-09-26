//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// Team worktree states (TeamCreateInfo.State).
const (
	TeamNotAttempted  = "not_attempted" // Orca absent, not running, or its ps failed - the ONLY state the caller may answer with a Go create
	TeamCreated       = "created"
	TeamAlreadyExists = "already_exists"
	TeamAmbiguous     = "ambiguous"
	TeamFailed        = "failed"
)

// TeamCreateOptions configures TeamCreate. Name is `team_<callsign>`; BaseBranch is
// `origin/<merge>` - Orca resolves a bare branch name as the LOCAL ref.
type TeamCreateOptions struct {
	Repo, Name, BaseBranch string
}

// TerminalBrief is one Orca terminal already open in the new worktree.
type TerminalBrief struct {
	Handle    string `json:"handle"`
	Title     string `json:"title"`
	AgentType string `json:"agent_type,omitempty"`
	State     string `json:"state,omitempty"`
}

// TeamCreateInfo is what TeamCreate did.
type TeamCreateInfo struct {
	Attempted      bool            `json:"attempted"`
	Created        bool            `json:"created"`
	Path           string          `json:"path,omitempty"`
	Branch         string          `json:"branch,omitempty"`
	Head           string          `json:"head,omitempty"`
	OrcaWorktreeID string          `json:"orca_worktree_id,omitempty"`
	Terminals      []TerminalBrief `json:"terminals"`
	State          string          `json:"state"`
	Reason         string          `json:"reason,omitempty"`
}

// TeamCreateResult is the team-create envelope.
type TeamCreateResult struct {
	Envelope
	Team *TeamCreateInfo `json:"team,omitempty"`
}

var worktreeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// validTeamName: `team_<callsign>` under the worktree-name rule.
func validTeamName(n string) bool {
	return strings.HasPrefix(n, "team_") && len(n) > len("team_") && worktreeNameRe.MatchString(n) && !strings.Contains(n, "..") && !strings.HasSuffix(n, ".lock")
}

// mainCheckout resolves --repo (default: the main checkout of the cwd) and checks it
// is a main checkout, as Open does.
func resolveMainCheckout(repo string) (string, *Error) {
	if repo == "" {
		cwd, _ := os.Getwd()
		_, repo = gitutil.DetectInsideWorktreeAt(cwd)
	}
	if fi, err := os.Stat(repo); repo == "" || err != nil || !fi.IsDir() {
		return "", &Error{Code: ExitRepoInvalid, Kind: "repo_invalid", Message: fmt.Sprintf("--repo %q is not a directory", repo)}
	}
	if inWt, main := gitutil.DetectInsideWorktreeAt(repo); inWt || main == "" || !fsutil.SameDir(main, repo) {
		return "", &Error{Code: ExitRepoInvalid, Kind: "repo_invalid", Message: fmt.Sprintf("--repo %q is not a main checkout", repo)}
	}
	return repo, nil
}

// psRow finds the Orca worktree named name (display name, or branch without refs/heads/).
func psRow(rows []PsWorktree, name string) (PsWorktree, bool) {
	for _, r := range rows {
		if r.DisplayName == name || strings.TrimPrefix(r.Branch, "refs/heads/") == name {
			return r, true
		}
	}
	return PsWorktree{}, false
}

// gitWorktreePaths lists the worktree paths git registers for repo.
func gitWorktreePaths(ctx context.Context, repo string) []string {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, l := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(l, "worktree "); ok {
			paths = append(paths, p)
		}
	}
	return paths
}

func gitIn(ctx context.Context, dir string, args ...string) string {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TeamCreate creates a standing team's worktree THROUGH Orca, so Orca registers it
// and can attach the lead's terminal: `orca worktree create --base-branch origin/<merge>
// --setup skip --no-parent`, with no agent and no prompt. Orca unavailable (or its ps
// failing before any create) is not_attempted, exit 0 - the only state the caller may
// answer with a Go create. Once a create was attempted the outcome is classified from
// both sides (Orca's ps and git's worktree list) and never degrades to not_attempted;
// Go never retries it. A created worktree is verified (name, branch, HEAD == base) and
// its terminals listed: a tracked agent already running there is refused, before the
// setup hook could run. The live probe (2026-09-26) opened one plain shell
// ("Terminal 1") and left .lets unlinked - the caller runs `lets worktree adopt`.
func TeamCreate(ctx context.Context, o TeamCreateOptions) (*TeamCreateResult, error) {
	res := &TeamCreateResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "team-create", Steps: []Step{}}}
	add := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }
	fail := func(e *Error) (*TeamCreateResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if !validTeamName(o.Name) {
		return fail(&Error{Code: ExitUsage, Kind: "name_invalid", Message: fmt.Sprintf("--name %q is not a team worktree name (team_<callsign>)", o.Name)})
	}
	if !strings.HasPrefix(o.BaseBranch, "origin/") || len(o.BaseBranch) == len("origin/") {
		return fail(&Error{Code: ExitUsage, Kind: "base_invalid", Message: fmt.Sprintf("--base-branch %q must be origin/<branch> - Orca resolves a bare name as the local ref", o.BaseBranch)})
	}
	repo, e := resolveMainCheckout(o.Repo)
	if e != nil {
		return fail(e)
	}
	info := &TeamCreateInfo{Terminals: []TerminalBrief{}, State: TeamNotAttempted}
	res.Team, res.OK = info, true
	notAttempted := func(reason string, f *Failure) (*TeamCreateResult, error) {
		info.Reason = reason
		add(StepWarn, fmt.Sprintf("orca not used (%s) - no create was attempted", f.Error()))
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
	if r, ok := psRow(rows, o.Name); ok {
		info.State, info.Path, info.OrcaWorktreeID = TeamAlreadyExists, r.Path, r.WorktreeID
		info.Branch = strings.TrimPrefix(r.Branch, "refs/heads/")
		add(StepSkip, fmt.Sprintf("an Orca worktree named %s already exists at %s", o.Name, r.Path))
		return res, nil
	}

	info.Attempted = true
	args := []string{"worktree", "create", "--repo", "path:" + repo, "--name", o.Name, "--base-branch", o.BaseBranch, "--setup", "skip", "--no-parent", "--json"}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	_, cf := c.Run(cctx, "worktree create", args...)
	cancel()
	if cf != nil {
		add(StepWarn, "orca worktree create reported: "+cf.Error()+" - classifying from Orca and git")
	}

	// Classify from both sides, whatever the create printed.
	gitPath := ""
	for _, p := range gitWorktreePaths(ctx, repo) {
		if filepath.Base(p) == o.Name {
			gitPath = p
		}
	}
	rows, pf := c.Ps(ctx)
	row, listed := PsWorktree{}, false
	if pf == nil {
		row, listed = psRow(rows, o.Name)
	}
	switch {
	case listed && gitPath != "" && fsutil.SameDir(row.Path, gitPath):
		info.Path, info.OrcaWorktreeID = gitPath, row.WorktreeID
	case gitPath == "" && !listed && pf == nil:
		info.State, info.Reason = TeamFailed, "create_failed"
		return fail(&Error{Code: ExitCreateFailed, Kind: "create_failed", Message: fmt.Sprintf("orca worktree create for %s left no worktree in Orca or git", o.Name), Remediation: "check `orca worktree ps` and `git worktree list`; nothing is retried"})
	default:
		info.State, info.Reason, info.Path = TeamAmbiguous, "create_ambiguous", gitPath
		orcaSide := "not listed"
		switch {
		case pf != nil:
			orcaSide = "unknown (" + pf.Error() + ")"
		case listed:
			orcaSide = "listed at " + row.Path
		}
		gitSide := "no worktree " + o.Name
		if gitPath != "" {
			gitSide = "a worktree at " + gitPath
		}
		return fail(&Error{Code: ExitCreateAmbiguous, Kind: "create_ambiguous", Message: fmt.Sprintf("after the create, git shows %s and Orca shows %s", gitSide, orcaSide), Remediation: "check `orca worktree ps` and `git worktree list`; nothing is retried, no Go worktree is created"})
	}

	// Verify the created worktree before anything runs in it.
	info.Branch = gitIn(ctx, info.Path, "branch", "--show-current")
	info.Head = gitIn(ctx, info.Path, "rev-parse", "HEAD")
	base := gitIn(ctx, repo, "rev-parse", "--verify", "--quiet", o.BaseBranch+"^{commit}")
	var mismatch []string
	if filepath.Base(info.Path) != o.Name {
		mismatch = append(mismatch, fmt.Sprintf("path %s (want basename %s)", info.Path, o.Name))
	}
	if info.Branch != o.Name {
		mismatch = append(mismatch, fmt.Sprintf("branch %q (want %s)", info.Branch, o.Name))
	}
	if info.Head == "" || info.Head != base {
		mismatch = append(mismatch, fmt.Sprintf("HEAD %s (want %s = %s)", info.Head, o.BaseBranch, base))
	}
	if len(mismatch) > 0 {
		info.State, info.Reason = TeamFailed, "team_worktree_mismatch"
		return fail(&Error{Code: ExitTeamWorktreeMismatch, Kind: "team_worktree_mismatch", Message: "the Orca worktree is not the expected team worktree: " + strings.Join(mismatch, "; ") + "; it stays, nothing is renamed or reset"})
	}

	// Terminal preflight: a tracked agent already running there would act before
	// the setup hook. A plain shell passes; a non-agent command a repo default starts
	// cannot be seen here.
	terms, tf := c.Terminals(ctx)
	if tf != nil {
		add(StepWarn, "terminal list failed ("+tf.Error()+"): the worktree's open terminals are unverified")
	}
	var agents []string
	for _, t := range terms {
		if !fsutil.SameDir(t.Path, info.Path) {
			continue
		}
		info.Terminals = append(info.Terminals, TerminalBrief{Handle: t.Handle, Title: t.Title, AgentType: t.AgentType, State: t.State})
		if t.AgentType != "" || t.State != "" {
			agents = append(agents, fmt.Sprintf("%s (%s %s)", t.Title, t.AgentType, t.State))
		}
	}
	if len(agents) > 0 {
		info.State, info.Reason = TeamFailed, "agent_terminal_present"
		return fail(&Error{Code: ExitAgentTerminalPresent, Kind: "agent_terminal_present", Message: "Orca started an agent in the new team worktree before its setup: " + strings.Join(agents, ", ") + "; the worktree stays"})
	}
	info.State, info.Created = TeamCreated, true
	add(StepOK, fmt.Sprintf("orca created %s at %s (branch %s, HEAD %s = %s); %d plain terminal(s) open - a command a repo default starts there cannot be seen", o.Name, info.Path, info.Branch, info.Head, o.BaseBranch, len(info.Terminals)))
	return res, nil
}

// RenderTeam writes a human-readable summary of a team-create result.
func RenderTeam(w io.Writer, env Envelope, t *TeamCreateInfo) {
	if env.Error != nil {
		fmt.Fprintf(w, "Error: %s: %s\n", env.Error.Kind, env.Error.Message)
		return
	}
	if t == nil {
		return
	}
	switch t.State {
	case TeamCreated:
		fmt.Fprintf(w, "Orca created team worktree %s (branch %s, HEAD %s)\n", t.Path, t.Branch, t.Head)
	case TeamAlreadyExists:
		fmt.Fprintf(w, "An Orca worktree of that name already exists at %s\n", t.Path)
	default:
		fmt.Fprintf(w, "Orca not used (%s) - no create was attempted\n", t.Reason)
	}
}
