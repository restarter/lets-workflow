//go:build unix

package orcacmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// Failure reasons.
const (
	ReasonNotFound           = "orca_not_found"
	ReasonAppNotRunning      = "orca_app_not_running"
	ReasonStatusUnrecognized = "orca_status_unrecognized"
	ReasonCapabilityMissing  = "orca_capability_missing"
	ReasonOutputUnrecognized = "orca_output_unrecognized"
	ReasonHandleStale        = "orca_handle_stale"
	ReasonError              = "orca_error"
	ReasonEnvAbsent          = "orca_env_absent"
	ReasonEnvMismatch        = "orca_env_mismatch"
	ReasonWorktreeNotFound   = "worktree_not_found"
	ReasonNotEnabled         = "orca_not_enabled"
	defaultVerbTimeout       = 15 * time.Second
	detailCap                = 300
)

// bundlePaths lists the app-bundle CLI locations, preferred over PATH.
var bundlePaths = func() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"/Applications/Orca.app/Contents/Resources/bin/orca",
		filepath.Join(home, "Applications", "Orca.app", "Contents", "Resources", "bin", "orca"),
	}
}

var lookPath = exec.LookPath

// lookOrca resolves the Orca CLI: the app bundle first, PATH last. On the dev machine
// (2026-09-14) /usr/local/bin/orca was a broken root-owned symlink that failed every
// call, and on Linux /usr/bin/orca is often the GNOME screen reader, so a PATH hit is
// accepted only when it answers Orca's `status --json` envelope. Reached only from an
// explicit `lets orca ...` command, an explicit --probe-orca / --orca-repos, or a
// caller that already found LETS_LAUNCHER=orca; ORCA_WORKTREE_ID alone never enables
// Orca.
var lookOrca = func() (string, bool) {
	for _, p := range bundlePaths() {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
			return p, true
		}
	}
	if p, err := lookPath("orca"); err == nil && probeOrca(p) {
		return p, true
	}
	return "", false
}

// probeOrca accepts a PATH hit only when `status --json` decodes into Orca's envelope
// (`result.app` present); an exit-0 `--version` is not enough.
var probeOrca = func(bin string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "status", "--json").Output()
	if err != nil {
		return false
	}
	var env struct {
		Result *struct {
			App *json.RawMessage `json:"app"`
		} `json:"result"`
	}
	return json.Unmarshal(out, &env) == nil && env.Result != nil && env.Result.App != nil
}

// runOrca executes the CLI with argv (never a shell). The caller owns the context.
var runOrca = func(ctx context.Context, bin string, args ...string) (stdout, stderr []byte, err error) {
	var o, e bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.Bytes(), e.Bytes(), err
}

// Failure is a classified Orca failure. Detail is redacted and capped.
type Failure struct {
	Reason string
	Verb   string
	Detail string
}

func (f *Failure) Error() string {
	if f.Detail != "" {
		return f.Reason + " (" + f.Verb + "): " + f.Detail
	}
	return f.Reason + " (" + f.Verb + ")"
}

var usageRe = regexp.MustCompile(`(?i)unknown (command|flag|option)|unrecognized|usage:|invalid_argument`)

// detail redacts and caps text for a Failure.
func detail(s string) string {
	s = strings.TrimSpace(redact.Control(redact.Text(redact.Creds(s))))
	if len(s) > detailCap {
		s = s[:detailCap]
	}
	return s
}

// errorEnvelope is Orca's `--json` failure shape: {"ok":false,"error":{"code","message"}}.
type errorEnvelope struct {
	OK    *bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// classify maps a failed call to a reason. It reads Orca's JSON error envelope when
// stdout carries one, and the stderr text otherwise.
func classify(verb string, stdout, stderr []byte) *Failure {
	text := string(stderr)
	var env errorEnvelope
	if json.Unmarshal(stdout, &env) == nil && env.Error != nil {
		text = env.Error.Code + ": " + env.Error.Message + " " + text
	}
	switch {
	case strings.Contains(text, "terminal_handle_stale"):
		return &Failure{Reason: ReasonHandleStale, Verb: verb, Detail: detail(text)}
	case usageRe.MatchString(text):
		return &Failure{Reason: ReasonCapabilityMissing, Verb: verb, Detail: detail(text)}
	default:
		return &Failure{Reason: ReasonError, Verb: verb, Detail: detail(text)}
	}
}

// WithHandleRetry re-lists (relist returns a fresh handle for the same terminal)
// and retries ONCE on a stale handle.
func WithHandleRetry(ctx context.Context, handle string, relist func() (string, error), call func(h string) *Failure) *Failure {
	_ = ctx
	f := call(handle)
	if f == nil || f.Reason != ReasonHandleStale {
		return f
	}
	h2, err := relist()
	if err != nil {
		return &Failure{Reason: ReasonHandleStale, Verb: f.Verb, Detail: "relist failed"}
	}
	return call(h2)
}

// Client calls one resolved Orca CLI binary.
type Client struct{ Bin string }

// NewClient resolves the Orca binary.
func NewClient() (*Client, *Failure) {
	bin, ok := lookOrca()
	if !ok {
		return nil, &Failure{Reason: ReasonNotFound, Verb: "lookup"}
	}
	return &Client{Bin: bin}, nil
}

// Run calls a verb and returns stdout, classifying a non-zero exit or an ok:false
// JSON envelope. A context without a deadline gets a bounded one.
func (c *Client) Run(ctx context.Context, verb string, args ...string) ([]byte, *Failure) {
	if _, has := ctx.Deadline(); !has {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultVerbTimeout)
		defer cancel()
	}
	out, stderr, err := runOrca(ctx, c.Bin, args...)
	if err != nil {
		return out, classify(verb, out, stderr)
	}
	var env errorEnvelope
	if json.Unmarshal(out, &env) == nil && env.OK != nil && !*env.OK {
		return out, classify(verb, out, stderr)
	}
	return out, nil
}

// RunChecked is Run with the output discarded.
func (c *Client) RunChecked(ctx context.Context, verb string, args ...string) *Failure {
	_, f := c.Run(ctx, verb, args...)
	return f
}

// DecodeJSON decodes Orca output, reporting orca_output_unrecognized on failure.
func DecodeJSON(verb string, out []byte, v any) *Failure {
	if err := json.Unmarshal(out, v); err != nil {
		return &Failure{Reason: ReasonOutputUnrecognized, Verb: verb, Detail: detail(err.Error())}
	}
	return nil
}

// Status reports whether the app runs. It reads `result.app.running`, and the
// version from `result.runtime.appVersion` (Orca 1.4.203), else `result.app.version`.
func (c *Client) Status(ctx context.Context) (StatusInfo, *Failure) {
	info := StatusInfo{Bin: c.Bin}
	out, f := c.Run(ctx, "status", "status", "--json")
	if f != nil {
		info.Reason = f.Reason
		return info, f
	}
	var env struct {
		Result *struct {
			App *struct {
				Running *bool  `json:"running"`
				Version string `json:"version"`
			} `json:"app"`
			Runtime *struct {
				AppVersion string `json:"appVersion"`
			} `json:"runtime"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &env) != nil || env.Result == nil || env.Result.App == nil || env.Result.App.Running == nil {
		info.Reason = ReasonStatusUnrecognized
		return info, &Failure{Reason: ReasonStatusUnrecognized, Verb: "status"}
	}
	info.Running = *env.Result.App.Running
	info.Version = env.Result.App.Version
	if env.Result.Runtime != nil && env.Result.Runtime.AppVersion != "" {
		info.Version = env.Result.Runtime.AppVersion
	}
	if !info.Running {
		info.Reason = ReasonAppNotRunning
		return info, &Failure{Reason: ReasonAppNotRunning, Verb: "status"}
	}
	return info, nil
}

// PsWorktree is one `worktree ps --json` row (the fields LETS reads).
type PsWorktree struct {
	WorktreeID  string    `json:"worktreeId"`
	Path        string    `json:"path"`
	Branch      string    `json:"branch"`
	DisplayName string    `json:"displayName"`
	Agents      []PsAgent `json:"agents"`
}

// PsAgent is one agent pane of a `worktree ps` row. State (Orca 1.4.203):
// working | done | waiting (a permission prompt is shown).
type PsAgent struct {
	PaneKey   string `json:"paneKey"`
	State     string `json:"state"`
	AgentType string `json:"agentType"`
	ToolName  string `json:"toolName"`
}

// Ps lists Orca's worktrees.
func (c *Client) Ps(ctx context.Context) ([]PsWorktree, *Failure) {
	out, f := c.Run(ctx, "worktree ps", "worktree", "ps", "--json")
	if f != nil {
		return nil, f
	}
	var env struct {
		Result *struct {
			Worktrees []PsWorktree `json:"worktrees"`
		} `json:"result"`
	}
	if f := DecodeJSON("worktree ps", out, &env); f != nil {
		return nil, f
	}
	if env.Result == nil {
		return nil, &Failure{Reason: ReasonOutputUnrecognized, Verb: "worktree ps"}
	}
	return env.Result.Worktrees, nil
}

// SelfWorktree returns the Orca worktree id of this process's checkout.
// ORCA_WORKTREE_ID is accepted only when `worktree ps` lists that id with a path
// that is this process's toplevel: a second `claude` started from the same shell in
// another checkout inherits the variable.
func (c *Client) SelfWorktree(ctx context.Context) (string, *Failure) {
	id := os.Getenv("ORCA_WORKTREE_ID")
	if id == "" {
		return "", &Failure{Reason: ReasonEnvAbsent, Verb: "env"}
	}
	cwd, _ := os.Getwd()
	top := gitutil.ProjectRoot(cwd, 2*time.Second)
	rows, f := c.Ps(ctx)
	if f != nil {
		return "", f
	}
	for _, r := range rows {
		if r.WorktreeID == id {
			if top != "" && fsutil.SameDir(r.Path, top) {
				return id, nil
			}
			break
		}
	}
	return "", &Failure{Reason: ReasonEnvMismatch, Verb: "worktree ps"}
}

// Receipt is what `terminal send --json` proves. TurnStarted is never inferred from
// acceptance: a send during a running tool is queued and stops at input_accepted.
type Receipt struct {
	InputAccepted bool
	TurnStarted   bool
}

// ParseReceipt reads a `terminal send --json` result.
func ParseReceipt(out []byte) (Receipt, *Failure) {
	var env struct {
		Result *struct {
			Send *struct {
				Accepted bool `json:"accepted"`
				Prompt   *struct {
					Stages []string `json:"stages"`
				} `json:"prompt"`
			} `json:"send"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &env) != nil || env.Result == nil || env.Result.Send == nil {
		return Receipt{}, &Failure{Reason: ReasonOutputUnrecognized, Verb: "terminal send"}
	}
	r := Receipt{InputAccepted: env.Result.Send.Accepted}
	if p := env.Result.Send.Prompt; p != nil {
		for _, s := range p.Stages {
			switch s {
			case "input_accepted":
				r.InputAccepted = true
			case "turn_started":
				r.TurnStarted = true
			}
		}
	}
	return r, nil
}
