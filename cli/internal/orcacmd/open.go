//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// OpenOptions configures Open.
type OpenOptions struct {
	Repo   string // the main checkout (absolute)
	Name   string // Orca worktree name; Orca turns `/` into `-`, so callers pass a `/`-free name
	Prompt string // first message for the agent (passed as a command-line argument by Orca)
	Force  bool   // open even when a worktree with this name or branch is already open
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
	args := []string{"worktree", "create", "--repo", "path:" + o.Repo, "--name", o.Name, "--no-parent", "--agent", "claude"}
	if o.Prompt != "" {
		args = append(args, "--prompt", o.Prompt)
	}
	out, f := c.Run(ctx, "worktree create", append(args, "--json")...)
	if f != nil {
		return degrade(f)
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
		return degrade(&Failure{Reason: ReasonOutputUnrecognized, Verb: "worktree create"})
	}
	w := env.Result.Worktree
	launch.Launched, launch.Created = true, true
	launch.Path, launch.OrcaWorktreeID = w.Path, w.ID
	launch.Branch = strings.TrimPrefix(w.Branch, "refs/heads/")
	launch.FallbackCommand = ""
	add(StepOK, fmt.Sprintf("orca worktree %s created at %s (branch %s)", o.Name, w.Path, launch.Branch))
	return res, nil
}
