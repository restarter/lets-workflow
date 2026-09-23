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
// new pid (same session, to.Pid). drop: the old file only goes - the new id already
// has its own file, or another old file of the same process won the new id.
type move struct {
	from, to roleFile
	drop     bool
}

// reconcileRoles finds the role files the registry shows under another id or pid.
// It never judges death - pruneRoles does, after these moves - so a file whose id
// was re-minted is carried to the new id instead of being pruned as a reused pid.
// Whatever re-minted the id (/clear, an in-session /resume, a harness duplicate),
// the process is the anchor, so any caller can make the move, not only the holder.
//
// A result computed outside peers.lock is a VIEW only: every write goes through
// reconcileLocked, which recomputes it from what is on disk under the lock.
func reconcileRoles(files map[string]roleFile, snap ccregistry.Snapshot) []move {
	sids := make([]string, 0, len(files))
	for sid := range files {
		sids = append(sids, sid)
	}
	sort.Strings(sids)
	var out []move
	rotated := map[string][]roleFile{} // new sid -> the old files that rotated to it
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
		if newSid, ok := snap.Rotated(sid, f.Pid, set); ok {
			rotated[newSid] = append(rotated[newSid], f)
		}
	}
	for _, newSid := range sortedKeys(rotated) {
		out = append(out, movesTo(newSid, rotated[newSid], files)...)
	}
	return out
}

// movesTo settles every old file that rotated to newSid, by destination: a file the
// new id already has wins (it was written by the session itself); otherwise the old
// file with the latest set wins, and a tie is left unmoved - never decided by the
// order of session ids.
func movesTo(newSid string, olds []roleFile, files map[string]roleFile) []move {
	var out []move
	if _, has := files[newSid]; has {
		for _, f := range olds {
			out = append(out, move{from: f, drop: true})
		}
		return out
	}
	win, tie := -1, false
	var latest time.Time
	for i, f := range olds {
		set, _ := time.Parse(time.RFC3339, f.Set)
		switch {
		case win < 0 || set.After(latest):
			win, latest, tie = i, set, false
		case set.Equal(latest):
			tie = true
		}
	}
	if tie {
		return nil
	}
	for i, f := range olds {
		if i != win {
			out = append(out, move{from: f, drop: true})
			continue
		}
		to := f
		to.Session, to.path = newSid, filepath.Join(filepath.Dir(f.path), newSid+".role")
		out = append(out, move{from: f, to: to})
	}
	return out
}

func sortedKeys(m map[string][]roleFile) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// persistMoves writes moves to disk. Only reconcileLocked calls it: the moves must
// be computed from files read under the same peers.lock hold, or a move planned
// earlier would write a stale image over a role the new id registered since.
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

// reconcileLocked re-reads role files and the registry - the caller holds
// peers.lock - carries every re-minted or re-pidded role, and returns the fresh
// state every locked decision after it must use.
func reconcileLocked(root string) (map[string]roleFile, []string, ccregistry.Snapshot, error) {
	files, invalid := loadRoles(root)
	snap := ccregistry.Read(ccregistry.HomeDir())
	moves := reconcileRoles(files, snap)
	applyMoves(files, moves)
	return files, invalid, snap, persistMoves(root, moves)
}
