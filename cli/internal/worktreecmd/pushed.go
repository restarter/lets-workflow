//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// PushedOptions configures Pushed. Branch only names the branch in the message;
// every configured remote is checked.
type PushedOptions struct {
	Commit  string
	Branch  string
	Timeout time.Duration // zero = gitutil.DefaultRemoteTimeout
}

// PushedResult is `lets worktree pushed`: whether Commit is on ANY branch of ANY
// configured remote (Remote names the one that has it). State is pushed | not_pushed | unverified; only not_pushed permits a
// history rewrite. Reason names why a state is unverified.
type PushedResult struct {
	Envelope
	Commit string `json:"commit"`
	Branch string `json:"branch,omitempty"`
	Remote string `json:"remote,omitempty"`
	State  string `json:"state"`
	Head   string `json:"head,omitempty"`
	Reason string `json:"reason,omitempty"`
}

var pushedCommitRe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// Pushed answers through gitutil.RemotesContainAny - the one owner of "is this
// commit on the remote" - before a rewrite of local history (the pipelined
// autosquash). Every state exits 0: the caller reads state, and anything but
// not_pushed stops its rewrite.
func Pushed(ctx context.Context, dir string, o PushedOptions) (*PushedResult, error) {
	res := &PushedResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "pushed", Steps: []Step{}}, Commit: o.Commit, Branch: o.Branch}
	fail := func(e *Error) (*PushedResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if !pushedCommitRe.MatchString(o.Commit) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: fmt.Sprintf("--commit %q is not a commit sha (7-64 hex)", o.Commit)})
	}
	root := gitutil.ProjectRoot(dir, 2*time.Second)
	if root == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = root
	state, remote, head, err := gitutil.RemotesContainAny(ctx, root, o.Commit, o.Timeout)
	res.State, res.Remote, res.Head, res.OK = state, remote, head, true
	switch {
	case errors.Is(err, gitutil.ErrTooManyHeads):
		res.Reason = "too_many_heads"
	case err != nil:
		res.Reason = "git_error" // a failed or timed-out git call, or a tip still missing
	}
	msg := fmt.Sprintf("%s on the remotes: %s", o.Commit, state)
	if head != "" {
		msg += " (" + remote + "/" + head + ")"
	}
	if err != nil {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: msg + " - " + err.Error()})
	} else {
		res.Steps = append(res.Steps, Step{Status: StepOK, Message: msg})
	}
	return res, nil
}

// RenderPushed is the human rendering of `lets worktree pushed`.
func RenderPushed(w io.Writer, res *PushedResult) {
	line := res.State
	if res.Head != "" {
		line += " (" + res.Remote + "/" + res.Head + ")"
	}
	if res.Reason != "" {
		line += " - " + res.Reason
	}
	fmt.Fprintln(w, line)
}
