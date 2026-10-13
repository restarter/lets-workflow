// Package peertail reads one Claude Code session's recent transcript turns safely:
// it locates the transcript, parses the records it recognizes, redacts and caps
// them, and bounds the whole call. The caller resolves which session to read; this
// package does no resolution, no git and no Orca.
//
// Leaf package: the standard library plus the ccregistry, redact and peername leaves.
package peertail

// Degraded names a source that could not be read in full (same JSON shape as
// peerscmd's).
type Degraded struct {
	Source string `json:"source"` // transcript
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Turn is one recognized transcript record, already redacted and capped.
type Turn struct {
	TS             string `json:"ts,omitempty"`
	Kind           string `json:"kind"` // TEXT | INBOUND | TOOL | RESULT
	Role           string `json:"role,omitempty"`
	From           string `json:"from,omitempty"` // an INBOUND native message's from-name, when it is a valid name
	Tool           string `json:"tool,omitempty"`
	Text           string `json:"text"`
	TruncatedBytes int    `json:"truncated_bytes,omitempty"` // bytes this turn's text lost to the per-turn cap
	// srcLen is Text's length BEFORE any cap or marker - unexported, never marshaled
	// (no JSON surface change). tail.go's call ceiling uses it to count a dropped
	// turn's true source size; len(Text) alone would include redact.Cap's own
	// "…[truncated N bytes]" marker on an already-per-turn-capped turn, overcounting
	// truncated_bytes by the marker's length (lets-cbmg7 FIX E).
	srcLen int
}

// Header is the parsed legacy `[lets-peer ...]` line that opened a v0.10.0 peer
// message. Kept for the compatibility release; names are display only and
// addressing compares session ids exactly.
type Header struct {
	ID      string // 16 hex chars
	Kind    string // ask | ping | tell | ask-ro
	FromSID string
	ToSID   string
	From    string // "<role>/<live name>"
	To      string // "<live name>"
}
