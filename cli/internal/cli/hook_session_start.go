package cli

import (
	"encoding/json"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
)

// NewHookSessionStartCmd builds `lets hook session-start --rules=<path>`.
// Output is the LETS Config block + optional drift notice (rules emission was
// removed in Phase 4b - rules now live in <project>/.claude/rules/lets-rules.md).
//
// Invoked by Claude Code via plugins/lets/hooks/hooks.json on SessionStart.
// Body shared with `lets hook precompact` via runHookSessionPipeline.
//
// In addition to the shared body, this command proactively refreshes the
// session boundary of the current branch's .task-<slug> file (lets-dsdmp) - but
// ONLY on a genuinely new session (input source=startup), so /lets:end has a
// fresh boundary even when /lets:start was skipped. PreCompact does NOT do this
// (the same session continues there - moving the boundary would drop commits).
func NewHookSessionStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Emit LETS Config + drift check (SessionStart hook target)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rulesPath, _ := cmd.Flags().GetString("rules")
			if err := runHookSessionPipeline(cmd, rulesPath); err != nil {
				return err
			}
			// Best-effort, non-fatal: refresh the session boundary on a new session.
			if sid, source := readHookInput(cmd.InOrStdin()); source == "startup" && sid != "" {
				_ = sessionstart.RefreshSessionBoundary(sessionstart.DetectProjectRoot(), sid)
			}
			return nil
		},
	}
	cmd.Flags().String("rules", "", "Path to plugin's rules/lets-rules.md (for drift check)")
	// MarkFlagRequired returns an error only if the flag name is wrong (typo);
	// the flag IS defined immediately above, so any error is a programmer bug
	// and would surface during dev. Intentional swallow.
	_ = cmd.MarkFlagRequired("rules")
	return cmd
}

// readHookInput best-effort parses the SessionStart hook stdin payload that
// Claude Code provides (e.g. {"session_id":"...","source":"startup"}; source is
// one of startup|resume|compact|clear). Any failure (no stdin, malformed, or an
// older Claude Code that sends nothing) yields empty values and the caller skips
// the refresh - never an error, never a block on a closed/empty reader.
func readHookInput(r io.Reader) (sessionID, source string) {
	if r == nil {
		return "", ""
	}
	// Don't block on an interactive terminal (a human running the hook by hand,
	// or `go test` in a TTY). Only read when stdin is piped/redirected - which is
	// exactly how Claude Code delivers the hook payload.
	if f, ok := r.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			return "", ""
		}
	}
	var in struct {
		SessionID string `json:"session_id"`
		Source    string `json:"source"`
	}
	_ = json.NewDecoder(r).Decode(&in)
	return in.SessionID, in.Source
}
