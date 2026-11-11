//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// OpenOptions configures Open.
type OpenOptions struct {
	Repo         string        // the main checkout (absolute)
	Name         string        // Orca worktree name; Orca turns `/` into `-`, so callers pass a `/`-free name
	Prompt       string        // first message for the agent (passed as a command-line argument by Orca)
	Force        bool          // open even when a worktree with this name or branch is already open
	Merge        string        // LETS_MERGE_BRANCH; a new branch gets the resolved base (lets-qjt3h)
	FetchTimeout time.Duration // 0 = 20 s
	NoAgent      bool          // Orca creates the worktree only: --setup skip, no agent, no prompt - the caller adopts, then opens a terminal (the team-create pattern, lets-41cpf)
}

// worktreeListProbe lists git's worktree paths; a test seam.
var worktreeListProbe = gitWorktreeList

// gitWorktreeList returns the `worktree ` paths of `git worktree list --porcelain`
// and reports its own failure - unlike gitWorktreePaths, an error never reads as
// "nothing created".
func gitWorktreeList(ctx context.Context, repo string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// Open creates an Orca worktree running Claude. Orca unavailable or refusing is not
// an error: Launched=false with a reason and a fallback command.
func Open(ctx context.Context, o OpenOptions) (*OpenResult, error) {
	res := &OpenResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "open", Steps: []Step{}}}
	add := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }
	fail := func(e *Error) (*OpenResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if o.Name == "" {
		return fail(&Error{Code: ExitUsage, Kind: "name_missing", Message: "--name is required"})
	}
	if o.NoAgent && o.Prompt != "" {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--prompt needs an agent; drop --no-agent or --prompt"})
	}
	if fi, err := os.Stat(o.Repo); o.Repo == "" || err != nil || !fi.IsDir() {
		return fail(&Error{Code: ExitRepoInvalid, Kind: "repo_invalid", Message: fmt.Sprintf("--repo %q is not a directory", o.Repo)})
	}
	if inWt, main := gitutil.DetectInsideWorktreeAt(o.Repo); inWt || main == "" || !fsutil.SameDir(main, o.Repo) {
		return fail(&Error{Code: ExitRepoInvalid, Kind: "repo_invalid", Message: fmt.Sprintf("--repo %q is not a main checkout", o.Repo)})
	}
	launch := &LaunchInfo{WorkspaceName: o.Name, FallbackCommand: fmt.Sprintf("/lets:worktree create %s --no-orca", o.Name)}
	res.Launch = launch
	res.OK = true
	degrade := func(f *Failure) (*OpenResult, error) {
		launch.Reason = f.Reason
		add(StepWarn, fmt.Sprintf("orca unavailable (%s) - falling back", f.Error()))
		return res, nil
	}

	c, f := NewClient()
	if f != nil {
		return degrade(f)
	}
	if _, f := c.Status(ctx); f != nil {
		return degrade(f)
	}
	if !o.Force {
		rows, f := c.Ps(ctx)
		if f != nil {
			return degrade(f)
		}
		for _, r := range rows {
			if r.DisplayName == o.Name || strings.TrimPrefix(r.Branch, "refs/heads/") == o.Name {
				launch.Reason = "already_open"
				launch.Path = r.Path
				launch.OrcaWorktreeID = r.WorktreeID
				add(StepSkip, fmt.Sprintf("an Orca worktree named %q is already open at %s", o.Name, r.Path))
				return res, nil
			}
		}
	}
	args := []string{"worktree", "create", "--repo", "path:" + o.Repo, "--name", o.Name, "--no-parent"}
	if o.NoAgent {
		args = append(args, "--setup", "skip")
	} else {
		args = append(args, "--agent", "claude")
	}
	// lets-qjt3h (MAIN-LETS Q5): a NEW branch gets the one base policy - Orca's own
	// default may be the stale local merge-branch. An existing branch is left to
	// Orca as before. A refusal degrades with the kind as the reason: the fallback
	// Go create reports it (31) with its fix.
	if o.Merge != "" && gitIn(ctx, o.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+o.Name) == "" {
		br, refusal := gitutil.ResolveBase(ctx, o.Repo, o.Merge, gitutil.BaseOptions{FetchTimeout: o.FetchTimeout})
		if refusal != nil {
			launch.Reason = refusal.Kind
			add(StepWarn, "base not resolved ("+refusal.Message+") - falling back")
			return res, nil
		}
		for _, n := range br.Notes {
			add(StepWarn, n)
		}
		args = append(args, "--base-branch", br.Ref)
		launch.BaseRef, launch.BaseResolution = br.Ref, br
	}
	if o.Prompt != "" && !o.NoAgent {
		args = append(args, "--prompt", o.Prompt)
	}
	// stop is a --no-agent refusal: no fallback, which would make a second checkout or
	// start a second agent; the worktree, if any, stays for the owner.
	stop := func(e *Error) (*OpenResult, error) {
		launch.Launched, launch.FallbackCommand = false, ""
		return fail(e)
	}
	// reconcile looks at git before degrading a failed --no-agent create: a fallback
	// is safe only when nothing was created.
	reconcile := func(f *Failure) (*OpenResult, error) {
		if !o.NoAgent {
			return degrade(f)
		}
		paths, err := worktreeListProbe(ctx, o.Repo)
		if err != nil {
			return stop(&Error{Code: ExitCreateAmbiguous, Kind: "create_ambiguous", Message: fmt.Sprintf("Orca reported %s and git could not be asked: %v", f.Error(), err), Remediation: "check `orca worktree ps` and `git worktree list`; nothing is retried, no Go worktree is created"})
		}
		for _, p := range paths {
			if filepath.Base(p) == o.Name {
				launch.Path = p
				return stop(&Error{Code: ExitCreateAmbiguous, Kind: "create_ambiguous", Message: fmt.Sprintf("Orca reported %s, but git shows a worktree at %s; nothing is retried, no Go worktree is created", f.Error(), p), Remediation: "inspect " + p + " (`orca worktree ps`, `git worktree list`) and remove it by hand if it is not wanted"})
			}
		}
		return degrade(f)
	}
	out, f := c.Run(ctx, "worktree create", append(args, "--json")...)
	if f != nil {
		return reconcile(f)
	}
	var env struct {
		Result *struct {
			Worktree *struct {
				ID     string `json:"id"`
				Path   string `json:"path"`
				Branch string `json:"branch"`
			} `json:"worktree"`
		} `json:"result"`
	}
	if f := DecodeJSON("worktree create", out, &env); f != nil || env.Result == nil || env.Result.Worktree == nil {
		return reconcile(&Failure{Reason: ReasonOutputUnrecognized, Verb: "worktree create"})
	}
	w := env.Result.Worktree
	launch.Launched, launch.Created = true, true
	launch.Path, launch.OrcaWorktreeID = w.Path, w.ID
	launch.Branch = strings.TrimPrefix(w.Branch, "refs/heads/")
	if launch.BaseResolution != nil {
		head := gitIn(ctx, w.Path, "rev-parse", "HEAD")
		if head == "" && launch.Branch != "" {
			head = gitIn(ctx, o.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+launch.Branch)
		}
		// Two HEADs keep the resolution: the resolved commit itself (Orca cut exactly
		// it, even if the ref moved after the cut), or the tip of the ref Orca was given,
		// read after the create (Orca fetched and moved it; reconciled below). A HEAD
		// cut from anything else is not that resolution and is never labelled with it
		// (c5-F1).
		tip := gitIn(ctx, o.Repo, "rev-parse", "--verify", "--quiet", launch.BaseResolution.Ref+"^{commit}")
		switch {
		case head == "":
			add(StepWarn, "the created worktree's HEAD could not be read - base_resolution omitted; base_ref is what Orca was given")
			launch.BaseResolution = nil
		case head != tip && head != launch.BaseResolution.Sha:
			add(StepWarn, fmt.Sprintf("Orca cut %s from %s, which is not %s - base_resolution omitted", o.Name, head, launch.BaseResolution.Ref))
			launch.BaseResolution = nil
		}
		reconcileBase(launch.BaseResolution, head, o.Name, add)
	}
	launch.FallbackCommand = ""
	if o.NoAgent {
		// An agent running before LETS linked the worktree would act unlinked.
		terms, tf := c.Terminals(ctx)
		if tf != nil {
			add(StepWarn, "terminal list failed ("+tf.Error()+"): the worktree's open terminals are unverified")
		}
		var agents []string
		for _, t := range terms {
			if fsutil.SameDir(t.Path, launch.Path) && (t.AgentType != "" || t.State != "") {
				agents = append(agents, fmt.Sprintf("%s (%s %s)", t.Title, t.AgentType, t.State))
			}
		}
		if len(agents) > 0 {
			return stop(&Error{Code: ExitAgentTerminalPresent, Kind: "agent_terminal_present", Message: fmt.Sprintf("Orca started an agent in %s before LETS linked it: %s; the worktree stays", o.Name, strings.Join(agents, ", "))})
		}
	}
	add(StepOK, fmt.Sprintf("orca worktree %s created at %s (branch %s)", o.Name, w.Path, launch.Branch))
	return res, nil
}
