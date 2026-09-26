//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/memberscmd"
)

// NewMembersCmd builds `lets members` (add / dismiss / status / lead): the registry
// of the members a lead spawned in one scope and of the scope's lead. The caller's
// session, pid and toplevel are computed here and in memberscmd, never taken from
// a flag.
func NewMembersCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "members",
		Short:         "Registry of a team's or an execute run's members and lead",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newMembersAddCmd(), newMembersDismissCmd(), newMembersStatusCmd(), newMembersLeadCmd())
	return root
}

// membersOptions resolves the registry root (the main checkout, whose .lets every
// worktree shares) and the caller's toplevel from the working directory.
func membersOptions(scope string) (memberscmd.Options, *memberscmd.Error) {
	cwd, _ := os.Getwd()
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	top := gitutil.ProjectRoot(cwd, 2*time.Second)
	if mainRoot == "" || top == "" {
		return memberscmd.Options{}, &memberscmd.Error{Code: memberscmd.ExitGeneric, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	return memberscmd.Options{Root: mainRoot, CallerToplevel: top, Session: os.Getenv("CLAUDE_CODE_SESSION_ID"), Scope: scope}, nil
}

// printMembers writes res as JSON, or human for a terminal, and returns runErr.
func printMembers(cmd *cobra.Command, jsonOut bool, res any, runErr error, human func()) error {
	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return runErr
	}
	if runErr == nil {
		human()
	}
	return runErr
}

// membersEarly emits the envelope of an error found before the verb ran.
func membersEarly(cmd *cobra.Command, jsonOut bool, sub, scope string, e *memberscmd.Error) error {
	env := memberscmd.NewErrorEnvelope(sub, scope, e.Kind, e.Message)
	return printMembers(cmd, jsonOut, env, e, func() {})
}

func newMembersAddCmd() *cobra.Command {
	var (
		scope   string
		a       memberscmd.AddOptions
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Record a member just spawned (pane: its own session; else in-process)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, e := membersOptions(scope)
			if e != nil {
				return membersEarly(cmd, jsonOut, "add", scope, e)
			}
			res, err := memberscmd.Add(o, a)
			return printMembers(cmd, jsonOut, res, err, func() {
				m := res.Member
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s, %s) recorded in %s, link %s\n", m.Name, m.Role, m.Kind, scope, m.Link)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.StringVar(&a.Name, "name", "", "Member name ([a-z0-9-]{1,40}; bare roster name in a team scope)")
	f.StringVar(&a.Role, "role", "", "Agent role: a shipped lets:* agent except actor")
	f.StringVar(&a.Model, "model", "", "Model the member runs on")
	f.StringVar(&a.Isolation, "isolation", "", "worktree for an isolated member, else empty")
	f.StringVar(&a.WorktreePath, "worktree-path", "", "Absolute path of an isolated member's worktree")
	f.StringVar(&a.WorktreeBranch, "worktree-branch", "", "Branch of an isolated member's worktree")
	f.StringVar(&a.Link, "link", memberscmd.LinkTeam, "team | peer")
	f.StringVar(&a.AgentID, "agent-id", "", "Id an isolated Agent call returned; messages address the member by it")
	f.StringVar(&a.Cwd, "cwd", "", "Where a pane member's session runs (default: --worktree-path when isolated, else the git toplevel)")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

func newMembersDismissCmd() *cobra.Command {
	var (
		scope, name  string
		all, jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "dismiss",
		Short: "Mark a member (or every member) dismissed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, e := membersOptions(scope)
			if e != nil {
				return membersEarly(cmd, jsonOut, "dismiss", scope, e)
			}
			res, err := memberscmd.Dismiss(o, name, all)
			return printMembers(cmd, jsonOut, res, err, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "dismissed: %v\n", res.Dismissed)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.StringVar(&name, "name", "", "Member to dismiss")
	f.BoolVar(&all, "all", false, "Dismiss every member of the scope")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

func newMembersStatusCmd() *cobra.Command {
	var (
		scope, name string
		jsonOut     bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Judge the lead and every member now (live | rotated | gone | unknown)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, e := membersOptions(scope)
			if e != nil {
				return membersEarly(cmd, jsonOut, "status", scope, e)
			}
			res, err := memberscmd.Status(o, name)
			return printMembers(cmd, jsonOut, res, err, func() {
				w := cmd.OutOrStdout()
				if res.Lead != nil {
					fmt.Fprintf(w, "lead %s %s\n", res.Lead.Name, res.Lead.Status)
				}
				for _, m := range res.Members {
					fmt.Fprintf(w, "%s %s %s %s %s %s\n", m.Name, m.Role, m.Kind, m.Status, m.Reason, m.AgentID)
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.StringVar(&name, "name", "", "Only this member")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

func newMembersLeadCmd() *cobra.Command {
	var (
		scope          string
		claim, jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "lead",
		Short: "Show the scope's recorded lead, or claim it for this session (--claim)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, e := membersOptions(scope)
			if e != nil {
				return membersEarly(cmd, jsonOut, "lead", scope, e)
			}
			var res *memberscmd.LeadResult
			var err error
			if claim {
				res, err = memberscmd.ClaimLead(o)
			} else {
				res, err = memberscmd.ShowLead(o)
			}
			return printMembers(cmd, jsonOut, res, err, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "lead %s (session %s) %s\n", res.Lead.Name, res.Lead.Session, res.Lead.Status)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.BoolVar(&claim, "claim", false, "Record this session as the lead")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}
