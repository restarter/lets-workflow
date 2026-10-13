//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/peerscmd"
)

// NewPeersCmd builds `lets peers`: who / tail / orchestrator / ask-ro, plus the
// deprecated v0.10.0 frame / tell / wait / role stubs. Messages themselves go
// through SendMessage from the orc skill.
func NewPeersCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "peers",
		Short:         "List and read the live Claude sessions of this repo; resolve its orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newPeersWhoCmd(), newPeersTailCmd(), newPeersFrameCmd(), newPeersTellCmd(), newPeersWaitCmd(), newPeersRoleCmd(), newPeersOrchestratorCmd(), newPeersAskROCmd())
	return root
}

func printPeers(cmd *cobra.Command, jsonOut bool, res any, render func()) {
	if jsonOut || render == nil {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return
	}
	render()
}

func sessionDefault() string { return os.Getenv("CLAUDE_CODE_SESSION_ID") }

func newPeersWhoCmd() *cobra.Command {
	var o peerscmd.WhoOptions
	var jsonOut bool
	var timeoutMs int
	cmd := &cobra.Command{
		Use: "who", Short: "List this repo's live Claude sessions (the Claude Code session registry)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.Timeout = time.Duration(timeoutMs) * time.Millisecond
			if o.Probe.FromName != "" || o.Probe.FromAddr != "" {
				o.Caller = sessionDefault() // the standing answer is for this session, unless --session names it
			}
			res, err := peerscmd.Who(cmd.Context(), o)
			printPeers(cmd, jsonOut, res, func() { peerscmd.RenderWho(cmd.OutOrStdout(), res) })
			return err
		},
	}
	// v0.10.0 flags (--role, --prune, --probe-orca, --repo) pass the whitelist and are ignored.
	whitelist(cmd)
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout to inspect (default: current directory)")
	f.StringVar(&o.Orc, "orc", "", "Only the sessions whose branch is bound to this orchestrator name")
	f.StringVar(&o.ExcludeSession, "exclude-session", "", "Leave this session id out")
	f.StringVar(&o.Session, "session", "", "Also report this session as self")
	f.BoolVar(&o.OrcaRepos, "orca-repos", false, "Every repo Orca knows (validated in Go): orchestrator rows with repo_index")
	f.StringVar(&o.Probe.FromName, "from-name", "", "With --orc <own name>: the inbound message's envelope from-name (with --from-addr) - answers standing_ok")
	f.StringVar(&o.Probe.FromAddr, "from-addr", "", "The inbound message's envelope from attribute, uds:/tmp/cc-socks/<pid>.sock (with --from-name)")
	f.IntVar(&timeoutMs, "timeout-ms", 2500, "Total time budget")
	f.BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

// repoIndexFlag registers --repo-index; the returned func yields nil when it was not given.
func repoIndexFlag(cmd *cobra.Command, usage string) func() *int {
	idx := -1
	cmd.Flags().IntVar(&idx, "repo-index", -1, usage)
	return func() *int {
		if idx < 0 {
			return nil
		}
		return &idx
	}
}

func newPeersTailCmd() *cobra.Command {
	var o peerscmd.TailOptions
	var repoIndex func() *int
	removed := []string{"to-terminal", "addressed-to-session", "count-only", "probe-orca", "repo"}
	cmd := &cobra.Command{
		Use: "tail [<name>]", Short: "Read a live session of this repo (redacted, capped)",
		Long: "Read a live session of this repo. Name it by its exact live name (positional) or --to-session SID; read a reply with --since <RFC 3339>.",
		Args: cobra.MaximumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				o.Name = args[0]
			}
			o.RepoIndex = repoIndex()
			for _, name := range removed {
				if cmd.Flags().Changed(name) {
					o.Removed = append(o.Removed, "--"+name)
				}
			}
			res, err := peerscmd.Tail(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(cmd)
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.IntVar(&o.Last, "last", 0, "Number of turns (default 5; with --since the whole reply, up to 100)")
	f.StringVar(&o.Since, "since", "", "Only the turns timestamped at or after this time (RFC 3339)")
	f.StringVar(&o.SinceMessage, "since-message", "", "Deprecated: only the turns after this msgid reached the peer (with --sent-at)")
	f.StringVar(&o.SentAt, "sent-at", "", "Deprecated: when that message was sent (RFC 3339)")
	// v0.10.0 flags: --repo-index answers cross_repo_read_retired; the others answer
	// usage naming --since. Hidden, so only an older plugin passes them.
	repoIndex = repoIndexFlag(cmd, "Retired: reading another repo's session was removed")
	var ignoredStr string
	var ignoredBool bool
	f.StringVar(&ignoredStr, "to-terminal", "", "Removed (v0.10.0)")
	f.StringVar(&ignoredStr, "addressed-to-session", "", "Removed (v0.10.0)")
	f.BoolVar(&ignoredBool, "count-only", false, "Removed (v0.10.0)")
	f.BoolVar(&ignoredBool, "probe-orca", false, "Removed (v0.10.0)")
	f.StringVar(&ignoredStr, "repo", "", "Removed (v0.10.0)")
	for _, name := range append([]string{"repo-index"}, removed...) {
		_ = f.MarkHidden(name)
	}
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

// whitelist lets a v0.10.0 caller's unknown flags through: every peers subcommand
// accepts and ignores a flag it no longer defines instead of failing to parse.
func whitelist(cmd *cobra.Command) { cmd.FParseErrWhitelist.UnknownFlags = true }

func newPeersFrameCmd() *cobra.Command {
	var o peerscmd.FrameOptions
	cmd := &cobra.Command{
		Use: "frame", Short: "Deprecated: issue a msgid, header and handoff file (v0.10.0 skills)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.Session == "" {
				o.Session = sessionDefault()
			}
			res, err := peerscmd.Frame(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(cmd)
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory)")
	f.StringVar(&o.Session, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.Kind, "kind", "", "ask | ping | tell | ask-ro")
	f.StringVar(&o.ToName, "to-name", "", "The peer's name for the header")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersTellCmd() *cobra.Command {
	var o peerscmd.TellOptions
	cmd := &cobra.Command{
		Use: "tell", Short: "Deprecated: hand a framed message back for SendMessage (never types anything)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := peerscmd.Tell(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(cmd) // --repo-index, --repo, --probe-orca of a v0.10.0 skill are ignored
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory; holds the handoff)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.MsgID, "msgid", "", "The msgid frame issued")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersWaitCmd() *cobra.Command {
	var o peerscmd.WaitOptions
	cmd := &cobra.Command{
		Use: "wait", Short: "Deprecated: answers completion_unverifiable (a reply arrives as a cross-session message)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := peerscmd.Wait(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(cmd)
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.SinceMessage, "since-message", "", "The msgid that was sent")
	f.StringVar(&o.SentAt, "sent-at", "", "When it was sent (RFC 3339)")
	f.IntVar(&o.TimeoutMs, "timeout-ms", 60000, "Ignored")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersRoleCmd() *cobra.Command {
	root := &cobra.Command{Use: "role", Short: "Deprecated: record or clear this session's /clear proof (until lets-gbfpd)", SilenceUsage: true, SilenceErrors: true, Hidden: true}
	var o peerscmd.RoleOptions
	set := &cobra.Command{
		Use: "set <word>", Short: "Record this session's /clear proof (the role word means nothing any more)",
		Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Role = args[0]
			if o.Session == "" {
				o.Session = sessionDefault()
			}
			res, err := peerscmd.RoleSet(cmd.Context(), o.Cwd, o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(set)
	sf := set.Flags()
	sf.StringVar(&o.Session, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	sf.StringVar(&o.Cwd, "cwd", "", "Checkout root (default: current directory)")
	sf.StringVar(&o.Task, "task", "", "Ignored (v0.10.0)")
	sf.StringVar(&o.Scope, "scope", "", "Ignored (v0.10.0)")
	sf.BoolVar(&o.Takeover, "takeover", false, "Ignored (v0.10.0)")
	sf.Bool("json", false, "Emit JSON envelope (always on)")
	var clearSession, clearCwd string
	clear := &cobra.Command{
		Use: "clear", Short: "Remove this session's /clear proof", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clearSession == "" {
				clearSession = sessionDefault()
			}
			res, err := peerscmd.RoleClear(cmd.Context(), clearCwd, clearSession)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(clear)
	clear.Flags().StringVar(&clearSession, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	clear.Flags().StringVar(&clearCwd, "cwd", "", "Checkout root (default: current directory)")
	clear.Flags().Bool("json", false, "Emit JSON envelope (always on)")
	root.AddCommand(set, clear)
	return root
}

func newPeersAskROCmd() *cobra.Command {
	var o peerscmd.AskROOptions
	var repoIndex func() *int
	cmd := &cobra.Command{
		Use: "ask-ro", Short: "Ask a stopped orchestrator a read-only question (forked, plan mode, Read/Grep/Glob only)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.RepoIndex = repoIndex()
			res, err := peerscmd.AskRO(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(cmd)
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "This checkout (holds the question file; default: current directory)")
	f.StringVar(&o.Repo, "repo", "", "The other project's main checkout")
	repoIndex = repoIndexFlag(cmd, "An index from lets orca repos / who --orca-repos")
	f.StringVar(&o.Session, "session", "", "The orchestrator's last-seen session id")
	f.IntVar(&o.Pid, "pid", 0, "Its recorded pid")
	f.StringVar(&o.QuestionFile, "question-file", "", "The question: a file under .lets/cache/ (read once, then removed)")
	f.StringVar(&o.MsgID, "msgid", "", "Deprecated: the msgid frame --kind ask-ro issued")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersOrchestratorCmd() *cobra.Command {
	var session, cwd string
	var probe peerscmd.StandingProbe
	cmd := &cobra.Command{
		Use: "orchestrator", Short: "Resolve whom this session's /lets:orc talks to (bound, self, single, ambiguous, none) and its own self_record",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if session == "" {
				session = sessionDefault()
			}
			res, err := peerscmd.OrchestratorWith(cmd.Context(), cwd, session, probe)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	whitelist(cmd)
	cmd.Flags().StringVar(&session, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Checkout (default: current directory)")
	cmd.Flags().StringVar(&probe.FromName, "from-name", "", "An inbound message's envelope from-name (with --from-addr) - answers standing_ok for the bound orchestrator")
	cmd.Flags().StringVar(&probe.FromAddr, "from-addr", "", "That message's envelope from attribute, uds:/tmp/cc-socks/<pid>.sock (with --from-name)")
	cmd.Flags().Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}
