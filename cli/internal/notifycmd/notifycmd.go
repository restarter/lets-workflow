//go:build unix

// Package notifycmd implements `lets notify` - the launcher-neutral gate
// notification sink. It resolves LETS_LAUNCHER (project .lets/.env over user
// ~/.lets/.env, via letsconfig.MergedEnv) and delegates to the matching
// launcher package.
//
// Callers (the /lets:plan-workflow and /lets:execute gate snippets) must NOT
// hardcode a launcher: a snippet that says `lets cmux notify` silently does
// nothing for a tmux user, and a snippet that interpolates the launcher name
// would need a `terminal`-guard replicated in every command file.
//
// Never hard-fails: an absent/unknown launcher, an absent launcher binary, or a
// launcher error all return OK=true with Notified=false + a reason.
package notifycmd

import (
	"context"

	"github.com/restarter/lets-workflow/cli/internal/cmuxcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/tmuxcmd"
)

// SchemaVersion is the version of the JSON envelope emitted by `lets notify`.
// Per-package; guarded by TestResult_SchemaContract.
const SchemaVersion = 1

// Step mirrors the per-launcher step shape.
type Step struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ErrorInfo is the first-class error object emitted when ok=false.
type ErrorInfo struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Info is the launcher-agnostic notify outcome. Notified=true means the active
// launcher ACCEPTED the notification - never that a human saw it.
// Reason: launcher_terminal | launcher_unknown | launcher_error | <the
// launcher's own reason, passed through verbatim>.
type Info struct {
	Notified bool   `json:"notified"`
	Launcher string `json:"launcher"`
	Target   string `json:"target,omitempty"` // cmux ref or tmux target
	Title    string `json:"title,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Result is the `lets notify` envelope.
type Result struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Steps         []Step     `json:"steps"`
	Notify        *Info      `json:"notify,omitempty"`
}

// Options mirrors the per-launcher NotifyOptions plus the env-file locators.
type Options struct {
	Title    string
	Subtitle string
	Body     string
	Ref      string
	Cwd      string
	// ProjectRoot / HomeDir locate the .env files. Empty ProjectRoot means the
	// caller could not resolve a git toplevel - MergedEnv then reads only the
	// user layer, and an absent LETS_LAUNCHER falls back to the default.
	ProjectRoot string
	HomeDir     string
}

// usageError carries the ExitUsage code for a missing --title.
type usageError struct{}

func (usageError) Error() string { return "title_missing: --title is required" }
func (usageError) ExitCode() int { return 2 }

// ResolveLauncher returns the effective LETS_LAUNCHER. Unknown or empty values
// fall back to the canonical default. Exported for tests and future callers.
func ResolveLauncher(projectRoot, homeDir string) string {
	v := letsconfig.MergedEnv(projectRoot, homeDir)["LETS_LAUNCHER"]
	if v == "" {
		return letsconfig.Defaults()["LETS_LAUNCHER"]
	}
	return v
}

// Notify dispatches to the active launcher. A missing Title is the only hard
// error (mirrors both launchers).
func Notify(ctx context.Context, opts Options) (*Result, error) {
	res := &Result{SchemaVersion: SchemaVersion, Subcommand: "notify", Steps: []Step{}}
	addStep := func(status, msg string) { res.Steps = append(res.Steps, Step{Status: status, Message: msg}) }

	if opts.Title == "" {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "title_missing", Message: "--title is required"}
		return res, usageError{}
	}

	launcher := ResolveLauncher(opts.ProjectRoot, opts.HomeDir)

	switch launcher {
	case "cmux":
		sub, _ := cmuxcmd.Notify(ctx, cmuxcmd.NotifyOptions{
			Title: opts.Title, Subtitle: opts.Subtitle, Body: opts.Body, Ref: opts.Ref, Cwd: opts.Cwd,
		})
		res.OK = true
		// Nil-guard is MANDATORY: cmuxcmd.Notify leaves Notify==nil only on the
		// title_missing hard error we already screened, so it is safe today - but
		// the day cmux grows a second hard-error path, an unconditional deref
		// would panic a gate. Degrade instead.
		if sub == nil || sub.Notify == nil {
			res.Notify = &Info{Notified: false, Launcher: launcher, Reason: "launcher_error"}
			addStep("warn", "cmux notify returned no result")
			return res, nil
		}
		res.Notify = &Info{Notified: sub.Notify.Notified, Launcher: launcher, Target: sub.Notify.Ref, Title: sub.Notify.Title, Reason: sub.Notify.Reason}
		res.Steps = append(res.Steps, cmuxSteps(sub.Steps)...)
	case "tmux":
		sub, _ := tmuxcmd.Notify(ctx, tmuxcmd.NotifyOptions{
			Title: opts.Title, Subtitle: opts.Subtitle, Body: opts.Body, Ref: opts.Ref, Cwd: opts.Cwd,
		})
		res.OK = true
		if sub == nil || sub.Notify == nil {
			res.Notify = &Info{Notified: false, Launcher: launcher, Reason: "launcher_error"}
			addStep("warn", "tmux notify returned no result")
			return res, nil
		}
		res.Notify = &Info{Notified: sub.Notify.Notified, Launcher: launcher, Target: sub.Notify.Target, Title: sub.Notify.Title, Reason: sub.Notify.Reason}
		res.Steps = append(res.Steps, tmuxSteps(sub.Steps)...)
	case "terminal":
		res.OK = true
		res.Notify = &Info{Notified: false, Launcher: launcher, Reason: "launcher_terminal"}
		addStep("skip", "LETS_LAUNCHER=terminal has no notification channel; skipped")
	default:
		// A hand-edited .env value the binary cannot serve. Degrade + name it -
		// `lets init --launcher` rejects these, but .env is hand-editable.
		res.OK = true
		res.Notify = &Info{Notified: false, Launcher: launcher, Reason: "launcher_unknown"}
		addStep("warn", "unknown LETS_LAUNCHER "+launcher+"; no notification channel")
	}
	return res, nil
}

// cmuxSteps / tmuxSteps convert a launcher's steps to notifycmd's. The two
// launcher Step types are structurally identical but nominally distinct; two
// tiny converters beat a generic helper for two call sites.
func cmuxSteps(in []cmuxcmd.Step) []Step {
	out := make([]Step, len(in))
	for i, s := range in {
		out[i] = Step{Status: s.Status, Message: s.Message}
	}
	return out
}

func tmuxSteps(in []tmuxcmd.Step) []Step {
	out := make([]Step, len(in))
	for i, s := range in {
		out[i] = Step{Status: s.Status, Message: s.Message}
	}
	return out
}
