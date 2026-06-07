//go:build unix

package cmuxcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenOptions configures the open flow.
type OpenOptions struct {
	Path        string // directory to open (the worktree path); required, must exist
	Name        string // workspace label (a short readable slug); optional
	Description string // workspace description (e.g. "<task-id> · <title>"); optional
	Command     string // command cmux runs in the workspace, e.g. claude '/lets:start <id>'
	Force       bool   // open even if a workspace already targets Path (skip the duplicate-session guard)
}

// Overridable in tests.
var (
	lookCmux    = func() (string, bool) { p, err := exec.LookPath("cmux"); return p, err == nil }
	runtimeGOOS = runtime.GOOS

	// createWorkspaceRaw runs `cmux workspace create ...`. Returns combined
	// output so a failure message can be surfaced in the fallback.
	createWorkspaceRaw = func(ctx context.Context, bin string, args []string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
		return cmd.CombinedOutput()
	}
)

// Open opens opts.Path in a cmux workspace. Never hard-fails on cmux
// unavailability: returns OK=true with Launch.Launched=false + FallbackCommand
// when cmux is absent, the host is not macOS, or cmux itself errors. Only a
// missing/invalid Path is a hard error (ExitPathInvalid).
func Open(ctx context.Context, opts OpenOptions) (*OpenResult, error) {
	result := &OpenResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "open", Steps: []Step{}}}
	addStep := func(status, msg string) { result.Steps = append(result.Steps, Step{Status: status, Message: msg}) }
	fallbackCmd := fmt.Sprintf("cd %s && claude", shellQuote(opts.Path))

	// Hard error: missing/invalid path.
	if opts.Path == "" {
		e := &Error{Code: ExitPathInvalid, Kind: "path_missing", Message: "--path is required"}
		result.OK = false
		result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return result, e
	}
	if fi, err := os.Stat(opts.Path); err != nil || !fi.IsDir() {
		e := &Error{Code: ExitPathInvalid, Kind: "path_invalid", Message: fmt.Sprintf("not a directory: %s", opts.Path)}
		result.OK = false
		result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return result, e
	}

	// Graceful fallback: not macOS.
	if runtimeGOOS != "darwin" {
		result.OK = true
		result.Launch = &LaunchInfo{Launched: false, Path: opts.Path, Reason: "not_macos", FallbackCommand: fallbackCmd}
		addStep(StepSkip, "cmux launcher is macOS-only; falling back to manual terminal")
		return result, nil
	}

	// Graceful fallback: cmux not installed.
	bin, ok := lookCmux()
	if !ok {
		result.OK = true
		result.Launch = &LaunchInfo{Launched: false, Path: opts.Path, Reason: "cmux_not_found", FallbackCommand: fallbackCmd}
		addStep(StepSkip, "cmux not found on PATH; falling back to manual terminal")
		return result, nil
	}

	// Duplicate-session guard: one live session per worktree. If a workspace
	// already targets this path, refuse to spawn a second (unless --force) - this
	// is the cmux-launcher slice of the spawner-concurrency class. A list failure
	// is non-fatal: fall through and create (best-effort guard).
	if !opts.Force {
		if ws, lerr := listWorkspaces(ctx, bin); lerr == nil {
			if existing := findByDir(ws, opts.Path); existing != nil {
				result.OK = true
				result.Launch = &LaunchInfo{
					Launched:        false,
					Path:            opts.Path,
					Reason:          "already_open",
					ExistingRef:     existing.Ref,
					ExistingTitle:   existing.Title,
					FallbackCommand: fallbackCmd,
				}
				addStep(StepWarn, fmt.Sprintf("a cmux workspace (%s %q) already targets %s; not opening a duplicate (use --force to override)", existing.Ref, existing.Title, opts.Path))
				return result, nil
			}
		}
	}

	// Canonical form: cmux workspace create --cwd <path> [--name] [--description] [--command].
	// `--description` is accepted (cmux: "create ... same flags as new-workspace").
	// No --focus: verified that `cmux workspace create` does not accept it
	// (`cmux workspace create --help`), so passing it would only force the
	// cmux_error fallback. Re-add when/if cmux supports it.
	args := []string{"workspace", "create", "--cwd", opts.Path}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Description != "" {
		args = append(args, "--description", opts.Description)
	}
	if opts.Command != "" {
		args = append(args, "--command", opts.Command)
	}
	if out, err := createWorkspaceRaw(ctx, bin, args); err != nil {
		// cmux present but errored: still optional - fall back, surface reason.
		result.OK = true
		result.Launch = &LaunchInfo{Launched: false, Path: opts.Path, Reason: "cmux_error", FallbackCommand: fallbackCmd}
		addStep(StepWarn, fmt.Sprintf("cmux workspace create failed: %s; falling back to manual terminal", strings.TrimSpace(string(out))))
		return result, nil
	}

	result.OK = true
	result.Launch = &LaunchInfo{Launched: true, WorkspaceName: opts.Name, Description: opts.Description, Path: opts.Path, Command: opts.Command}
	addStep(StepOK, "cmux workspace created")
	return result, nil
}

// shellQuote single-quotes a path for the fallback command string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
