package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// `lets members` is a deprecated stub for one release (lets-gbfpd): members are
// tracked by ListAgents and the caller's run record / team file (member-run). It
// records nothing; its answers keep a v0.10.0 plugin (member-run, start.md,
// team.md) running against this binary until /lets:update loads the matching one.
// Every verb exits 0 with the v0.10.0 envelope - every key and JSON type kept,
// values the stub cannot know inert - plus a warn step and `deprecated`.

const (
	membersDeprecated = "removed next release - members are tracked by ListAgents and the caller's run record / team file (member-run); run /lets:update to load the matching plugin"
	membersLiveStatus = "live"
	membersInProcess  = "in_process"
	membersLinkTeam   = "team"
)

type membersStubStep struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// membersStubEnvelope is the v0.10.0 envelope plus `deprecated`.
type membersStubEnvelope struct {
	SchemaVersion int               `json:"schema_version"`
	OK            bool              `json:"ok"`
	Subcommand    string            `json:"subcommand"`
	Scope         string            `json:"scope"`
	Steps         []membersStubStep `json:"steps"`
	Deprecated    string            `json:"deprecated"`
}

// membersStubLead keeps v0.10.0's lead keys: session, name, pid, set, status.
type membersStubLead struct {
	Session string    `json:"session"`
	Name    string    `json:"name"`
	Pid     int       `json:"pid"`
	Set     time.Time `json:"set"`
	Status  string    `json:"status"`
}

// membersStubMember keeps v0.10.0's member keys and types.
type membersStubMember struct {
	Name           string    `json:"name"`
	AgentName      string    `json:"agent_name"`
	AgentID        string    `json:"agent_id,omitempty"`
	Role           string    `json:"role"`
	Model          string    `json:"model"`
	Isolation      string    `json:"isolation"`
	WorktreePath   string    `json:"worktree_path"`
	WorktreeBranch string    `json:"worktree_branch"`
	Kind           string    `json:"kind"`
	Session        string    `json:"session"`
	Pid            int       `json:"pid"`
	Set            time.Time `json:"set"`
	CallerToplevel string    `json:"caller_toplevel"`
	LeadPid        int       `json:"lead_pid"`
	Link           string    `json:"link"`
	Status         string    `json:"status"`
}

type membersStubAdd struct {
	membersStubEnvelope
	Member *membersStubMember `json:"member,omitempty"`
}

type membersStubDismiss struct {
	membersStubEnvelope
	Dismissed []string `json:"dismissed"`
}

type membersStubStatus struct {
	membersStubEnvelope
	Lead    *membersStubLead    `json:"lead"`
	Members []membersStubMember `json:"members"`
}

type membersStubLeadResult struct {
	membersStubEnvelope
	Lead    *membersStubLead `json:"lead,omitempty"`
	Claimed bool             `json:"claimed"`
}

func membersEnvelope(verb, scope string) membersStubEnvelope {
	return membersStubEnvelope{
		SchemaVersion: 1, OK: true, Subcommand: verb, Scope: scope, Deprecated: membersDeprecated,
		Steps: []membersStubStep{{Status: "warn", Message: "lets members is deprecated and records nothing: " + verb + " answered for compatibility only"}},
	}
}

// membersAgentName is the name the harness runs a member under: bare in an execute
// run's scope, <callsign>-<name> in a team's.
func membersAgentName(scope, name string) string {
	if strings.HasPrefix(scope, "run-") {
		return name
	}
	return scope + "-" + name
}

// membersStubLeadFor is the lead v0.10.0 readers need to go on: this session, live,
// for a run scope or when the caller's worktree is the one team file <scope>
// claims; nil otherwise (D1 disband from the main checkout then takes its "no lead
// record" branch).
func membersStubLeadFor(scope string) *membersStubLead {
	lead := &membersStubLead{Session: os.Getenv("CLAUDE_CODE_SESSION_ID"), Status: membersLiveStatus}
	if strings.HasPrefix(scope, "run-") {
		return lead
	}
	if scope == "" {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	top := gitutil.ProjectRoot(cwd, 2*time.Second)
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if top == "" || mainRoot == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gitDir, err := exec.CommandContext(ctx, "git", "-C", top, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return nil
	}
	team, ok, _, err := teamfile.FindByWorktree(filepath.Join(mainRoot, ".lets", "teams"), top, strings.TrimSpace(string(gitDir)))
	if err != nil || !ok || team != scope {
		return nil
	}
	return lead
}

// membersPrint writes res as one JSON object, or one line for a terminal.
func membersPrint(cmd *cobra.Command, jsonOut bool, res any, line string) error {
	if jsonOut {
		b, _ := json.Marshal(res)
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), line)
	return nil
}

func membersLeadLine(l *membersStubLead) string {
	if l == nil {
		return "no lead"
	}
	return "lead " + l.Status
}

// NewMembersCmd builds the deprecated `lets members` stub (add / dismiss / status /
// lead) with the v0.10.0 flag sets; it records nothing and always exits 0.
func NewMembersCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "members",
		Short:         "Deprecated: records nothing (members are tracked by ListAgents and member-run)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newMembersAddCmd(), newMembersDismissCmd(), newMembersStatusCmd(), newMembersLeadCmd())
	return root
}

// membersVerb is a stub verb: no args, unknown flags accepted.
func membersVerb(use, short string, run func(cmd *cobra.Command) error) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		Args:               cobra.NoArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE:               func(cmd *cobra.Command, _ []string) error { return run(cmd) },
	}
}

func newMembersAddCmd() *cobra.Command {
	var scope, name, role, model, isolation, wtPath, wtBranch, link, agentID, cwd string
	var jsonOut bool
	cmd := membersVerb("add", "Deprecated: answers as if the member were recorded", func(cmd *cobra.Command) error {
		res := membersStubAdd{membersStubEnvelope: membersEnvelope("add", scope), Member: &membersStubMember{
			Name: name, AgentName: membersAgentName(scope, name), AgentID: agentID, Role: role, Model: model,
			Isolation: isolation, WorktreePath: wtPath, WorktreeBranch: wtBranch,
			Kind: membersInProcess, Link: link, Status: membersLiveStatus,
		}}
		return membersPrint(cmd, jsonOut, res, res.Steps[0].Message+" - "+name+" "+membersInProcess)
	})
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.StringVar(&name, "name", "", "Member name")
	f.StringVar(&role, "role", "", "Agent role")
	f.StringVar(&model, "model", "", "Model the member runs on")
	f.StringVar(&isolation, "isolation", "", "worktree for an isolated member, else empty")
	f.StringVar(&wtPath, "worktree-path", "", "Absolute path of an isolated member's worktree")
	f.StringVar(&wtBranch, "worktree-branch", "", "Branch of an isolated member's worktree")
	f.StringVar(&link, "link", membersLinkTeam, "team | peer")
	f.StringVar(&agentID, "agent-id", "", "Id an isolated Agent call returned")
	f.StringVar(&cwd, "cwd", "", "Where a pane member's session runs (ignored)")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

func newMembersDismissCmd() *cobra.Command {
	var scope, name string
	var all, jsonOut bool
	cmd := membersVerb("dismiss", "Deprecated: answers as if the member were dismissed", func(cmd *cobra.Command) error {
		res := membersStubDismiss{membersStubEnvelope: membersEnvelope("dismiss", scope), Dismissed: []string{}}
		if name != "" && !all {
			res.Dismissed = []string{name}
		}
		return membersPrint(cmd, jsonOut, res, fmt.Sprintf("%s - dismissed: %v", res.Steps[0].Message, res.Dismissed))
	})
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.StringVar(&name, "name", "", "Member to dismiss")
	f.BoolVar(&all, "all", false, "Dismiss every member of the scope")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

func newMembersStatusCmd() *cobra.Command {
	var scope, name string
	var jsonOut bool
	cmd := membersVerb("status", "Deprecated: answers a live lead and, with --name, a live member", func(cmd *cobra.Command) error {
		res := membersStubStatus{membersStubEnvelope: membersEnvelope("status", scope), Lead: membersStubLeadFor(scope), Members: []membersStubMember{}}
		line := res.Steps[0].Message + " - " + membersLeadLine(res.Lead)
		if name != "" {
			res.Members = append(res.Members, membersStubMember{Name: name, AgentName: membersAgentName(scope, name), Kind: membersInProcess, Link: membersLinkTeam, Status: membersLiveStatus})
			line += ", " + name + " " + membersLiveStatus
		}
		return membersPrint(cmd, jsonOut, res, line)
	})
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.StringVar(&name, "name", "", "Only this member")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}

func newMembersLeadCmd() *cobra.Command {
	var scope string
	var claim, jsonOut bool
	cmd := membersVerb("lead", "Deprecated: answers this session as a live lead where v0.10.0 needs one", func(cmd *cobra.Command) error {
		res := membersStubLeadResult{membersStubEnvelope: membersEnvelope("lead", scope), Lead: membersStubLeadFor(scope), Claimed: claim}
		return membersPrint(cmd, jsonOut, res, fmt.Sprintf("%s - %s, claimed %v", res.Steps[0].Message, membersLeadLine(res.Lead), claim))
	})
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "Team callsign or run-<RUN>")
	f.BoolVar(&claim, "claim", false, "Record this session as the lead (records nothing)")
	f.BoolVar(&jsonOut, "json", false, "Emit a JSON envelope")
	return cmd
}
