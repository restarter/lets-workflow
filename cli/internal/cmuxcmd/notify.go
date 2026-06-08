//go:build unix

package cmuxcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NotifyOptions configures the notify flow. Title is required; the workspace is
// resolved by explicit Ref, else Cwd match, else the active (selected) workspace.
type NotifyOptions struct {
	Title    string // required: notification title
	Subtitle string // optional
	Body     string // optional
	Ref      string // optional: explicit workspace ref/uuid/index; skips resolution
	Cwd      string // optional: resolve the workspace by current_directory; else the active one
}

// notifyRaw runs `cmux notify --workspace <ref> --title <t> [--subtitle ..][--body ..]`.
// Args go straight to execve - no shell, so titles/bodies are injection-safe.
// Overridable in tests.
var notifyRaw = func(ctx context.Context, bin, ref, title, subtitle, body string) ([]byte, error) {
	args := []string{"notify", "--workspace", ref, "--title", title}
	if subtitle != "" {
		args = append(args, "--subtitle", subtitle)
	}
	if body != "" {
		args = append(args, "--body", body)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
	return cmd.CombinedOutput()
}

// Notify enqueues a notification to a cmux workspace. NOTE the semantic:
// Notified=true means cmux ACCEPTED the notification (enqueued in its sidebar) -
// NOT that a human saw it. Callers must keep an in-band signal too (the gate
// halts visibly regardless). Never hard-fails on cmux unavailability - returns
// OK=true with Notified=false + a reason (not_macos | cmux_not_found |
// workspace_not_found | cmux_error). Only a missing --title is a hard error
// (ExitUsage). This is the LETS gate-notification sink: callers fire it at
// human-gate points (plan-needs-answers, plan-ready, execute-blocked) and render
// whatever Notify reports. Resolution: explicit Ref, else Cwd match, else the
// active (selected) workspace.
func Notify(ctx context.Context, opts NotifyOptions) (*NotifyResult, error) {
	result := &NotifyResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "notify", Steps: []Step{}}}
	addStep := func(status, msg string) { result.Steps = append(result.Steps, Step{Status: status, Message: msg}) }

	if opts.Title == "" {
		e := &Error{Code: ExitUsage, Kind: "title_missing", Message: "--title is required"}
		result.OK = false
		result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return result, e
	}

	if runtimeGOOS != "darwin" {
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Reason: "not_macos"}
		addStep(StepSkip, "cmux is macOS-only; notification skipped")
		return result, nil
	}
	bin, ok := lookCmux()
	if !ok {
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Reason: "cmux_not_found"}
		addStep(StepSkip, "cmux not found on PATH; notification skipped")
		return result, nil
	}

	ref := opts.Ref
	if ref == "" {
		ws, err := listWorkspaces(ctx, bin)
		if err != nil {
			result.OK = true
			result.Notify = &NotifyInfo{Notified: false, Reason: "cmux_error"}
			addStep(StepWarn, fmt.Sprintf("cmux workspace list failed: %v", err))
			return result, nil
		}
		var match *workspaceEntry
		if opts.Cwd != "" {
			match = findByDir(ws, opts.Cwd)
		} else {
			match = findSelected(ws)
		}
		if match == nil {
			result.OK = true
			result.Notify = &NotifyInfo{Notified: false, Reason: "workspace_not_found"}
			addStep(StepWarn, "no matching cmux workspace (pass --ref, a --cwd that matches, or run from an active workspace)")
			return result, nil
		}
		ref = match.Ref
	}

	if out, err := notifyRaw(ctx, bin, ref, opts.Title, opts.Subtitle, opts.Body); err != nil {
		result.OK = true
		result.Notify = &NotifyInfo{Notified: false, Ref: ref, Reason: "cmux_error"}
		addStep(StepWarn, fmt.Sprintf("cmux notify failed: %s", strings.TrimSpace(string(out))))
		return result, nil
	}

	result.OK = true
	result.Notify = &NotifyInfo{Notified: true, Ref: ref, Title: opts.Title}
	addStep(StepOK, fmt.Sprintf("notified %s: %q", ref, opts.Title))
	return result, nil
}
