//go:build unix

package ccregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withAlive(t *testing.T, pids ...int) {
	t.Helper()
	set := map[int]bool{}
	for _, p := range pids {
		set[p] = true
	}
	old := ProcAlive
	ProcAlive = func(pid int) bool { return set[pid] }
	t.Cleanup(func() { ProcAlive = old })
}

func fixture(name string) string { return filepath.Join("testdata", "registry", name) }

func TestRegistry_Basic(t *testing.T) {
	withAlive(t, 1001)
	s := Read(fixture("basic"))
	if len(s.Entries) != 1 || s.Degraded != nil {
		t.Fatalf("basic: %+v", s)
	}
	e := s.Entries[0]
	if e.Pid != 1001 || !e.NameOK || e.Name != "MAIN" || e.Cwd != "/repo" {
		t.Errorf("entry: %+v", e)
	}
}

func TestRegistry_Mixed(t *testing.T) {
	withAlive(t, 1001, 1003) // 1002 is a stale file of a dead session
	s := Read(fixture("mixed"))
	if len(s.Entries) != 1 || s.Entries[0].Pid != 1001 {
		t.Fatalf("mixed entries: %+v", s.Entries)
	}
	if s.Degraded == nil || s.Degraded.Reason != "registry_protocol_unknown" || s.Degraded.Detail != "1 live: peerProtocol=2" {
		t.Errorf("mixed degraded: %+v", s.Degraded)
	}
}

func TestRegistry_StaleOnly(t *testing.T) {
	withAlive(t)
	s := Read(fixture("stale-only"))
	if len(s.Entries) != 0 || s.Degraded != nil {
		t.Errorf("stale-only: %+v", s)
	}
}

func TestRegistry_Bad(t *testing.T) {
	withAlive(t, 1004, 1005)
	s := Read(fixture("bad"))
	if len(s.Entries) != 0 || s.Degraded == nil || s.Degraded.Detail != "2 live: sessionId invalid, unparseable" {
		t.Errorf("bad: %+v %+v", s.Entries, s.Degraded)
	}
}

func TestRegistry_AbsentAndConfigDir(t *testing.T) {
	if s := Read(t.TempDir()); s.Degraded == nil || s.Degraded.Reason != "registry_absent" {
		t.Errorf("absent: %+v", s.Degraded)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", fixture("basic"))
	if HomeDir() != fixture("basic") {
		t.Errorf("HomeDir ignores CLAUDE_CONFIG_DIR: %q", HomeDir())
	}
}

func TestLiveness(t *testing.T) {
	const a, b = "11111111-1111-4111-8111-111111111111", "44444444-4444-4444-8444-444444444444"
	withAlive(t, 1001, 1003, 1009)
	s := Read(fixture("mixed")) // 1001 = a (protocol 1), 1003 unrecognized
	cases := []struct {
		name string
		sid  string
		pid  int
		want Liveness
	}{
		{"recorded pid dead, session resumed under a live pid", a, 1002, Alive},
		{"dead pid, session not in the registry", b, 1002, Dead},
		{"live pid, same sid", a, 1001, Alive},
		{"live pid, other sid (reuse)", b, 1001, Dead},
		{"live pid in Unrecognized", b, 1003, Unknown},
		{"live pid without a file", b, 1009, Dead},
		{"pid 0, sid found", a, 0, Alive},
		{"pid 0, not found, unrecognized present", b, 0, Unknown},
	}
	for _, c := range cases {
		if got := s.Liveness(c.sid, c.pid); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	withAlive(t)
	if got := Read(fixture("stale-only")).Liveness(b, 0); got != Unknown {
		t.Errorf("pid 0, not found, clean registry: %v", got)
	}
	if got := Read(t.TempDir()).Liveness(a, 1001); got != Unknown {
		t.Errorf("registry absent: %v", got)
	}
}

func TestRotated(t *testing.T) {
	const old, cur = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	set := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	write := func(startedAt int64) Snapshot {
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
		b, _ := json.Marshal(map[string]any{"sessionId": cur, "name": "MAIN", "peerProtocol": 1, "startedAt": startedAt})
		_ = os.WriteFile(filepath.Join(dir, "sessions", "1001.json"), b, 0o600)
		return Read(dir)
	}
	withAlive(t, 1001)
	cases := []struct {
		name    string
		started int64
		sid     string
		pid     int
		want    bool
	}{
		{"same process, id re-minted", set.Add(-time.Hour).UnixMilli(), old, 1001, true},
		{"started within set's second", set.Add(800 * time.Millisecond).UnixMilli(), old, 1001, true},
		{"pid reused after set", set.Add(time.Hour).UnixMilli(), old, 1001, false},
		{"startedAt unknown", 0, old, 1001, false},
		{"same sid is not a rotation", set.Add(-time.Hour).UnixMilli(), cur, 1001, false},
		{"pid not in the registry", set.Add(-time.Hour).UnixMilli(), old, 1002, false},
	}
	for _, c := range cases {
		got, ok := write(c.started).Rotated(c.sid, c.pid, set)
		if ok != c.want || (ok && got != cur) {
			t.Errorf("%s: (%q, %v), want ok=%v", c.name, got, ok, c.want)
		}
	}
}
