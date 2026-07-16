//go:build unix

package tmuxcmd

import (
	"context"
	"fmt"
	"strings"
)

// RenameOptions configures the rename flow.
type RenameOptions struct {
	Title string // required: the new window name (tab label)
	Ref   string // optional: explicit tmux target (session:window); skips resolution
	Cwd   string // optional: resolve the window by pane_current_path; else the active one
}

// Rename changes a tmux window's name. Side-effect-free: no git, branch, or
// filesystem change. Never hard-fails on tmux unavailability - returns OK=true
// with Rename.Renamed=false + a reason. Only a missing --title is a hard error
// (ExitUsage). Resolution: explicit Ref, else Cwd match, else the active window.
func Rename(ctx context.Context, opts RenameOptions) (*RenameResult, error) {
	result := &RenameResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "rename", Steps: []Step{}}}
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
		result.Rename = &RenameInfo{Renamed: false, Reason: "tmux_not_found"}
		addStep(StepSkip, "tmux not found on PATH; nothing to rename")
		return result, nil
	}

	target, oldTitle := resolveTarget(ctx, bin, opts.Ref, opts.Cwd)
	if target == "" {
		result.OK = true
		result.Rename = &RenameInfo{Renamed: false, Reason: "pane_not_found"}
		addStep(StepWarn, "no matching tmux window (pass --ref, a --cwd that matches, or run inside tmux)")
		return result, nil
	}

	if out, err := runTmux(ctx, bin, "rename-window", "-t", target, sanitizeName(opts.Title)); err != nil {
		result.OK = true
		result.Rename = &RenameInfo{Renamed: false, Target: target, Reason: "tmux_error"}
		addStep(StepWarn, fmt.Sprintf("tmux rename-window failed: %s", strings.TrimSpace(string(out))))
		return result, nil
	}

	result.OK = true
	result.Rename = &RenameInfo{Renamed: true, Target: target, Title: opts.Title, OldTitle: oldTitle}
	addStep(StepOK, fmt.Sprintf("renamed %s -> %q", target, opts.Title))
	return result, nil
}
