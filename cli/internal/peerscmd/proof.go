//go:build unix

package peerscmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

// The session proof (until lets-gbfpd replaces it with a pid-based proof): one file
// per session, <root>/.lets/sessions/peers/<sid>.role, holding "session:", "pid:"
// and "set:" lines. Nothing resolves roles from it any more; the SessionStart
// session guard reads it (RoleProof) to prove a /clear re-mint, and CarryProof
// moves it to the re-minted id so the next /clear is provable too. A file an older
// binary wrote (with role:, task:, name: ... lines) is read the same way.

var now = time.Now

func peersDir(root string) string { return filepath.Join(root, ".lets", "sessions", "peers") }

var errPeersLockBusy = errors.New("peers_lock_busy")

// lockPeers takes .lets/locks/peers.lock, blocking or until deadline.
func lockPeers(root string, deadline time.Duration) (func(), error) {
	dir := filepath.Join(root, ".lets", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "peers.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if deadline > 0 {
		err = fsutil.TryLockFile(f, now().Add(deadline))
	} else {
		err = fsutil.LockFile(f)
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, fsutil.ErrLockBusy) {
			return nil, errPeersLockBusy
		}
		return nil, err
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

func session6(sid string) string {
	if len(sid) >= 6 {
		return sid[:6]
	}
	return sid
}

// proofFile is one parsed proof file.
type proofFile struct {
	path    string
	Session string
	Pid     int
	Set     string // RFC 3339
}

// writeProof writes sid's proof atomically (dir 0700, file 0600).
func writeProof(root, sid string, pid int, set time.Time) error {
	dir := peersDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	body := fmt.Sprintf("session: %s\npid: %d\nset: %s\n", sid, pid, set.UTC().Format(time.RFC3339))
	tmp, err := os.CreateTemp(dir, ".role-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	_ = tmp.Chmod(0o600)
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, sid+".role"))
}

// RoleProof returns the pid and set time sid's proof file recorded - the proof a
// later session id is the same process re-minted (ccregistry.Snapshot.Rotated).
// Read-only: the SessionStart hook reads it BEFORE CarryProof moves a re-minted
// proof to the new id and deletes the old one.
func RoleProof(root, sid string) (pid int, set time.Time, ok bool) {
	if !ccregistry.ValidSession(sid) {
		return 0, time.Time{}, false
	}
	data, err := os.ReadFile(filepath.Join(peersDir(root), sid+".role"))
	if err != nil {
		return 0, time.Time{}, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, cut := strings.Cut(line, ": ")
		switch {
		case !cut:
		case k == "pid":
			pid, _ = strconv.Atoi(strings.TrimSpace(v))
		case k == "set":
			set, _ = time.Parse(time.RFC3339, strings.TrimSpace(v))
		}
	}
	return pid, set, pid > 0 && !set.IsZero()
}

// proofLockWait bounds CarryProof's wait for peers.lock: it runs on hot paths (the
// SessionStart hook, who, orchestrator, role set) and must never stall them.
var proofLockWait = time.Second

// CarryProof reconciles EVERY proof file against one registry snapshot, under the
// peers lock (1 s deadline): a file whose session runs under a new pid (`claude -r`)
// gets pid and set refreshed; a file whose session was re-minted in the same process
// (Snapshot.Rotated) moves to the new id. Never creates a proof from nothing. A
// missing peers dir, a busy lock or a failed write is silent - the next call (the
// hook, or the lazy run in who / orchestrator / role set) retries.
func CarryProof(ctx context.Context, root string) {
	if root == "" || ctx.Err() != nil {
		return
	}
	if _, err := os.Stat(peersDir(root)); err != nil {
		return
	}
	unlock, err := lockPeers(root, proofLockWait)
	if err != nil {
		return
	}
	defer unlock()
	files := loadProofs(root)
	snap := ccregistry.Read(ccregistry.HomeDir())
	_ = persistCarries(root, carries(files, snap))
}

// loadProofs parses peers/*.role; a file whose name is not a session id is skipped.
func loadProofs(root string) map[string]proofFile {
	paths, _ := filepath.Glob(filepath.Join(peersDir(root), "*.role"))
	out := map[string]proofFile{}
	for _, p := range paths {
		sid := strings.TrimSuffix(filepath.Base(p), ".role")
		data, err := os.ReadFile(p)
		if !ccregistry.ValidSession(sid) || err != nil {
			continue
		}
		f := proofFile{path: p, Session: sid}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, ": ")
			if !ok {
				continue
			}
			switch k {
			case "set":
				f.Set = strings.TrimSpace(v)
			case "pid":
				f.Pid, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		out[sid] = f
	}
	return out
}

// carry is one correction to a proof file: its session id was re-minted in the same
// process (from.Session -> to.Session), or its live session runs under a new pid
// (same session, to.Pid). drop: the old file only goes - the new id already has its
// own file, or another old file of the same process won the new id.
type carry struct {
	from, to proofFile
	drop     bool
}

// carries finds the proof files the registry shows under another id or pid. The
// process is the anchor, so any caller can make the move, not only the holder.
func carries(files map[string]proofFile, snap ccregistry.Snapshot) []carry {
	sids := make([]string, 0, len(files))
	for sid := range files {
		sids = append(sids, sid)
	}
	sort.Strings(sids)
	var out []carry
	rotated := map[string][]proofFile{} // new sid -> the old files that rotated to it
	for _, sid := range sids {
		f := files[sid]
		if e, ok := snap.Find(sid); ok {
			// Same session, new pid (`claude -r`): refresh pid, and set, so a later
			// /clear in THIS process still passes Rotated's started-before-set test.
			// A duplicate (the recorded pid still carries the session) is left alone.
			if cur, ok := snap.FindPid(f.Pid); !ok || cur.SessionID != sid {
				to := f
				to.Pid, to.Set = e.Pid, now().UTC().Format(time.RFC3339)
				out = append(out, carry{from: f, to: to})
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
	keys := make([]string, 0, len(rotated))
	for k := range rotated {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, newSid := range keys {
		out = append(out, carriesTo(newSid, rotated[newSid], files)...)
	}
	return out
}

// carriesTo settles every old file that rotated to newSid, by destination: a file
// the new id already has wins; otherwise the old file with the latest set wins, and a
// tie is left unmoved - never decided by the order of session ids.
func carriesTo(newSid string, olds []proofFile, files map[string]proofFile) []carry {
	var out []carry
	if _, has := files[newSid]; has {
		for _, f := range olds {
			out = append(out, carry{from: f, drop: true})
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
			out = append(out, carry{from: f, drop: true})
			continue
		}
		to := f
		to.Session, to.path = newSid, filepath.Join(filepath.Dir(f.path), newSid+".role")
		out = append(out, carry{from: f, to: to})
	}
	return out
}

// persistCarries writes the carries; the caller holds peers.lock and computed them
// from files read under that same hold.
func persistCarries(root string, cs []carry) error {
	for _, c := range cs {
		if !c.drop {
			set, err := time.Parse(time.RFC3339, c.to.Set)
			if err != nil {
				set = now()
			}
			if err := writeProof(root, c.to.Session, c.to.Pid, set); err != nil {
				return err
			}
		}
		if c.drop || c.to.Session != c.from.Session {
			if err := os.Remove(c.from.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
