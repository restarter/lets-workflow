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

// NewPeersCmd builds `lets peers` (who / tail / frame / tell / wait / role /
// orchestrator): the Go side of peer messaging between LETS sessions.
func NewPeersCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "peers",
		Short:         "List, read and message the live Claude sessions of this repo",
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
		Use: "who", Short: "List this repo's live peers (Claude registry; Orca when LETS_LAUNCHER=orca or --probe-orca)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.Timeout = time.Duration(timeoutMs) * time.Millisecond
			res, err := peerscmd.Who(cmd.Context(), o)
			printPeers(cmd, jsonOut, res, func() { peerscmd.RenderWho(cmd.OutOrStdout(), res) })
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout to inspect (default: current directory)")
	f.StringVar(&o.Role, "role", "", "Only this role: orchestrator | worker | peer")
	f.StringVar(&o.Orc, "orc", "", "Only the workers bound to this orchestrator name")
	f.StringVar(&o.ExcludeSession, "exclude-session", "", "Leave this session id out")
	f.StringVar(&o.Session, "session", "", "Also report this session as self")
	f.BoolVar(&o.Prune, "prune", false, "Remove the role files of dead holders")
	f.BoolVar(&o.ProbeOrca, "probe-orca", false, "Consult Orca even when LETS_LAUNCHER is not orca")
	f.StringVar(&o.Repo, "repo", "", "Another registered repo's main checkout (read-only; adds last_orchestrators)")
	f.BoolVar(&o.OrcaRepos, "orca-repos", false, "Every repo Orca knows (validated in Go); rows carry repo_index")
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
	cmd := &cobra.Command{
		Use: "tail", Short: "Read a peer's recent output (redacted, capped)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.RepoIndex = repoIndex()
			res, err := peerscmd.Tail(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.ToTerminal, "to-terminal", "", "Orca terminal handle of a non-Claude agent (screen only)")
	f.IntVar(&o.Last, "last", 0, "Number of turns (default 5; with --since-message the whole reply, up to 100)")
	f.StringVar(&o.SinceMessage, "since-message", "", "Only the turns after this msgid reached the peer")
	f.StringVar(&o.SentAt, "sent-at", "", "When that message was sent (RFC 3339)")
	f.StringVar(&o.AddressedToSession, "addressed-to-session", "", "Only the peer's messages addressed to this session id")
	f.BoolVar(&o.CountOnly, "count-only", false, "With --addressed-to-session: return the count, no text")
	f.BoolVar(&o.ProbeOrca, "probe-orca", false, "Consult Orca even when LETS_LAUNCHER is not orca")
	f.StringVar(&o.Repo, "repo", "", "The peer's project main checkout, when it is not this repo")
	repoIndex = repoIndexFlag(cmd, "The peer's project: an index from lets orca repos / who --orca-repos")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersFrameCmd() *cobra.Command {
	var o peerscmd.FrameOptions
	cmd := &cobra.Command{
		Use: "frame", Short: "Issue a msgid, the message header and the handoff file for a send",
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
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory)")
	f.StringVar(&o.Session, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.Kind, "kind", "", "ask | ping | tell | ask-ro")
	f.StringVar(&o.ToName, "to-name", "", "ask-ro: the name of an orchestrator that is not running")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersTellCmd() *cobra.Command {
	var o peerscmd.TellOptions
	var repoIndex func() *int
	cmd := &cobra.Command{
		Use: "tell", Short: "Deliver a framed message (Orca when send-safe; a Claude peer returns the text for SendMessage)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.RepoIndex = repoIndex()
			res, err := peerscmd.Tell(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory; holds the handoff)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.MsgID, "msgid", "", "The msgid frame issued")
	f.BoolVar(&o.ProbeOrca, "probe-orca", false, "Consult Orca even when LETS_LAUNCHER is not orca")
	f.StringVar(&o.Repo, "repo", "", "The peer's project main checkout, when it is not this repo")
	repoIndex = repoIndexFlag(cmd, "The peer's project: an index from lets orca repos / who --orca-repos")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersWaitCmd() *cobra.Command {
	var o peerscmd.WaitOptions
	cmd := &cobra.Command{
		Use: "wait", Short: "Wait until the peer's transcript shows the end of its reply turn",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := peerscmd.Wait(cmd.Context(), o)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "Checkout (default: current directory)")
	f.StringVar(&o.ToSession, "to-session", "", "Session id of the peer")
	f.StringVar(&o.SinceMessage, "since-message", "", "The msgid that was sent")
	f.StringVar(&o.SentAt, "sent-at", "", "When it was sent (RFC 3339)")
	f.IntVar(&o.TimeoutMs, "timeout-ms", 60000, "How long to wait")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersRoleCmd() *cobra.Command {
	root := &cobra.Command{Use: "role", Short: "Set or clear this session's peer role", SilenceUsage: true, SilenceErrors: true}
	var o peerscmd.RoleOptions
	set := &cobra.Command{
		Use: "set <orchestrator|worker|peer>", Short: "Record this session's role (orchestrator names are unique per repo)",
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
	sf := set.Flags()
	sf.StringVar(&o.Session, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	sf.StringVar(&o.Cwd, "cwd", "", "Checkout root (default: current directory)")
	sf.StringVar(&o.Task, "task", "", "Worker: the task id")
	sf.StringVar(&o.Scope, "scope", "", "Orchestrator: the part of the repo it owns (one line)")
	sf.BoolVar(&o.Takeover, "takeover", false, "Orchestrator: demote the live session holding this name to a plain peer")
	sf.Bool("json", false, "Emit JSON envelope (always on)")
	var clearSession, clearCwd string
	clear := &cobra.Command{
		Use: "clear", Short: "Remove this session's role file", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clearSession == "" {
				clearSession = sessionDefault()
			}
			res, err := peerscmd.RoleClear(cmd.Context(), clearCwd, clearSession)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
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
	f := cmd.Flags()
	f.StringVar(&o.Cwd, "cwd", "", "This checkout (holds the handoff; default: current directory)")
	f.StringVar(&o.Repo, "repo", "", "The other project's main checkout")
	repoIndex = repoIndexFlag(cmd, "An index from lets orca repos / who --orca-repos")
	f.StringVar(&o.Session, "session", "", "The orchestrator's last-seen session id")
	f.IntVar(&o.Pid, "pid", 0, "Its recorded pid")
	f.StringVar(&o.MsgID, "msgid", "", "The msgid frame --kind ask-ro issued")
	f.Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}

func newPeersOrchestratorCmd() *cobra.Command {
	var session, cwd string
	cmd := &cobra.Command{
		Use: "orchestrator", Short: "Resolve which orchestrator this session answers to (self, bound, single, ambiguous, none)",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if session == "" {
				session = sessionDefault()
			}
			res, err := peerscmd.Orchestrator(cmd.Context(), cwd, session)
			printPeers(cmd, true, res, nil)
			return err
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "This session's id (default: $CLAUDE_CODE_SESSION_ID)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Checkout (default: current directory)")
	cmd.Flags().Bool("json", false, "Emit JSON envelope (always on)")
	return cmd
}
