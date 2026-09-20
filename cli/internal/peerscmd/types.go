// Package peerscmd implements `lets peers`: list the live Claude Code sessions of a
// repo, read their output safely, frame and send messages, wait for a reply, and
// keep the role registry (orchestrators, workers, peers). Two sources: the Claude
// Code session registry and transcripts (always), and Orca terminals (only when
// LETS_LAUNCHER=orca or --probe-orca). Every unknown format degrades by name.
package peerscmd

// Peer is one live session of the repo as `who` reports it.
type Peer struct {
	Role         string   `json:"role,omitempty"`  // orchestrator | worker | peer ("" = no role file)
	Name         string   `json:"name,omitempty"`  // the live registry name (display; never an address)
	Scope        string   `json:"scope,omitempty"` // an orchestrator's declared part of the repo
	Session      string   `json:"session,omitempty"`
	Session6     string   `json:"session6,omitempty"`
	TerminalID   string   `json:"terminal_id,omitempty"` // the Orca terminal handle, when joined or Orca-only
	Task         string   `json:"task,omitempty"`
	Orc          string   `json:"orc,omitempty"` // a worker's bound orchestrator name (from .task-<slug>)
	Branch       string   `json:"branch,omitempty"`
	Cwd          string   `json:"cwd,omitempty"`
	State        string   `json:"state,omitempty"` // registry status, or the Orca agent state
	AgentType    string   `json:"agent_type,omitempty"`
	LastActivity string   `json:"last_activity,omitempty"`
	Alive        string   `json:"alive"`          // alive | dead | unknown
	Via          []string `json:"via"`            // claude, orca
	Send         string   `json:"send,omitempty"` // orca | claude | none (computed in who/peers; absent when not computed)
	Reason       string   `json:"reason,omitempty"`
	RepoIndex    *int     `json:"repo_index,omitempty"` // who --orca-repos: which Orca repo this row belongs to
}

// LastOrchestrator is an orchestrator that is not running, as the hub needs it to
// wake or ask it: from a last-seen file, or a dead role file not yet pruned.
type LastOrchestrator struct {
	RepoIndex *int   `json:"repo_index,omitempty"`
	Name      string `json:"name"`
	Scope     string `json:"scope,omitempty"`
	Session   string `json:"session,omitempty"`
	Session6  string `json:"session6,omitempty"`
	Pid       *int   `json:"pid,omitempty"`
	Source    string `json:"source"` // last_seen | role_file
	Note      string `json:"note,omitempty"`
}

// Degraded names a source that could not be read in full.
type Degraded struct {
	Source string `json:"source"` // claude | orca | transcript | roles | git | context | repo
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Turn is one recognized transcript record, already redacted and capped.
type Turn struct {
	TS             string `json:"ts,omitempty"`
	Kind           string `json:"kind"` // TEXT | INBOUND | TOOL | RESULT | SCREEN
	Role           string `json:"role,omitempty"`
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
