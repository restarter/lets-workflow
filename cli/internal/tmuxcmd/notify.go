//go:build unix

package tmuxcmd

import (
	"context"
	"fmt"
	"strings"
)

// NotifyOptions configures the notify flow. Title is required; the window is
// resolved by explicit Ref, else Cwd match, else the active window.
type NotifyOptions struct {
	Title    string // required: notification title
	Subtitle string // optional
	Body     string // optional
	Ref      string // optional: explicit tmux target; skips resolution
	Cwd      string // optional: resolve by pane_current_path; else the active window
}

// Notify displays a gate message on every ATTACHED tmux client. Notified=true
// means the message was displayed on >=1 attached client - it does NOT mean a
// human read it, so callers must keep an in-band signal too (the gate halts
// visibly regardless). Never hard-fails on tmux unavailability - returns OK=true
// with Notified=false + a reason (tmux_not_found | no_client | tmux_error).
// Only a missing --title is a hard error (ExitUsage). This is the tmux arm of
// the LETS gate-notification sink (see cli/internal/notifycmd).
//
// WHY SERVER-WIDE, NOT -t <target>: `display-message -t <session>` on a session
// with ZERO attached clients exits 0 and displays to NOBODY (verified, tmux
// 3.6b). Since Open deliberately creates the worktree session DETACHED, a
// target-scoped notify would report a phantom Notified=true on the autonomous
// pipeline's own default path. A gate must reach the operator where they are
// currently looking - so we enumerate the server's attached clients and message
// each. Zero clients is reported honestly as no_client, never as success.
//
// Message text carries the task identity (composeMessage) precisely because it
// may land on a client attached to some OTHER session - the operator must be
// able to tell which of N parallel worktrees is waiting on them.
//
// `-d 0` holds the message until a key is pressed (man tmux: "a delay of zero
// waits for a key press") - the right dwell for a human gate; tmux's default
// display-time (750ms) would vanish before an away-from-keyboard operator
// returns.
func Notify(ctx context.Context, opts NotifyOptions) (*NotifyResult, error) {
	result := &NotifyResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "notify", Steps: []Step{}}}
	addStep := func(status, msg string) { result.Steps = append(result.Steps, Step{Status: status, Message: msg}) }

	if opts.Title == "" {
		e := &Error{Code: ExitUsage, Kind: "title_missing", Message: "--title is required"}
		result.OK = false
		result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return result, e
	}

	bin, ok := lookTmux()
	if !ok {
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Reason: "tmux_not_found"}
		addStep(StepSkip, "tmux not found on PATH; notification skipped")
		return result, nil
	}

	// Best-effort: name the target window in the envelope when we can resolve
	// it, purely so the caller can report WHICH worktree the gate belongs to.
	// Resolution failure is NOT fatal here - unlike Rename, a notification does
	// not need a target window, only an audience.
	target, _ := resolveTarget(ctx, bin, opts.Ref, opts.Cwd)

	clients, err := listClients(ctx, bin)
	if err != nil {
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Target: target, Reason: "tmux_error"}
		addStep(StepWarn, fmt.Sprintf("tmux list-clients failed: %v", err))
		return result, nil
	}
	if len(clients) == 0 {
		// The honest case: a tmux server is running (maybe with our detached
		// session in it) but no human is attached. Reporting success here would
		// be a phantom - the message would go nowhere.
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Target: target, Reason: "no_client"}
		addStep(StepWarn, "no tmux client attached; nobody to notify (the gate still halts in-band)")
		return result, nil
	}

	// Args go straight to execve - no shell - so a title/body containing $(...)
	// or backticks is a literal, never expanded. Mirrors cmuxcmd.notifyRaw.
	msg := composeMessage(opts)
	var delivered int
	for _, c := range clients {
		if out, derr := runTmux(ctx, bin, "display-message", "-c", c, "-d", "0", msg); derr != nil {
			// One client may have detached between list and display - that is a
			// race, not a failure of the whole notify. Keep going.
			addStep(StepWarn, fmt.Sprintf("display-message to client %s failed: %s", c, strings.TrimSpace(string(out))))
			continue
		}
		delivered++
	}
	if delivered == 0 {
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Target: target, Reason: "tmux_error"}
		addStep(StepWarn, "every attached client rejected the message")
		return result, nil
	}

	result.OK = true
	result.Notify = &NotifyInfo{Notified: true, Target: target, Title: opts.Title, Clients: delivered}
	addStep(StepOK, fmt.Sprintf("displayed on %d attached client(s): %q", delivered, opts.Title))
	return result, nil
}

// composeMessage flattens title/subtitle/body into the single string tmux's
// status line accepts. tmux interprets '#' as the start of a format expansion
// (#{...}, #S, …) even in a literal message, so '#' is doubled to escape it.
//
// The message lands on EVERY attached client - possibly one attached to a
// different session than the gated worktree - so the caller is expected to put
// the task identity in Title/Subtitle. Keep it short: this renders on a status
// line, not in a notification centre.
func composeMessage(opts NotifyOptions) string {
	parts := []string{opts.Title}
	if opts.Subtitle != "" {
		parts = append(parts, opts.Subtitle)
	}
	if opts.Body != "" {
		parts = append(parts, opts.Body)
	}
	return strings.ReplaceAll(strings.Join(parts, " - "), "#", "##")
}
