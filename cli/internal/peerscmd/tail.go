//go:build unix

package peerscmd

import (
	"context"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/peertail"
)

// TailOptions configures Tail: Cwd, exactly one of Name (an exact live name of this
// repo) or ToSession, Last, Since. SinceMessage + SentAt are the compatibility-release
// reply read; RepoIndex is the retired cross-repo read; Removed names v0.10.0 flags
// the caller passed that no longer exist.
type TailOptions struct {
	Cwd          string
	Name         string // a live peer's exact name, resolved here
	ToSession    string
	Last         int    // 0: 5 turns, or the whole reply (up to 100) with Since / SinceMessage
	Since        string // RFC 3339: only the turns timestamped at or after it
	SinceMessage string // deprecated: the turns after this legacy msgid reached the peer
	SentAt       string
	RepoIndex    *int     // retired: an index from `lets orca repos` (nil = unset)
	Removed      []string // v0.10.0 flags given (--to-terminal, --addressed-to-session, ...)
}

// crossRepoRetired is the message a v0.10.0 cross-repo `tail --repo-index` gets: the
// v0.10.0 skill shows a failed envelope's error, so the remedy lives in the message.
const crossRepoRetired = "reading another repo's session was removed - update the LETS plugin (/lets:update); the peer can answer with /lets:orc, which arrives here as a cross-session message"

// Tail reads a session's recent transcript turns, redacted and capped. The session
// must be one of this repo's live registry rows - the peers carve-out reads only this
// repo's transcripts. There is no full-output mode for tool results.
func Tail(ctx context.Context, o TailOptions) (*TailResult, error) {
	res := &TailResult{Envelope: newEnvelope("tail"), Turns: []peertail.Turn{}}
	fail := func(e *Error) (*TailResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if o.RepoIndex != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "cross_repo_read_retired", Message: crossRepoRetired})
	}
	if len(o.Removed) > 0 {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: strings.Join(o.Removed, ", ") + " no longer exist - read a session with <name> or --to-session, and a reply with --since <RFC 3339>"})
	}
	if (o.Name == "") == (o.ToSession == "") {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "pass exactly one of <name> or --to-session"})
	}
	if o.Name != "" && !ccregistry.ValidName(o.Name) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "not a peer name"})
	}
	if o.ToSession != "" && !ccregistry.ValidSession(o.ToSession) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--to-session is not a session id"})
	}
	if (o.SinceMessage == "") != (o.SentAt == "") {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--since-message needs --sent-at"})
	}
	if o.Since != "" && o.SinceMessage != "" {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--since and --since-message conflict"})
	}
	var since time.Time
	if o.Since != "" {
		t, err := time.Parse(time.RFC3339Nano, o.Since)
		if err != nil {
			return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--since is not an RFC 3339 time"})
		}
		since = t
	}
	mainRoot, e := mainRootOf(o.Cwd)
	if e != nil {
		return fail(e)
	}
	entries, _, _, _, degraded := registryPeers(ctx, mainRoot)
	res.Degraded = append(res.Degraded, degraded...)
	hits := map[string]ccregistry.Entry{}
	for _, e := range entries {
		if (o.Name != "" && e.NameOK && e.Name == o.Name) || (o.ToSession != "" && e.SessionID == o.ToSession) {
			if _, dup := hits[e.SessionID]; !dup {
				hits[e.SessionID] = e
			}
		}
	}
	switch {
	case len(hits) == 0 && o.Name != "":
		return fail(&Error{Code: ExitGeneric, Kind: "peer_not_found", Message: "no live peer named " + o.Name + " in this repo"})
	case len(hits) == 0:
		return fail(&Error{Code: ExitGeneric, Kind: "peer_not_found", Message: "no live session " + session6(o.ToSession) + " in this repo"})
	case len(hits) > 1:
		return fail(&Error{Code: ExitGeneric, Kind: "peer_ambiguous", Message: "several live peers are named " + o.Name + " - use --to-session"})
	}
	var target ccregistry.Entry
	for _, e := range hits {
		target = e
	}
	res.OK = true
	res.Session = target.SessionID
	if o.SinceMessage != "" {
		res.Deprecated = deprecatedText
	}
	r := peertail.Read(peertail.Options{ClaudeDir: ccregistry.HomeDir(), Cwd: target.Cwd, Session: target.SessionID, Last: o.Last, Since: since,
		SinceMessage: o.SinceMessage, SentAt: o.SentAt})
	res.Turns, res.Omitted, res.TruncatedBytes, res.Note = r.Turns, r.Omitted, r.TruncatedBytes, r.Note
	for _, d := range r.Degraded {
		res.Degraded = append(res.Degraded, Degraded{Source: d.Source, Reason: d.Reason, Detail: d.Detail})
	}
	return res, nil
}
