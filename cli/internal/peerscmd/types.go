// Package peerscmd implements `lets peers`: list the live Claude Code sessions of a
// repo from the session registry, read their transcripts safely (through peertail),
// and resolve which orchestrator a session answers to - by session NAME, with the
// orchestrators recorded in the merge-branch task-state. Messages go through the
// harness's SendMessage from the orc skill; the v0.10.0 frame / tell / wait verbs
// remain as deprecated stubs for one release. Every unknown format degrades by name.
package peerscmd

// Peer is one live session of the repo as `who` reports it.
type Peer struct {
	Name         string `json:"name,omitempty"`         // the live registry name (the SendMessage address)
	Orchestrator bool   `json:"orchestrator,omitempty"` // a recorded orchestrator name, or (none recorded live) <repo>-orc[-scope]
	Scope        string `json:"scope,omitempty"`        // that orchestrator's part of the repo
	Session      string `json:"session,omitempty"`
	Session6     string `json:"session6,omitempty"`
	Task         string `json:"task,omitempty"` // the branch's task-state task:
	Orc          string `json:"orc,omitempty"`  // the branch's task-state orc: binding
	Branch       string `json:"branch,omitempty"`
	Cwd          string `json:"cwd,omitempty"`
	State        string `json:"state,omitempty"` // registry status
	LastActivity string `json:"last_activity,omitempty"`
	Alive        string `json:"alive"`                    // alive (a live registry row)
	Standing     bool   `json:"standing"`                 // its record's standing=on (unset, off and cross-repo are all false)
	Duplicate    bool   `json:"duplicate_name,omitempty"` // another live session anywhere has this name
	RepoIndex    *int   `json:"repo_index,omitempty"`     // who --orca-repos: which Orca repo this row belongs to
}

// LastOrchestrator is an orchestrator that is not running, as the hub needs it to
// wake or ask it: from its last-seen file.
type LastOrchestrator struct {
	RepoIndex *int   `json:"repo_index,omitempty"`
	Name      string `json:"name"`
	Scope     string `json:"scope,omitempty"`
	Session   string `json:"session,omitempty"`
	Session6  string `json:"session6,omitempty"`
	Pid       *int   `json:"pid,omitempty"`
	Source    string `json:"source"` // last_seen
	Note      string `json:"note,omitempty"`
}

// Degraded names a source that could not be read in full.
type Degraded struct {
	Source string `json:"source"` // claude | transcript | taskstate | git | context | repo | orca
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// SelfRecord is the caller's own recorded `orchestrator:` line of the merge-branch
// task-state. Standing keeps the record's tri-state ("" never asked, "on", "off").
type SelfRecord struct {
	Name     string `json:"name"`
	Scope    string `json:"scope,omitempty"`
	Standing string `json:"standing"`
}
