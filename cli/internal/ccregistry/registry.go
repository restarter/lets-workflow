//go:build unix

package ccregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Entry is one live registry file read under peerProtocol 1. Only the fields LETS
// reads are decoded; <pid>.<hash>.key files beside the entries hold peer tokens and
// are never opened.
type Entry struct {
	Pid          int    `json:"-"`
	NameOK       bool   `json:"-"`
	Name         string `json:"name"`
	NameSource   string `json:"nameSource"`
	SessionID    string `json:"sessionId"`
	Cwd          string `json:"cwd"`
	Status       string `json:"status"`
	Version      string `json:"version"`
	StartedAt    int64  `json:"startedAt"` // ms since epoch; unchanged across /clear (same process)
	PeerProtocol *int   `json:"peerProtocol"`
}

// Degraded names why the registry could not be read in full.
type Degraded struct{ Reason, Detail string }

// Liveness of one session, never guessed.
type Liveness int

const (
	Unknown Liveness = iota
	Alive
	Dead
)

// String is the envelope form: alive | dead | unknown.
func (l Liveness) String() string {
	switch l {
	case Alive:
		return "alive"
	case Dead:
		return "dead"
	}
	return "unknown"
}

// Snapshot is one read of the registry. Entries are live pids parsed under
// peerProtocol 1 with a valid session id; Unrecognized holds live pids that could
// not be read: pid -> "peerProtocol=2" | "peerProtocol=absent" | "unparseable" |
// "sessionId invalid".
type Snapshot struct {
	Entries      []Entry
	Unrecognized map[int]string
	Degraded     *Degraded
}

// HomeDir is the Claude config dir: $CLAUDE_CONFIG_DIR when set, else ~/.claude.
var HomeDir = func() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// ProcAlive reports whether pid is a running process (EPERM counts: it exists).
var ProcAlive = func(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Read judges each entry on its own. Liveness comes from the filename pid before
// any content check, so a stale file of a dead session never degrades the
// registry; a live entry that cannot be read is counted by pid and named in one
// degraded reason, and the readable rows still come back.
func Read(claudeDir string) Snapshot {
	dir := filepath.Join(claudeDir, "sessions")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return Snapshot{Degraded: &Degraded{Reason: "registry_absent"}}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	s := Snapshot{Unrecognized: map[int]string{}}
	for _, f := range files {
		pid, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil || pid <= 0 || !ProcAlive(pid) {
			continue
		}
		var e Entry
		if b, err := os.ReadFile(f); err != nil || json.Unmarshal(b, &e) != nil {
			s.Unrecognized[pid] = "unparseable"
			continue
		}
		if e.PeerProtocol == nil || *e.PeerProtocol != 1 {
			s.Unrecognized[pid] = protoString(e.PeerProtocol)
			continue
		}
		// After the protocol check: a renamed field under a new protocol is counted
		// as that protocol, not silently skipped.
		if !ValidSession(e.SessionID) {
			s.Unrecognized[pid] = "sessionId invalid"
			continue
		}
		e.Pid, e.NameOK = pid, ValidName(e.Name)
		s.Entries = append(s.Entries, e)
	}
	if n := len(s.Unrecognized); n > 0 {
		s.Degraded = &Degraded{Reason: "registry_protocol_unknown", Detail: fmt.Sprintf("%d live: %s", n, sortedLabels(s.Unrecognized))}
	}
	return s
}

func protoString(p *int) string {
	if p == nil {
		return "peerProtocol=absent"
	}
	return "peerProtocol=" + strconv.Itoa(*p)
}

func sortedLabels(m map[int]string) string {
	seen := map[string]bool{}
	var labels []string
	for _, l := range m {
		if !seen[l] {
			seen[l] = true
			labels = append(labels, l)
		}
	}
	sort.Strings(labels)
	return strings.Join(labels, ", ")
}

// Find returns the entry of session sid.
func (s Snapshot) Find(sid string) (Entry, bool) {
	for _, e := range s.Entries {
		if e.SessionID == sid {
			return e, true
		}
	}
	return Entry{}, false
}

// FindPid returns the entry of pid.
func (s Snapshot) FindPid(pid int) (Entry, bool) {
	for _, e := range s.Entries {
		if e.Pid == pid {
			return e, true
		}
	}
	return Entry{}, false
}

// Rotated returns the session id now running in the process that held sid: pid is
// alive in the registry under ANOTHER session id, sid itself runs nowhere, and that
// process started no later than set (the time the holder recorded itself; a second
// of tolerance for set's precision). /clear and an in-session /resume re-mint the id
// this way. A reused pid started after set; an entry without startedAt proves
// nothing - both return false, leaving Liveness's Dead verdict in place.
func (s Snapshot) Rotated(sid string, pid int, set time.Time) (string, bool) {
	if pid <= 0 || set.IsZero() {
		return "", false
	}
	if _, running := s.Find(sid); running {
		return "", false
	}
	e, ok := s.FindPid(pid)
	if !ok || e.SessionID == sid || e.StartedAt <= 0 {
		return "", false
	}
	if time.UnixMilli(e.StartedAt).After(set.Add(time.Second)) {
		return "", false
	}
	return e.SessionID, true
}

// Liveness of one session; pid is the holder's recorded pid (0 when unknown).
// Never guesses: a pid-less holder that the registry does not show is Unknown,
// because "not found" proves nothing about a session the registry never described.
func (s Snapshot) Liveness(sid string, pid int) Liveness {
	if s.Degraded != nil && s.Degraded.Reason == "registry_absent" {
		return Unknown
	}
	// Entries hold live pids only, so the session id found under ANY of them proves the
	// session runs: `claude -r <sid>` resumes the same id under a new pid, and the
	// recorded pid is then dead while the session is not.
	if _, ok := s.Find(sid); ok {
		return Alive
	}
	if pid > 0 {
		if !ProcAlive(pid) {
			return Dead
		}
		for _, e := range s.Entries {
			if e.Pid == pid {
				if e.SessionID == sid {
					return Alive
				}
				return Dead // the pid was reused by another session
			}
		}
		if _, ok := s.Unrecognized[pid]; ok {
			return Unknown
		}
		return Dead // a live pid without a registry file: the session exited, the pid was reused
	}
	return Unknown
}
