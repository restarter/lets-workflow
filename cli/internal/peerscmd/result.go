//go:build unix

package peerscmd

// ErrorInfo is the typed error of a failed envelope.
type ErrorInfo struct {
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Envelope is the core of every `lets peers` result. A degraded source keeps ok=true
// and names itself in degraded[].
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Subcommand    string     `json:"subcommand"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Degraded      []Degraded `json:"degraded"`
}

func newEnvelope(sub string) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, Subcommand: sub, Degraded: []Degraded{}}
}

// WhoResult lists the repo's peers.
type WhoResult struct {
	Envelope
	Peers []Peer `json:"peers"`
	Self  *Peer  `json:"self,omitempty"`
}

// AddressedCount is `tail --count-only`: how many messages were addressed to a
// session, without their text.
type AddressedCount struct {
	Count  int    `json:"count"`
	LastAt string `json:"last_at,omitempty"`
}

// TailResult is a peer's recent output.
type TailResult struct {
	Envelope
	Session       string          `json:"session,omitempty"`
	TerminalID    string          `json:"terminal_id,omitempty"`
	Turns         []Turn          `json:"turns"`
	Screen        []string        `json:"screen,omitempty"`
	Note          string          `json:"note,omitempty"`
	AddressedToMe *AddressedCount `json:"addressed_to_me,omitempty"`
}

// FrameResult is a framed, addressed message slot.
type FrameResult struct {
	Envelope
	Header      string `json:"header"`
	MsgID       string `json:"msgid"`
	SentAt      string `json:"sent_at"`
	HandoffPath string `json:"handoff_path"`
}

// ReceiptInfo is what Orca's `terminal send` proved.
type ReceiptInfo struct {
	InputAccepted bool `json:"input_accepted"`
	TurnStarted   bool `json:"turn_started"`
}

// TellResult is a send attempt.
type TellResult struct {
	Envelope
	Delivered bool         `json:"delivered"`
	Route     string       `json:"route"` // orca | claude | none
	Reason    string       `json:"reason,omitempty"`
	State     string       `json:"state,omitempty"`
	Receipt   *ReceiptInfo `json:"receipt,omitempty"`
	SentAt    string       `json:"sent_at,omitempty"`
	Observed  bool         `json:"observed"`
	Text      string       `json:"text,omitempty"` // the framed text for the skill's SendMessage (claude route)
}

// WaitResult is a reply wait.
type WaitResult struct {
	Envelope
	Satisfied bool   `json:"satisfied"`
	Reason    string `json:"reason,omitempty"`
}

// RoleResult is a role change.
type RoleResult struct {
	Envelope
	Role *RoleInfo `json:"role,omitempty"`
}

// OrchestratorResult is the resolver's answer.
type OrchestratorResult struct {
	Envelope
	Source     string      `json:"source"`
	Scope      string      `json:"scope"`
	Target     *Peer       `json:"target,omitempty"`
	Candidates []Candidate `json:"candidates"`
	Reason     string      `json:"reason,omitempty"`
}
