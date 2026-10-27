//go:build unix

package worktreecmd

import (
	"context"
	"fmt"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// BaseOptions configures Base; FetchTimeout 0 = 20 s.
type BaseOptions struct{ FetchTimeout time.Duration }

// BaseResult is `lets worktree base`: the base a new branch would be cut from now.
type BaseResult struct {
	Envelope
	BaseRef        string                  `json:"base_ref,omitempty"`
	BaseResolution *gitutil.BaseResolution `json:"base_resolution,omitempty"`
}

// Base resolves - fetch included - the base a new task branch is cut from and cuts
// nothing: the one spec-facing read of the policy create and switch apply
// themselves (gitutil.ResolveBase, lets-qjt3h). /lets:team run reads it once per
// run and passes its sha to every worker's create as --base.
func Base(ctx context.Context, dir string, o BaseOptions) (*BaseResult, error) {
	res := &BaseResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "base", Steps: []Step{}}}
	fail := func(e *Error) (*BaseResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	root := gitutil.ProjectRoot(dir, 2*time.Second)
	if root == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = root
	_, mainRoot := gitutil.DetectInsideWorktreeAt(root)
	merge := mergeBranch(mainRoot)
	br, refusal := gitutil.ResolveBase(ctx, root, merge, gitutil.BaseOptions{FetchTimeout: o.FetchTimeout})
	if refusal != nil {
		return fail(ErrBaseRefusal(refusal, merge, false))
	}
	for _, n := range br.Notes {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: n})
	}
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("base %s (%s)", br.Ref, br.Sha)})
	res.OK, res.BaseRef, res.BaseResolution = true, br.Ref, br
	return res, nil
}
