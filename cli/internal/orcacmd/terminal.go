//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// TerminalOptions configures OpenTerminal: open ONE Orca terminal running Command in
// an existing team worktree - Orca-created on the orca launcher, Go-created otherwise
// (a standing team's lead). Orca never creates the worktree here.
type TerminalOptions struct {
	Worktree string // absolute path of the existing worktree
	Title    string // terminal title (the lead's name, `<callsign>-lead`)
	Command  string // the shell command the terminal runs
}

// TerminalInfo is the outcome. Launched=false is NOT an error: the caller prints
// FallbackCommand (`cd <worktree> && <command>`) for a plain terminal.
type TerminalInfo struct {
	Launched        bool   `json:"launched"`
	Handle          string `json:"handle,omitempty"`
	Path            string `json:"path"`
	Title           string `json:"title"`
	Reason          string `json:"reason,omitempty"`
	FallbackCommand string `json:"fallback_command,omitempty"`
}

// TerminalResult is the terminal-subcommand envelope.
type TerminalResult struct {
	Envelope
	Terminal *TerminalInfo `json:"terminal,omitempty"`
}

// OpenTerminal runs `orca terminal create --worktree path:<w> --title <t> --command <c>`
// and returns its handle. Only bad usage is a hard error (a relative or missing
// worktree, an empty title, a newline or control character in the title or the
// command); Orca absent, not running or refusing degrades to Launched=false with a
// reason and the printed fallback - never an Orca worktree create.
func OpenTerminal(ctx context.Context, o TerminalOptions) (*TerminalResult, error) {
	res := &TerminalResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "terminal", Steps: []Step{}}}
	fail := func(kind, msg string) (*TerminalResult, error) {
		res.Error = &ErrorInfo{Kind: kind, Message: msg}
		return res, &Error{Code: ExitUsage, Kind: kind, Message: msg}
	}
	if fi, err := os.Stat(o.Worktree); !filepath.IsAbs(o.Worktree) || err != nil || !fi.IsDir() {
		return fail("worktree_invalid", fmt.Sprintf("--worktree %q is not an existing absolute directory", o.Worktree))
	}
	if strings.TrimSpace(o.Title) == "" || strings.TrimSpace(o.Command) == "" {
		return fail("usage", "--title and --command are required")
	}
	for _, f := range []struct{ flag, v string }{{"--title", o.Title}, {"--command", o.Command}} {
		if strings.ContainsFunc(f.v, unicode.IsControl) {
			return fail("control_char", f.flag+" carries a newline or control character")
		}
	}
	info := &TerminalInfo{Path: o.Worktree, Title: o.Title, FallbackCommand: "cd " + shellQuote(o.Worktree) + " && " + o.Command}
	res.Terminal, res.OK = info, true
	degrade := func(f *Failure) (*TerminalResult, error) {
		info.Reason = f.Reason
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: fmt.Sprintf("orca unavailable (%s) - print the command for a terminal instead", f.Error())})
		return res, nil
	}
	c, f := NewClient()
	if f != nil {
		return degrade(f)
	}
	if _, f := c.Status(ctx); f != nil {
		return degrade(f)
	}
	h, f := c.CreateTerminal(ctx, o.Worktree, o.Title, o.Command)
	if f != nil {
		return degrade(f)
	}
	info.Launched, info.Handle, info.FallbackCommand = true, h, ""
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("orca terminal %s opened in %s (%s)", o.Title, o.Worktree, h)})
	return res, nil
}

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// RenderTerminal writes a human-readable summary of a TerminalResult.
func RenderTerminal(w io.Writer, res *TerminalResult) {
	if !res.OK {
		if res.Error != nil {
			fmt.Fprintf(w, "Error: %s: %s\n", res.Error.Kind, res.Error.Message)
		}
		return
	}
	t := res.Terminal
	if t.Launched {
		fmt.Fprintf(w, "Orca terminal %s opened in %s (handle %s)\n", t.Title, t.Path, t.Handle)
		return
	}
	fmt.Fprintf(w, "Orca unavailable (%s) - open a terminal and run:\n\n    %s\n", t.Reason, t.FallbackCommand)
}
