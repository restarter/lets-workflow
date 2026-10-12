package peertail

import "time"

// replyTurnsMax bounds a read with Since and no Last: a reply is relayed whole, so
// the default five would cut its beginning off.
const replyTurnsMax = 100

// Options: one session's transcript, already resolved by the caller.
type Options struct {
	ClaudeDir string    // ccregistry.HomeDir()
	Cwd       string    // the session's cwd from the registry row
	Session   string    // the session id
	Last      int       // 0 = 5 turns; with Since non-zero: up to replyTurnsMax
	Since     time.Time // zero = no bound; else only turns timestamped at or after it
}

// Result is what Read returns: the turns, what the limits left out, and every source
// that could not be read in full.
type Result struct {
	Turns          []Turn
	Omitted        int
	TruncatedBytes int
	Note           string
	Degraded       []Degraded
}

// Read returns a session's recent transcript turns, redacted and capped. It does no
// resolution, no git and no Orca: the caller passes the session it already resolved.
func Read(o Options) Result {
	res := Result{Turns: []Turn{}}
	limit := o.Last
	if limit <= 0 {
		limit = 5
		if !o.Since.IsZero() {
			limit = replyTurnsMax
		}
	}
	path, d := LocateTranscript(o.ClaudeDir, o.Cwd, o.Session)
	if d != nil {
		res.Degraded = append(res.Degraded, *d)
		return res
	}
	recs, d := readAll(path)
	if d != nil {
		res.Degraded = append(res.Degraded, *d)
		return res
	}
	perTurn := textCapDefault
	if !o.Since.IsZero() {
		recs = since(recs, o.Since)
		perTurn = textCapMessage // a reply: the payload IS the point
	}
	turns := turnsOf(recs, perTurn)
	if len(turns) > limit {
		res.Omitted = len(turns) - limit
		turns = turns[len(turns)-limit:]
	}
	// The call ceiling: drop the oldest turns once their sum would exceed
	// callCapBytes. The turn that breaks the budget is dropped, not kept - keeping it
	// would let the reply exceed the ceiling by a whole turn. The newest turn is never
	// dropped (i < len(turns)-1): the per-turn cap is well under callCapBytes, so one
	// turn alone can never overflow it.
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
			res.TruncatedBytes += t.srcLen
		}
		res.Omitted += len(dropped)
		turns = turns[len(turns)-keep:]
	}
	for _, t := range turns {
		res.TruncatedBytes += t.TruncatedBytes
	}
	res.Turns = turns
	return res
}

// since keeps the records timestamped at or after bound. Timestamps are compared as
// instants, never as strings: fractional precision and offsets break lexical order.
// A record with no or a malformed timestamp is dropped.
func since(recs []record, bound time.Time) []record {
	var out []record
	for _, r := range recs {
		ts, err := time.Parse(time.RFC3339Nano, r.turn.TS)
		if err != nil || ts.Before(bound) {
			continue
		}
		out = append(out, r)
	}
	return out
}
