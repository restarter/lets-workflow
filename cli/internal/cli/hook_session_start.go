package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/hook/sessionstart"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/rulescache"
)

// rulesSyncFn keeps ~/.claude/rules/lets-rules.md a cache of THIS session's
// plugin (lets-tg008). Every source: the no-op path hashes the plugin's and the installed rules (sub-millisecond) under the per-home lock.
var rulesSyncFn = func(rulesPath string) string {
	if rulesPath == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	o := rulescache.Options{PluginRoot: filepath.Dir(filepath.Dir(rulesPath)), HomeDir: home}
	if root := sessionstart.DetectProjectRoot(); root != "" {
		if _, err := os.Stat(filepath.Join(root, ".lets", ".env")); err == nil {
			o.InProject = true
			o.ScopeUser = letsconfig.MergedEnv(root, home)["LETS_RULES_SCOPE"] == "user"
		}
	}
	return rulescache.Sync(o).Notice()
}

// NewHookSessionStartCmd builds `lets hook session-start --rules=<path>`.
// Output is the LETS Config block + optional drift notice (rules emission was
// removed in Phase 4b - rules now live in <project>/.claude/rules/lets-rules.md).
//
// Invoked by Claude Code via plugins/lets/hooks/hooks.json on SessionStart.
// Body shared with `lets hook precompact` via runHookSessionPipeline.
//
// In addition to the shared body, this command:
//   - reads the stdin payload FIRST, and on source startup|resume|clear self-heals
//     an unlinked linked worktree of an initialized LETS project (selfHeal: an
//     Orca worktree whose setup hook did not run) BEFORE LETS Config is built, so
//     the Config comes from the linked main .lets/.env. compact never self-heals,
//     and PreCompact never reads the payload at all;
//   - on source startup|resume|clear, restores this session's peer role
//     (peersHeal): a role file carried to a re-minted id, or the role rebuilt from
//     its anchor - best-effort and silent, like selfHeal;
//   - on every source, after the self-heal, keeps the global
//     ~/.claude/rules/lets-rules.md a cache of this session's plugin
//     (rulesSyncFn, lets-tg008); its outcome joins the Notice;
//   - proactively refreshes the session boundary of the current branch's
//     .task-<slug> file (lets-dsdmp) - but ONLY on a genuinely new session
//     (source=startup), so /lets:end has a fresh boundary even when /lets:start
//     was skipped - and on /clear carries it to the re-minted id on proof only.
//     Both go through the session-owner guard (sessionOwnerPass): a teammate pane
//     in the same worktree never takes a live session's boundary, and a declined
//     write is a Notice, never silent. PreCompact does NOT do this (the same
//     session continues there - moving the boundary would drop commits).
func NewHookSessionStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Emit LETS Config + drift check (SessionStart hook target)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rulesPath, _ := cmd.Flags().GetString("rules")
			sid, source := readHookInput(cmd.InOrStdin())
			var notices, sessionNotices []string
			switch source {
			case "startup", "resume", "clear":
				root := sessionstart.DetectProjectRoot()
				notices = append(notices, selfHealFn(root, rulesPath))
				// Before peersHeal: its reconcile moves a re-minted role file to the new
				// id, and the /clear carry's proof is the OLD id's role file.
				sessionNotices = sessionOwnerPass(root, sid, source)
				peersHealFn(root, sid)
			}
			notices = append(notices, rulesSyncFn(rulesPath)) // every source, after self-heal
			notices = append(notices, sessionNotices...)      // after rules sync, for a stable read
			return runHookSessionPipeline(cmd, rulesPath, notices)
		},
	}
	cmd.Flags().String("rules", "", "Path to plugin's rules/lets-rules.md (for drift check)")
	// MarkFlagRequired returns an error only if the flag name is wrong (typo);
	// the flag IS defined immediately above, so any error is a programmer bug
	// and would surface during dev. Intentional swallow.
	_ = cmd.MarkFlagRequired("rules")
	return cmd
}

// sessionOwnerPass runs the guarded session: write for source - a refresh on
// startup, a carry on /clear, none on resume - and, on an allowed pass (or a resume)
// of the recorded team lead, refreshes the lead's pid. It returns the Notice
// messages of a declined or carried write; nothing here fails the hook.
func sessionOwnerPass(root, sid, source string) []string {
	if root == "" || sid == "" {
		return nil
	}
	g, onAllowed := sessionGuardFn(root, sid)
	var o sessionstart.Outcome
	var msg string
	switch source {
	case "startup":
		o, msg = sessionstart.RefreshSessionBoundary(root, sid, g)
	case "clear":
		o, msg = sessionstart.CarrySession(root, sid, g)
	case "resume":
		o = sessionstart.OutcomeUnchanged // `claude -r`: same id, maybe a new pid
	default:
		return nil
	}
	if o.Allowed() && onAllowed != nil {
		onAllowed()
	}
	// No peers dir = no role file can ever prove a carry: saying so on every /clear
	// would be noise for every install without peers. Nothing was written, and the
	// session-boundary reader still reports the line as prior-session.
	if o == sessionstart.OutcomeNoProof {
		if _, err := os.Stat(filepath.Join(root, ".lets", "sessions", "peers")); err != nil {
			return nil
		}
	}
	if msg == "" {
		return nil
	}
	return []string{msg}
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
