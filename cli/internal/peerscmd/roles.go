//go:build unix

package peerscmd

import (
	"encoding/hex"
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
	"github.com/restarter/lets-workflow/cli/internal/redact"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
)

// orcaTerminalVar is the variable an Orca pane exports with its own terminal handle
// (spike 1.0 item 2: it equals the `terminal list` handle). A session records it in
// its role file; that self-report is the only thing that joins it to an Orca row.
const orcaTerminalVar = "ORCA_TERMINAL_HANDLE"

// legacyRoleMaxAge prunes a pid-less role file (written by an older binary).
const legacyRoleMaxAge = 7 * 24 * time.Hour

var now = time.Now

// RoleOptions configures SetRole.
type RoleOptions struct {
	Session  string
	Role     string // orchestrator | worker | peer
	Task     string // worker only
	Scope    string // orchestrator only
	Cwd      string
	Takeover bool
	// LockDeadline bounds the wait for peers.lock (zero blocks).
	LockDeadline time.Duration
}

// Holder is the session holding a name another orchestrator asked for.
type Holder struct {
	Name     string `json:"name"`
	Session6 string `json:"session6"`
	Alive    string `json:"alive"`
	Since    string `json:"since,omitempty"`
}

// RoleInfo is what SetRole decided.
type RoleInfo struct {
	Granted     bool     `json:"granted"`
	Role        string   `json:"role,omitempty"`
	Reason      string   `json:"reason,omitempty"`      // name_held | orchestrator_needs_name | session_not_in_registry
	Remediation string   `json:"remediation,omitempty"` // the exact line to show the user; set with every Reason
	Holder      *Holder  `json:"holder,omitempty"`
	Registered  bool     `json:"registered"`
	Demoted     string   `json:"demoted,omitempty"` // session6 of a holder turned into a plain peer by --takeover
	Pruned      int      `json:"pruned"`
	Invalid     []string `json:"invalid,omitempty"` // role files that could not be parsed (left in place)
}

type roleFile struct {
	path                                                string
	Session, Role, Task, Name, Scope, Cwd, OrcaTerminal string
	Set                                                 string
	Pid                                                 int
}

func peersDir(root string) string { return filepath.Join(root, ".lets", "sessions", "peers") }

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

var errPeersLockBusy = errors.New("peers_lock_busy")

// loadRoles parses peers/*.role. A file whose name is not a session id, or whose
// fields fail validation, is skipped and reported.
func loadRoles(root string) (map[string]roleFile, []string) {
	files, _ := filepath.Glob(filepath.Join(peersDir(root), "*.role"))
	out := map[string]roleFile{}
	var invalid []string
	for _, p := range files {
		sid := strings.TrimSuffix(filepath.Base(p), ".role")
		data, err := os.ReadFile(p)
		if !ccregistry.ValidSession(sid) || err != nil {
			invalid = append(invalid, filepath.Base(p))
			continue
		}
		f := roleFile{path: p, Session: sid}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, ": ")
			if !ok {
				continue
			}
			switch k {
			case "role":
				f.Role = v
			case "task":
				f.Task = v
			case "name":
				f.Name = v
			case "scope":
				f.Scope = v
			case "cwd":
				f.Cwd = v
			case "orca_terminal":
				f.OrcaTerminal = v
			case "set":
				f.Set = v
			case "pid":
				f.Pid, _ = strconv.Atoi(v)
			}
		}
		if !ValidRole(f.Role) || (f.Role == "worker" && !taskid.Valid(f.Task)) {
			invalid = append(invalid, filepath.Base(p))
			continue
		}
		out[sid] = f
	}
	sort.Strings(invalid)
	return out, invalid
}

// writeRole writes f atomically (dir 0700, file 0600).
func writeRole(root string, f roleFile) error {
	dir := peersDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	var b strings.Builder
	fmt.Fprintf(&b, "role: %s\n", f.Role)
	for _, kv := range [][2]string{{"task", f.Task}, {"name", f.Name}, {"scope", f.Scope}, {"cwd", f.Cwd}, {"orca_terminal", f.OrcaTerminal}} {
		if kv[1] != "" {
			fmt.Fprintf(&b, "%s: %s\n", kv[0], kv[1])
		}
	}
	if f.Pid > 0 {
		fmt.Fprintf(&b, "pid: %d\n", f.Pid)
	}
	fmt.Fprintf(&b, "set: %s\n", f.Set)
	tmp, err := os.CreateTemp(dir, ".role-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	_ = tmp.Chmod(0o600)
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, f.Session+".role"))
}

// liveName is the holder's name from the live registry entry (by pid, then by sid);
// the stored name only when neither resolves.
func liveName(snap ccregistry.Snapshot, f roleFile) string {
	if f.Pid > 0 {
		if e, ok := snap.FindPid(f.Pid); ok && e.SessionID == f.Session {
			return e.Name
		}
	}
	if e, ok := snap.Find(f.Session); ok {
		return e.Name
	}
	return f.Name
}

func session6(sid string) string {
	if len(sid) >= 6 {
		return sid[:6]
	}
	return sid
}

// pruneRoles removes the role files of dead holders (never self) and legacy
// pid-less files older than legacyRoleMaxAge whose sid the registry does not show.
func pruneRoles(files map[string]roleFile, snap ccregistry.Snapshot, self string) int {
	n := 0
	for sid, f := range files {
		if sid == self {
			continue
		}
		dead := snap.Liveness(f.Session, f.Pid) == ccregistry.Dead
		if !dead && f.Pid == 0 {
			if _, found := snap.Find(sid); !found {
				if set, err := time.Parse(time.RFC3339, f.Set); err == nil && now().Sub(set) > legacyRoleMaxAge {
					dead = true
				}
			}
		}
		if dead {
			if f.Role == "orchestrator" {
				_ = writeLastSeen(filepath.Dir(f.path), f)
			}
			_ = os.Remove(f.path)
			delete(files, sid)
			n++
		}
	}
	return n
}

// lastDir holds one last-seen file per orchestrator name.
func lastDir(peers string) string { return filepath.Join(peers, "last") }

// lastSeenFile names a name's file by the lowercase hex of its UTF-8 bytes, so
// "MAIN PWA" and "MAIN_PWA" never share a file and no name reaches a path unescaped.
func lastSeenFile(peers, name string) string {
	return filepath.Join(lastDir(peers), hex.EncodeToString([]byte(name))+".last")
}

// writeLastSeen records a pruned orchestrator (so the hub can wake it later),
// atomically, one file per name; a later prune of the same name overwrites only it.
func writeLastSeen(peers string, f roleFile) error {
	if !ccregistry.ValidName(f.Name) {
		return nil
	}
	dir := lastDir(peers)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body := fmt.Sprintf("session: %s\npid: %d\nname: %s\nscope: %s\ncwd: %s\nset: %s\n", f.Session, f.Pid, f.Name, f.Scope, f.Cwd, f.Set)
	tmp, err := os.CreateTemp(dir, ".last-*")
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
	return os.Rename(tmp.Name(), lastSeenFile(peers, f.Name))
}

// cleanScope keeps one line of at most 120 bytes without control bytes.
func cleanScope(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	s = strings.TrimSpace(redact.Control(s))
	return strings.TrimSpace(redact.Cap(s, 120))
}

// SetRole records this session's role. Several orchestrators may share a repo, each
// under a unique LIVE name (the name is the address); a second live session asking
// for a held name gets name_held unless it takes over, which demotes only that
// holder. Dead holders are pruned first. Nothing is written for a session the
// registry does not describe: a pid-less file could never be pruned.
func SetRole(root string, o RoleOptions) (*RoleInfo, error) {
	if !ccregistry.ValidSession(o.Session) || !ValidRole(o.Role) || (o.Role == "worker" && !taskid.Valid(o.Task)) {
		return nil, &Error{Code: ExitUsage, Kind: "usage", Message: "role set needs a valid --session, a role (orchestrator|worker|peer) and, for a worker, a valid --task"}
	}
	unlock, err := lockPeers(root, o.LockDeadline)
	if err != nil {
		return nil, err
	}
	defer unlock()
	files, invalid, snap, err := reconcileLocked(root)
	if err != nil {
		return nil, err
	}
	info := &RoleInfo{Invalid: invalid}
	info.Pruned = pruneRoles(files, snap, o.Session)
	self, selfKnown := snap.Find(o.Session)
	info.Registered = selfKnown
	if !selfKnown {
		info.Reason = "session_not_in_registry"
		info.Remediation = "Claude Code's session registry does not list this session - restart it (or update Claude Code), then run /lets:start again"
		return info, nil
	}
	if o.Role == "orchestrator" {
		if !ccregistry.ValidName(self.Name) {
			info.Reason = "orchestrator_needs_name"
			info.Remediation = remedy("orchestrator_needs_name", "", "", false, false)
			return info, nil
		}
		for sid, f := range files {
			if sid == o.Session || f.Role != "orchestrator" || liveName(snap, f) != self.Name {
				continue
			}
			if !o.Takeover {
				info.Reason = "name_held"
				info.Remediation = "/rename this session and run /lets:start --main again, or rerun with --takeover to demote " + self.Name + " (" + session6(sid) + ")"
				info.Holder = &Holder{Name: self.Name, Session6: session6(sid), Alive: snap.Liveness(f.Session, f.Pid).String(), Since: f.Set}
				return info, nil
			}
			f.Role = "peer"
			f.Scope = ""
			if err := writeRole(root, f); err != nil {
				return nil, err
			}
			info.Demoted = session6(sid)
		}
	}
	rf := roleFile{Session: o.Session, Role: o.Role, Name: self.Name, Pid: self.Pid, Cwd: o.Cwd, OrcaTerminal: os.Getenv(orcaTerminalVar), Set: now().UTC().Format(time.RFC3339)}
	if o.Role == "worker" {
		rf.Task = o.Task
	}
	if o.Role == "orchestrator" {
		rf.Scope = cleanScope(o.Scope)
	}
	if err := writeRole(root, rf); err != nil {
		return nil, err
	}
	info.Granted, info.Role = true, o.Role
	return info, nil
}

// ClearRole removes this session's role file.
func ClearRole(root, session string) error {
	if !ccregistry.ValidSession(session) {
		return &Error{Code: ExitUsage, Kind: "usage", Message: "role clear needs a valid --session"}
	}
	unlock, err := lockPeers(root, 0)
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.Remove(filepath.Join(peersDir(root), session+".role")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
