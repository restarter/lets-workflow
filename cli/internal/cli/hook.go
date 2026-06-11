package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

// Hook output size limits enforced by Claude Code (lets-q9bx7). The hook
// stdout is capped at 10K - anything beyond is silently truncated by Claude
// Code to a 2K preview, dropping the LETS Config block entirely. The size
// guard makes any future regression of the cap surface loudly via:
//   - inline truncation marker in stdout (visible to the orchestrator)
//   - warning to cmd.ErrOrStderr() (visible to dev / captured by tests)
const (
	hookSizeWarn  = 9000
	hookSizeError = 10000
)

// truncationMarker is appended to stdout when the body exceeds hookSizeError.
// Visible to the orchestrator so it knows context is incomplete.
const truncationMarker = "\n\n[lets: hook output truncated by size guard - check stderr]\n"

// NewHookCmd builds the parent `hook` command. Subcommands invoke logic
// for individual Claude Code hook events (SessionStart, PreCompact, etc.).
//
// Direct invocation of `lets hook` (no subcommand) shows help.
func NewHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Hook subcommands invoked by Claude Code (SessionStart, PreCompact, ...)",
	}
	cmd.AddCommand(NewHookSessionStartCmd())
	cmd.AddCommand(NewHookPreCompactCmd())
	return cmd
}

// runHookSessionPipeline produces the SessionStart/PreCompact body via
// sessionstart.Run, then hands it to renderHookOutput for size-guarded
// emission. Shared by both hook subcommands. homeDir resolution failure
// degrades to "" (no user scope) - the hook must never error out of a
// Claude Code startup over a missing $HOME.
func runHookSessionPipeline(cmd *cobra.Command, rulesPath string) error {
	home, _ := os.UserHomeDir()
	var buf bytes.Buffer
	if err := sessionstart.Run(&buf, rulesPath, sessionstart.DetectProjectRoot(), home); err != nil {
		return err
	}
	return renderHookOutput(cmd, buf.Bytes())
}

// renderHookOutput writes body to cmd.OutOrStdout() with size-cap awareness.
//
//   - body size < hookSizeWarn: full body, no warning.
//   - hookSizeWarn ≤ body size < hookSizeError: full body + stderr warning.
//   - body size ≥ hookSizeError: truncated body (safe portion + inline
//     marker) + stderr error. The inline marker stays visible to the
//     orchestrator even when Claude Code would otherwise silently chop the
//     remainder.
func renderHookOutput(cmd *cobra.Command, body []byte) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	size := len(body)

	switch {
	case size >= hookSizeError:
		safeLen := hookSizeError - len(truncationMarker)
		if safeLen < 0 {
			safeLen = 0
		}
		if safeLen > size {
			safeLen = size
		}
		if _, err := out.Write(body[:safeLen]); err != nil {
			return err
		}
		if _, err := io.WriteString(out, truncationMarker); err != nil {
			return err
		}
		fmt.Fprintf(errOut, "error: hook output %d bytes exceeds %d cap - output truncated with marker\n", size, hookSizeError)
	case size >= hookSizeWarn:
		if _, err := out.Write(body); err != nil {
			return err
		}
		fmt.Fprintf(errOut, "warning: hook output %d bytes approaching %d cap\n", size, hookSizeError)
	default:
		if _, err := out.Write(body); err != nil {
			return err
		}
	}
	return nil
}
