//go:build unix

package peerscmd

import (
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/peertail"
)

// ErrorInfo is the typed error of a failed envelope.
type ErrorInfo struct {
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// deprecatedText is the one line every v0.10.0 compatibility envelope carries.
const deprecatedText = "removed next release - peer messages go through SendMessage (/lets:orc); run /lets:update to load the matching plugin"

// Envelope is the core of every `lets peers` result. A degraded source keeps ok=true
// and names itself in degraded[]. Deprecated is set on a verb (or flag) that still
// works for this release only.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Subcommand    string     `json:"subcommand"`
	Deprecated    string     `json:"deprecated,omitempty"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Degraded      []Degraded `json:"degraded"`
}

func newEnvelope(sub string) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, Subcommand: sub, Degraded: []Degraded{}}
}

// WhoResult lists the repo's peers.
type WhoResult struct {
	Envelope
	Peers             []Peer             `json:"peers"`
	Self              *Peer              `json:"self,omitempty"`
	LastOrchestrators []LastOrchestrator `json:"last_orchestrators,omitempty"`
	Repos             []orcacmd.RepoInfo `json:"repos,omitempty"`
	// RepoSlug is <repo> of every role name (peername.RepoSlug of the main checkout).
	RepoSlug string `json:"repo_slug"`
	Standing
}

// Standing is the process-bound standing answer for one inbound message, present
// only when --from-name / --from-addr were passed: StandingOK is then always in the
// envelope (false included), StandingReason names why it is false.
type Standing struct {
	StandingOK     *bool  `json:"standing_ok,omitempty"`
	StandingReason string `json:"standing_reason,omitempty"`
}

func standingOf(ok bool, reason string) Standing {
	return Standing{StandingOK: &ok, StandingReason: reason}
}

// TailResult is a session's recent output.
type TailResult struct {
	Envelope
	Session string          `json:"session,omitempty"`
	Turns   []peertail.Turn `json:"turns"`
	Omitted int             `json:"omitted,omitempty"` // earlier turns the limit left out
	Note    string          `json:"note,omitempty"`
	// TruncatedBytes counts SOURCE bytes lost to either cut: bytes removed inside a
	// turn that was kept, plus the full SOURCE size of a turn the call ceiling
	// dropped. So a whole-turn drop moves both Omitted and this counter - omitted: 0
	// does not imply a complete answer on its own.
	TruncatedBytes int `json:"truncated_bytes,omitempty"`
}

// FrameResult is a framed, addressed message slot (deprecated: v0.10.0 skills).
type FrameResult struct {
	Envelope
	Header      string `json:"header"`
	MsgID       string `json:"msgid"`
	SentAt      string `json:"sent_at"`
	HandoffPath string `json:"handoff_path"`
}

// TellResult is the deprecated `tell`: it never types anything - it hands the framed
// text back (route claude, reason claude_transport_model_send) for the v0.10.0
// skill's SendMessage.
type TellResult struct {
	Envelope
	Delivered bool   `json:"delivered"`
	Route     string `json:"route"` // claude | none
	Reason    string `json:"reason,omitempty"`
	Text      string `json:"text,omitempty"` // the framed text, header line included
}

// WaitResult is the deprecated `wait`: completion is never verified any more.
type WaitResult struct {
	Envelope
	Satisfied bool   `json:"satisfied"`
	Reason    string `json:"reason,omitempty"`
}

// RoleInfo is a `role set` answer: nothing is recorded (v0.10.0 compatibility).
type RoleInfo struct {
	Granted     bool   `json:"granted"`
	Role        string `json:"role,omitempty"` // the word the caller passed; it means nothing any more
	Reason      string `json:"reason,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Registered  bool   `json:"registered"`
}

// RoleResult is a `role set` / `role clear` (deprecated: records nothing).
type RoleResult struct {
	Envelope
	Role *RoleInfo `json:"role,omitempty"`
}

// OrchestratorResult is the resolver's answer. SelfRecord ("is this session an
// orchestrator") is always present - null when the caller has no record; Source /
// Target answer "whom do my messages go to".
type OrchestratorResult struct {
	Envelope
	SelfRecord  *SelfRecord `json:"self_record"`
	Source      string      `json:"source"`
	Scope       string      `json:"scope"`
	Target      *Peer       `json:"target,omitempty"`
	Candidates  []Candidate `json:"candidates"`
	Reason      string      `json:"reason,omitempty"`
	Remediation string      `json:"remediation,omitempty"` // the exact line to show the user; set with every Reason
	Refused     []Refused   `json:"refused,omitempty"`
	Standing
}
