//go:build unix

package peerscmd

import (
	"context"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// TailOptions configures Tail. Callers resolve a name to a session id (or, for a
// non-Claude Orca row, a terminal id) first; tail never resolves a name.
type TailOptions struct {
	Cwd                string
	ToSession          string
	ToTerminal         string
	Last               int // 0: 5 turns, or the whole reply (up to replyTurnsMax) with SinceMessage
	SinceMessage       string
	SentAt             string
	AddressedToSession string
	CountOnly          bool
	ProbeOrca          bool
	Repo               string // the peer's project, when it is not this one (the hub), or
	RepoIndex          *int   // an index from `lets orca repos` (nil = unset)
}

// replyTurnsMax bounds a reply read with --since-message and no --last: the reply is
// relayed whole, so the default five would cut its beginning off.
const replyTurnsMax = 100

// Tail reads a peer's recent output: transcript turns (plus the screen of a joined
// Orca terminal) for a session, the screen only for a terminal. There is no
// full-output mode for tool results.
func Tail(ctx context.Context, o TailOptions) (*TailResult, error) {
	res := &TailResult{Envelope: newEnvelope("tail"), Turns: []Turn{}}
	fail := func(e *Error) (*TailResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if (o.ToSession == "") == (o.ToTerminal == "") {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "pass exactly one of --to-session or --to-terminal"})
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
			turns := turnsOf(recs, perTurn)
			if len(turns) > limit {
				res.Omitted = len(turns) - limit
				turns = turns[len(turns)-limit:]
			}
			// The call ceiling: drop the oldest turns once their sum would exceed
			// callCapBytes. The turn that breaks the budget is dropped, not kept -
			// keeping it would let the reply exceed the ceiling by a whole turn. The
			// newest turn is never dropped (i < len(turns)-1): the per-turn cap is
			// well under callCapBytes, so one turn alone can never overflow it.
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
					// len(t.Text) is what the ceiling dropped (what the reader would have
					// received); t.TruncatedBytes is what the per-turn cap had ALREADY cut
					// from that same turn before it ever got here - both are source bytes
					// lost, so both count, or a heavily-capped dropped turn under-reports.
					res.TruncatedBytes += len(t.Text) + t.TruncatedBytes
				}
				res.Omitted += len(dropped)
				turns = turns[len(turns)-keep:]
			}
			for _, t := range turns {
				res.TruncatedBytes += t.TruncatedBytes
			}
			res.Turns = turns
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
