//go:build unix

package peerscmd

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// move is one correction to a role file: its holder's session id was re-minted in
// the same process (from.Session -> to.Session), or its live session runs under a
// new pid (same session, to.Pid). drop: the new id already has its own file, which
// is newer; the old one only goes.
type move struct {
	from, to roleFile
	drop     bool
}

// reconcileRoles finds the role files the registry shows under another id or pid.
// It never judges death - pruneRoles does, after these moves - so a file whose id
// was re-minted is carried to the new id instead of being pruned as a reused pid.
// Whatever re-minted the id (/clear, an in-session /resume, a harness duplicate),
// the process is the anchor, so any caller can make the move, not only the holder.
func reconcileRoles(files map[string]roleFile, snap ccregistry.Snapshot) []move {
	sids := make([]string, 0, len(files))
	taken := map[string]bool{}
	for sid := range files {
		sids = append(sids, sid)
		taken[sid] = true
	}
	sort.Strings(sids)
	var out []move
	for _, sid := range sids {
		f := files[sid]
		if e, ok := snap.Find(sid); ok {
			// Same session, new pid (`claude -r`): refresh pid, and set, so a later
			// /clear in THIS process still passes Rotated's started-before-set test.
			// A duplicate (the recorded pid still carries the session) is left alone.
			if cur, ok := snap.FindPid(f.Pid); !ok || cur.SessionID != sid {
				to := f
				to.Pid, to.Set = e.Pid, now().UTC().Format(time.RFC3339)
				out = append(out, move{from: f, to: to})
			}
			continue
		}
		set, err := time.Parse(time.RFC3339, f.Set)
		if err != nil {
			continue
		}
		newSid, ok := snap.Rotated(sid, f.Pid, set)
		if !ok {
			continue
		}
		to := f
		to.Session, to.path = newSid, filepath.Join(filepath.Dir(f.path), newSid+".role")
		out = append(out, move{from: f, to: to, drop: taken[newSid]})
		taken[newSid] = true
	}
	return out
}

// applyMoves rewrites files in memory; a read-only load (another project) stops here.
func applyMoves(files map[string]roleFile, moves []move) {
	for _, m := range moves {
		delete(files, m.from.Session)
		if !m.drop {
			files[m.to.Session] = m.to
		}
	}
}

// persistMoves writes moves to disk. The caller holds peers.lock. A move another
// session already made is harmless: the write is idempotent, a missing old file ok.
func persistMoves(root string, moves []move) error {
	for _, m := range moves {
		if !m.drop {
			if err := writeRole(root, m.to); err != nil {
				return err
			}
		}
		if m.drop || m.to.Session != m.from.Session {
			if err := os.Remove(m.from.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
