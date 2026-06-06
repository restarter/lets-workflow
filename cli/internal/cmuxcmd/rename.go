//go:build unix

package cmuxcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RenameOptions configures the rename flow.
type RenameOptions struct {
	Title string // required: the new workspace title (tab label)
	Ref   string // optional: explicit workspace ref/uuid/index; skips resolution
	Cwd   string // optional: resolve the workspace by current_directory; else use the active (selected) one
}

// renameWorkspaceRaw runs `cmux workspace rename <ref> --title <title>`.
// Overridable in tests.
var renameWorkspaceRaw = func(ctx context.Context, bin, ref, title string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, "workspace", "rename", ref, "--title", title)
	cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
	return cmd.CombinedOutput()
}

// Rename changes a cmux workspace's tab label. Side-effect-free: no git, branch,
// or filesystem change. Never hard-fails on cmux unavailability - returns
// OK=true with Rename.Renamed=false + a reason. Only a missing --title is a hard
// error (ExitUsage). Resolution: explicit Ref, else Cwd match, else the active
// (selected) workspace.
func Rename(ctx context.Context, opts RenameOptions) (*RenameResult, error) {
	result := &RenameResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "rename", Steps: []Step{}}}
	addStep := func(status, msg string) { result.Steps = append(result.Steps, Step{Status: status, Message: msg}) }

	if opts.Title == "" {
		e := &Error{Code: ExitUsage, Kind: "title_missing", Message: "--title is required"}
		result.OK = false
		result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return result, e
	}

	if runtimeGOOS != "darwin" {
		result.OK = true
		result.Rename = &RenameInfo{Renamed: false, Reason: "not_macos"}
		addStep(StepSkip, "cmux is macOS-only; nothing to rename")
		return result, nil
	}
	bin, ok := lookCmux()
	if !ok {
		result.OK = true
		result.Rename = &RenameInfo{Renamed: false, Reason: "cmux_not_found"}
		addStep(StepSkip, "cmux not found on PATH; nothing to rename")
		return result, nil
	}

	ref := opts.Ref
	var oldTitle string
	if ref == "" {
		ws, err := listWorkspaces(ctx, bin)
		if err != nil {
			result.OK = true
			result.Rename = &RenameInfo{Renamed: false, Reason: "cmux_error"}
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
			result.Rename = &RenameInfo{Renamed: false, Reason: "workspace_not_found"}
			addStep(StepWarn, "no matching cmux workspace (pass --ref, a --cwd that matches, or run from an active workspace)")
			return result, nil
		}
		ref = match.Ref
		oldTitle = match.Title
	}

	if out, err := renameWorkspaceRaw(ctx, bin, ref, opts.Title); err != nil {
		result.OK = true
		result.Rename = &RenameInfo{Renamed: false, Ref: ref, Reason: "cmux_error"}
		addStep(StepWarn, fmt.Sprintf("cmux workspace rename failed: %s", strings.TrimSpace(string(out))))
		return result, nil
	}

	result.OK = true
	result.Rename = &RenameInfo{Renamed: true, Ref: ref, Title: opts.Title, OldTitle: oldTitle}
	addStep(StepOK, fmt.Sprintf("renamed %s -> %q", ref, opts.Title))
	return result, nil
}
