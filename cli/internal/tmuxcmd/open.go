//go:build unix

package tmuxcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenOptions configures the open flow.
type OpenOptions struct {
	Path        string // directory to open (the worktree path); required, must exist
	Name        string // session/window label (a short readable slug); optional
	Description string // stamped into the @lets_task window option, e.g. "<task-id> · <title>"
	Command     string // command to run in the pane, e.g. claude '/lets:start <id>'
	Force       bool   // open even if a pane already lives at Path (skip the duplicate guard)
}

// Overridable in tests.
var (
	lookTmux   = func() (string, bool) { p, err := exec.LookPath("tmux"); return p, err == nil }
	insideTmux = func() bool { return os.Getenv("TMUX") != "" }

	// runTmux runs an arbitrary tmux invocation, returning combined output so a
	// failure message can be surfaced in the fallback.
	runTmux = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, bin, args...).CombinedOutput()
	}
)

// Open opens opts.Path in a tmux window (inside $TMUX) or a new detached session
// (outside). Never hard-fails on tmux unavailability: returns OK=true with
// Launch.Launched=false + FallbackCommand when tmux is absent, tmux errors, or a
// pane already lives at Path. Only a missing/invalid Path is a hard error
// (ExitPathInvalid).
//
// NEVER auto-attaches. Outside tmux the session is created detached and
// AttachCommand is surfaced instead - attaching here would seize the terminal of
// whatever process invoked us (typically a Claude Code bash subprocess).
func Open(ctx context.Context, opts OpenOptions) (*OpenResult, error) {
	result := &OpenResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "open", Steps: []Step{}}}
	addStep := func(status, msg string) { result.Steps = append(result.Steps, Step{Status: status, Message: msg}) }

	launchCmd := opts.Command
	if launchCmd == "" {
		launchCmd = "claude"
	}
	fallbackCmd := fmt.Sprintf("cd %s && %s", shellQuote(opts.Path), launchCmd)

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

	// Graceful fallback: tmux not installed.
	bin, ok := lookTmux()
	if !ok {
		result.OK = true
		result.Launch = &LaunchInfo{Launched: false, Path: opts.Path, Reason: "tmux_not_found", FallbackCommand: fallbackCmd}
		addStep(StepSkip, "tmux not found on PATH; falling back to manual terminal")
		return result, nil
	}

	// Duplicate guard: one live session per worktree. A list failure (no server
	// running yet) is non-fatal - fall through and create. Mirrors cmuxcmd.
	if !opts.Force {
		if panes, lerr := listPanes(ctx, bin); lerr == nil {
			if existing := findByPath(panes, opts.Path); existing != nil {
				result.OK = true
				result.Launch = &LaunchInfo{
					Launched:        false,
					Path:            opts.Path,
					Reason:          "already_open",
					ExistingTarget:  existing.Target(),
					ExistingTitle:   existing.Title,
					FallbackCommand: fallbackCmd,
				}
				addStep(StepWarn, fmt.Sprintf("a tmux pane (%s %q) already lives at %s; not opening a duplicate (use --force to override)", existing.Target(), existing.Title, opts.Path))
				return result, nil
			}
		}
	}

	name := sanitizeName(opts.Name)
	if opts.Name == "" {
		name = sanitizeName(filepath.Base(opts.Path))
	}
	// The window name IS the tmux status line - it gets the SHORT slug, never
	// the description. cmux's --description is a tooltip with no width budget;
	// tmux has no tooltip. The full description is stamped into the @lets_task
	// window option below instead (queryable, invisible, no width cost).
	windowName := name

	// -P -F prints the created target so we never have to guess the window index.
	const targetFormat = "#{session_name}:#{window_index}"

	var (
		target   string
		inside   = insideTmux()
		attachTo string
	)
	switch {
	case inside:
		out, err := runTmux(ctx, bin, "new-window", "-n", windowName, "-c", opts.Path, "-P", "-F", targetFormat)
		if err != nil {
			return tmuxErrFallback(result, addStep, opts.Path, fallbackCmd, "new-window", out)
		}
		target = strings.TrimSpace(string(out))
	default:
		// Outside tmux. If a session with this name already exists (a different
		// worktree that slugged the same), add a window to it rather than
		// hard-failing on tmux's "duplicate session" error.
		if _, err := runTmux(ctx, bin, "has-session", "-t", "="+name); err == nil {
			out, werr := runTmux(ctx, bin, "new-window", "-t", "="+name+":", "-n", windowName, "-c", opts.Path, "-P", "-F", targetFormat)
			if werr != nil {
				return tmuxErrFallback(result, addStep, opts.Path, fallbackCmd, "new-window", out)
			}
			target = strings.TrimSpace(string(out))
		} else {
			out, serr := runTmux(ctx, bin, "new-session", "-d", "-s", name, "-n", windowName, "-c", opts.Path, "-P", "-F", targetFormat)
			if serr != nil {
				return tmuxErrFallback(result, addStep, opts.Path, fallbackCmd, "new-session", out)
			}
			target = strings.TrimSpace(string(out))
		}
		attachTo = fmt.Sprintf("tmux attach -t %s", shellQuote(name))
	}

	// Identity stamp. tmux has no tooltip, so the full "<task-id> · <title>"
	// goes into a window-level user option rather than the (width-budgeted)
	// window name. Readable via `list-panes -F '#{@lets_task}'`. Best-effort:
	// a failure here must NOT sink a launch that already succeeded - the window
	// exists and the command is about to run.
	if opts.Description != "" {
		if out, err := runTmux(ctx, bin, "set-option", "-w", "-t", target, "@lets_task", opts.Description); err != nil {
			addStep(StepWarn, fmt.Sprintf("could not stamp @lets_task: %s", strings.TrimSpace(string(out))))
		}
	}

	// Deliver the command via send-keys: the pane keeps its shell after the
	// command exits, and tmux buffers the keys into the pane pty (no race).
	if opts.Command != "" {
		if out, err := runTmux(ctx, bin, "send-keys", "-t", target, opts.Command, "Enter"); err != nil {
			return tmuxErrFallback(result, addStep, opts.Path, fallbackCmd, "send-keys", out)
		}
	}

	result.OK = true
	result.Launch = &LaunchInfo{
		Launched:          true,
		WorkspaceName:     name,
		Target:            target,
		Description:       opts.Description,
		Path:              opts.Path,
		Command:           opts.Command,
		InExistingSession: inside,
		AttachCommand:     attachTo,
	}
	if inside {
		addStep(StepOK, fmt.Sprintf("opened tmux window %s in the current session", target))
	} else {
		addStep(StepOK, fmt.Sprintf("created detached tmux session %q (%s)", name, target))
	}
	return result, nil
}

// tmuxErrFallback degrades a failed tmux invocation into the graceful
// launched=false envelope (tmux is optional; never hard-fail on its errors).
func tmuxErrFallback(result *OpenResult, addStep func(string, string), path, fallbackCmd, op string, out []byte) (*OpenResult, error) {
	result.OK = true
	result.Launch = &LaunchInfo{Launched: false, Path: path, Reason: "tmux_error", FallbackCommand: fallbackCmd}
	addStep(StepWarn, fmt.Sprintf("tmux %s failed: %s; falling back to manual terminal", op, strings.TrimSpace(string(out))))
	return result, nil
}

// shellQuote single-quotes a path for the fallback command string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
