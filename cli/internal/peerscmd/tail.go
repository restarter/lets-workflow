//go:build unix

package peerscmd

import (
	"context"
	"os"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/peertail"
)

// TailOptions configures Tail. The `tail` verb: Cwd, exactly one of Name (an exact
// live name of this repo) or ToSession, Last, Since; SinceMessage + SentAt are the
// compatibility-release reply read. ToTerminal, AddressedToSession, CountOnly,
// ProbeOrca and Repo belong to the old role / Orca path (tailLegacy), which goes in
// the next step of lets-0xsw2; RepoIndex is the retired cross-repo read.
type TailOptions struct {
	Cwd                string
	Name               string // a live peer's exact name, resolved here
	ToSession          string
	ToTerminal         string // old path
	Last               int    // 0: 5 turns, or the whole reply (up to replyTurnsMax) with Since / SinceMessage
	Since              string // RFC 3339: only the turns timestamped at or after it
	SinceMessage       string // deprecated: the turns after this legacy msgid reached the peer
	SentAt             string
	AddressedToSession string // old path
	CountOnly          bool   // old path
	ProbeOrca          bool   // old path
	Repo               string // old path: the peer's project, when it is not this one
	RepoIndex          *int   // retired: an index from `lets orca repos` (nil = unset)
}

// crossRepoRetired is the message a v0.10.0 cross-repo `tail --repo-index` gets: the
// v0.10.0 skill shows a failed envelope's error, so the remedy lives in the message.
const crossRepoRetired = "reading another repo's session was removed - update the LETS plugin (/lets:update); the peer can answer with /lets:orc, which arrives here as a cross-session message"

// sinceMessageDeprecated is the envelope's deprecated line for --since-message.
const sinceMessageDeprecated = "--since-message / --sent-at are deprecated and go in the next release; read a reply with --since <RFC 3339>"

// replyTurnsMax bounds a reply read with --since-message and no --last: the reply is
// relayed whole, so the default five would cut its beginning off.
const replyTurnsMax = 100

// Tail reads a session's recent transcript turns, redacted and capped. The session
// must be one of this repo's live registry rows - the peers carve-out reads only this
// repo's transcripts. There is no full-output mode for tool results.
func Tail(ctx context.Context, o TailOptions) (*TailResult, error) {
	res := &TailResult{Envelope: newEnvelope("tail"), Turns: []Turn{}}
	fail := func(e *Error) (*TailResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if o.RepoIndex != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "cross_repo_read_retired", Message: crossRepoRetired})
	}
	if o.Repo != "" || o.ToTerminal != "" || o.AddressedToSession != "" || o.CountOnly || o.ProbeOrca {
		return tailLegacy(ctx, o)
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
	cwd := o.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if gitutil.ProjectRoot(cwd, 2*time.Second) == "" || mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	entries, _, _, _, degraded := registryPeers(ctx, mainRoot)
	res.Degraded = append(res.Degraded, degraded...)
	var target *ccregistry.Entry
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
	for _, e := range hits {
		target = &e
	}
	res.OK = true
	res.Session = target.SessionID
	if o.SinceMessage != "" {
		res.Deprecated = sinceMessageDeprecated
		tailSinceMessage(res, target.Cwd, target.SessionID, o)
		return res, nil
	}
	r := peertail.Read(peertail.Options{ClaudeDir: ccregistry.HomeDir(), Cwd: target.Cwd, Session: target.SessionID, Last: o.Last, Since: since})
	res.Turns, res.Omitted, res.TruncatedBytes, res.Note = fromPeertail(r.Turns), r.Omitted, r.TruncatedBytes, r.Note
	for _, d := range r.Degraded {
		res.Degraded = append(res.Degraded, Degraded{Source: d.Source, Reason: d.Reason, Detail: d.Detail})
	}
	return res, nil
}

// tailSinceMessage is the compatibility-release reply read (--since-message
// --sent-at): the turns after the INBOUND record whose legacy header id equals the
// msgid, at or after sent-at; not seen yet -> no turns and a note, never a time-only
// fallback (a held or delayed delivery would relay what happened before the question).
func tailSinceMessage(res *TailResult, cwd, sid string, o TailOptions) {
	path, d := LocateTranscript(ccregistry.HomeDir(), cwd, sid)
	if d != nil {
		res.Degraded = append(res.Degraded, *d)
		return
	}
	recs, d := readAll(path)
	if d != nil {
		res.Degraded = append(res.Degraded, *d)
		return
	}
	after, ok := SinceMessage(recs, o.SinceMessage, o.SentAt)
	if !ok {
		res.Note = "message not seen yet"
		return
	}
	limit := o.Last
	if limit <= 0 {
		limit = replyTurnsMax
	}
	turns, omitted, truncated := capTurns(turnsOf(after, textCapMessage), limit)
	res.Turns, res.Omitted, res.TruncatedBytes = turns, omitted, truncated
}

// fromPeertail converts peertail's turns to the envelope's (same JSON shape).
func fromPeertail(turns []peertail.Turn) []Turn {
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		out = append(out, Turn{TS: t.TS, Kind: t.Kind, Role: t.Role, From: t.From, Tool: t.Tool, Text: t.Text, TruncatedBytes: t.TruncatedBytes})
	}
	return out
}

// capTurns keeps the newest limit turns, then applies the call ceiling: the oldest
// turns are dropped once their sum would exceed callCapBytes. The turn that breaks
// the budget is dropped, not kept - keeping it would let the reply exceed the ceiling
// by a whole turn. The newest turn is never dropped (i < len(turns)-1): the per-turn
// cap is well under callCapBytes, so one turn alone can never overflow it. omitted
// counts every turn left out; truncated counts source bytes lost to either cut.
func capTurns(turns []Turn, limit int) (kept []Turn, omitted, truncated int) {
	if len(turns) > limit {
		omitted = len(turns) - limit
		turns = turns[len(turns)-limit:]
	}
	total, keep := 0, len(turns)
	for i := len(turns) - 1; i >= 0; i-- { // newest first
		if total+len(turns[i].Text) > callCapBytes && i < len(turns)-1 {
			keep = len(turns) - 1 - i // turns[i] broke the budget: drop it and everything older
			break
		}
		total += len(turns[i].Text)
	}
	if keep < len(turns) {
		dropped := turns[:len(turns)-keep]
		for _, t := range dropped {
			// srcLen is the turn's true pre-cap source size. len(t.Text) would
			// undercount a turn the per-turn cap already shortened (missing what that
			// cap cut), and len(t.Text)+t.TruncatedBytes would OVERcount by
			// redact.Cap's own marker, which len(t.Text) already includes once a turn
			// has been capped. srcLen carries neither error.
			truncated += t.srcLen
		}
		omitted += len(dropped)
		turns = turns[len(turns)-keep:]
	}
	for _, t := range turns {
		truncated += t.TruncatedBytes
	}
	return turns, omitted, truncated
}

// tailLegacy is the old role / Orca tail (screen, --repo, --addressed-to-session),
// reached only from Go callers that still set those fields. It goes with that path.
func tailLegacy(ctx context.Context, o TailOptions) (*TailResult, error) {
	res := &TailResult{Envelope: newEnvelope("tail"), Turns: []Turn{}}
	fail := func(e *Error) (*TailResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	targets := 0
	for _, s := range []string{o.Name, o.ToSession, o.ToTerminal} {
		if s != "" {
			targets++
		}
	}
	if targets != 1 {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "pass exactly one of <name>, --to-session or --to-terminal"})
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
	if o.CountOnly && o.AddressedToSession == "" {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--count-only needs --addressed-to-session"})
	}
	limit := o.Last
	if limit <= 0 {
		limit = 5
		if o.SinceMessage != "" {
			limit = replyTurnsMax
		}
	}
	rc, err := loadRepo(ctx, o.Cwd, o.ProbeOrca)
	if err != nil {
		return fail(err.(*Error))
	}
	rc, e := peerRepo(ctx, rc, o.Repo, o.RepoIndex, o.ProbeOrca)
	if e != nil {
		return fail(e)
	}
	res.Degraded = rc.degraded
	if o.Name != "" {
		var hits []Peer
		for _, p := range rc.peers(ctx) {
			if p.Name == o.Name {
				hits = append(hits, p)
			}
		}
		switch len(hits) {
		case 0:
			return fail(&Error{Code: ExitGeneric, Kind: "peer_not_found", Message: "no live peer named " + o.Name})
		case 1:
			o.ToSession, o.ToTerminal = hits[0].Session, ""
			if o.ToSession == "" {
				o.ToTerminal = hits[0].TerminalID
			}
		default:
			return fail(&Error{Code: ExitGeneric, Kind: "peer_ambiguous", Message: "several live peers are named " + o.Name + " - use --to-session"})
		}
	}
	res.OK = true

	if o.ToTerminal != "" {
		res.TerminalID = o.ToTerminal
		res.Note = "no transcript (non-Claude agent)"
		if rc.ops == nil {
			res.Degraded = append(res.Degraded, Degraded{Source: "orca", Reason: "orca_not_selected"})
			return res, nil
		}
		lines, d := ScreenTail(ctx, rc.ops, o.ToTerminal)
		if d != nil {
			res.Degraded = append(res.Degraded, *d)
		}
		res.Screen = lines
		return res, nil
	}

	var peer *Peer
	for _, p := range rc.peers(ctx) {
		if p.Session == o.ToSession {
			pp := p
			peer = &pp
			break
		}
	}
	if peer == nil {
		res.OK = false
		e := &Error{Code: ExitGeneric, Kind: "peer_not_found", Message: "no live session " + session6(o.ToSession) + " in this repo"}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	res.Session = o.ToSession
	path, d := LocateTranscript(ccregistry.HomeDir(), peer.Cwd, o.ToSession)
	if d != nil {
		res.Degraded = append(res.Degraded, *d)
	} else {
		recs, d := readAll(path)
		if d != nil {
			res.Degraded = append(res.Degraded, *d)
		} else {
			switch {
			case o.AddressedToSession != "":
				hits := AddressedTo(recs, o.AddressedToSession)
				if o.CountOnly {
					res.AddressedToMe = &AddressedCount{Count: len(hits)}
					if len(hits) > 0 {
						res.AddressedToMe.LastAt = hits[len(hits)-1].turn.TS
					}
					return res, nil
				}
				recs = hits
			case o.SinceMessage != "":
				after, ok := SinceMessage(recs, o.SinceMessage, o.SentAt)
				if !ok {
					res.Note = "message not seen yet"
				}
				recs = after
			}
			perTurn := textCapDefault
			if o.SinceMessage != "" || o.AddressedToSession != "" {
				perTurn = textCapMessage // the reply to an ask, or a message addressed to the reader
			}
			turns, omitted, truncated := capTurns(turnsOf(recs, perTurn), limit)
			res.Turns, res.Omitted, res.TruncatedBytes = turns, omitted, truncated
		}
	}
	if peer.TerminalID != "" && rc.ops != nil {
		lines, d := ScreenTail(ctx, rc.ops, peer.TerminalID)
		if d != nil {
			res.Degraded = append(res.Degraded, *d)
		}
		res.Screen = lines
		res.TerminalID = peer.TerminalID
	}
	return res, nil
}
