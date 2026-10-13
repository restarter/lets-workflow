//go:build unix

package ccregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

// An entry under another peerProtocol is not addressable, but its session id still
// proves that session runs: Loose keeps it (a valid id only); an unparseable one
// and a stale file add nothing.
func TestSnapshot_LooseKeepsProtocolUnknown(t *testing.T) {
	dir := t.TempDir()
	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(pid, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sessions, pid+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const v2, bad = "bbbbbbbb-0000-4000-8000-000000000002", "not-a-uuid"
	write("2001", `{"sessionId":"`+v2+`","peerProtocol":2}`)
	write("2002", `{"sessionId":"`+bad+`"}`)
	write("2003", `{not json`)
	write("2004", `{"sessionId":"cccccccc-0000-4000-8000-000000000003","peerProtocol":2}`) // stale: pid dead
	withAlive(t, 2001, 2002, 2003)
	s := Read(dir)
	if len(s.Loose) != 1 || s.Loose[v2] != 2001 {
		t.Errorf("Loose = %v, want only %s -> 2001", s.Loose, v2)
	}
	if s.Degraded == nil || s.Degraded.Reason != "registry_protocol_unknown" || len(s.Unrecognized) != 3 {
		t.Errorf("degraded=%+v unrecognized=%v", s.Degraded, s.Unrecognized)
	}
	if _, ok := s.Find(v2); ok {
		t.Error("a Loose session must never be addressable through Find")
	}
}

// writeRows writes one registry file per pid -> JSON body and reads the registry
// back with exactly those pids alive.
func writeRows(t *testing.T, rows map[int]string) Snapshot {
	t.Helper()
	dir := t.TempDir()
	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	var pids []int
	for pid, body := range rows {
		pids = append(pids, pid)
		if err := os.WriteFile(filepath.Join(sessions, strconv.Itoa(pid)+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	withAlive(t, pids...)
	return Read(dir)
}

const (
	parkedSID = "aaaaaaaa-1111-4111-8111-111111111111"
	bgSID     = "bbbbbbbb-2222-4222-8222-222222222222"
)

// parkedRow is the observed shape of a session parked into a background job.
func parkedRow(parkedJobID string) string {
	b, _ := json.Marshal(map[string]any{
		"pid": 3001, "sessionId": parkedSID, "cwd": "/repo", "startedAt": 1760000000000,
		"procStart": "Mon Oct 12 17:00:00 2026", "kind": "interactive", "status": "idle",
		"name": "snake-lead", "nameSource": "user", "peerProtocol": 1, "parkedJobId": parkedJobID,
	})
	return string(b)
}

// bgRow is the observed shape of the background job's row: the shared name, no
// nameSource. kind "" or jobID "" omit that key.
func bgRow(kind, jobID string) string {
	m := map[string]any{
		"pid": 3002, "sessionId": bgSID, "cwd": "/repo", "startedAt": 1760000100000,
		"procStart": "Mon Oct 12 17:01:40 2026", "status": "idle", "name": "snake-lead", "peerProtocol": 1,
	}
	if kind != "" {
		m["kind"] = kind
	}
	if jobID != "" {
		m["jobId"] = jobID
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// selfParkedBgRow is one background-job row whose parkedJobId equals its own jobId.
var selfParkedBgRow = `{"sessionId":"` + parkedSID + `","name":"snake-lead","peerProtocol":1,"kind":"bg","jobId":"deadbeef","parkedJobId":"deadbeef"}`

func TestRead_DecodesJobFields(t *testing.T) {
	plain := `{"sessionId":"` + bgSID + `","name":"MAIN","peerProtocol":1,"startedAt":1760000100000}`
	s := writeRows(t, map[int]string{3001: parkedRow("bbbbbbbb"), 3002: plain})
	if s.Degraded != nil || len(s.Entries) != 2 {
		t.Fatalf("entries=%+v degraded=%+v", s.Entries, s.Degraded)
	}
	p, _ := s.Find(parkedSID)
	if p.Kind != "interactive" || p.ParkedJobID != "bbbbbbbb" || p.JobID != "" {
		t.Errorf("parked row: %+v", p)
	}
	c, _ := s.Find(bgSID)
	if c.Kind != "" || c.JobID != "" || c.ParkedJobID != "" {
		t.Errorf("row without job keys: %+v", c)
	}
	s = writeRows(t, map[int]string{3002: bgRow("bg", "bbbbbbbb")})
	if c, _ = s.Find(bgSID); c.Kind != "bg" || c.JobID != "bbbbbbbb" || c.ParkedJobID != "" {
		t.Errorf("bg row: %+v", c)
	}
	s = writeRows(t, map[int]string{3001: selfParkedBgRow})
	if e, _ := s.Find(parkedSID); e.Kind != "bg" || e.JobID != "deadbeef" || e.ParkedJobID != "deadbeef" {
		t.Errorf("row with all three job keys: %+v", e)
	}
}

func TestRead_JobFieldWrongTypeStillReads(t *testing.T) {
	body := `{"sessionId":"` + bgSID + `","name":"snake-lead","peerProtocol":1,"kind":["bg"],"jobId":123,"parkedJobId":{"x":1}}`
	s := writeRows(t, map[int]string{3002: body})
	if s.Degraded != nil || len(s.Entries) != 1 {
		t.Fatalf("entries=%+v degraded=%+v", s.Entries, s.Degraded)
	}
	if e := s.Entries[0]; e.JobID != "" || e.Kind != "" || e.ParkedJobID != "" {
		t.Errorf("non-string job fields must read as empty: %+v", e)
	}
}

func TestContinuedIn(t *testing.T) {
	const other = "cccccccc-3333-4333-8333-333333333333"
	plainParked := `{"sessionId":"` + parkedSID + `","name":"snake-lead","peerProtocol":1}`
	plainBg := `{"sessionId":"` + bgSID + `","name":"snake-lead","peerProtocol":1}`
	cases := []struct {
		name     string
		rows     map[int]string
		sid, own string
		want     bool
	}{
		{"parked + bg, matching ids", map[int]string{3001: parkedRow("bbbbbbbb"), 3002: bgRow("bg", "bbbbbbbb")}, parkedSID, bgSID, true},
		{"explicit ids, not the own sid prefix", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("bg", "deadbeef")}, parkedSID, bgSID, true},
		{"own row without kind", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("", "deadbeef")}, parkedSID, bgSID, true},
		{"neither row carries a job field", map[int]string{3001: plainParked, 3002: plainBg}, parkedSID, bgSID, false},
		{"own row kind interactive", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("interactive", "deadbeef")}, parkedSID, bgSID, false},
		{"different job id", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("bg", "feedface")}, parkedSID, bgSID, false},
		{"own row without jobId", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("bg", "")}, parkedSID, bgSID, false},
		{"parkedJobId is own sid prefix, own has no jobId", map[int]string{3001: parkedRow(bgSID[:8]), 3002: bgRow("bg", "")}, parkedSID, bgSID, false},
		{"sid not listed", map[int]string{3002: bgRow("bg", "deadbeef")}, other, bgSID, false},
		{"own not listed", map[int]string{3001: parkedRow("deadbeef")}, parkedSID, other, false},
		// a bg row that carries its own job id as parkedJobId: every field matches, so
		// only the sid == own guard makes it false
		{"sid == own", map[int]string{3001: selfParkedBgRow}, parkedSID, parkedSID, false},
	}
	for _, c := range cases {
		if got := writeRows(t, c.rows).ContinuedIn(c.sid, c.own); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSameProcess(t *testing.T) {
	const started = int64(1760000000000)
	s := writeRows(t, map[int]string{3001: parkedRow("deadbeef")})
	cases := []struct {
		name    string
		pid     int
		started int64
		want    bool
	}{
		{"equal ms", 3001, started, true},
		{"+800 ms", 3001, started + 800, false},
		{"-800 ms", 3001, started - 800, false},
		{"pid not listed", 3009, started, false},
		{"startedMs 0", 3001, 0, false},
	}
	for _, c := range cases {
		if got := s.SameProcess(c.pid, c.started); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// Named collapses a parked session and its background continuation into ONE logical
// session - the continuation, its address; anything else counts per process.
func TestNamed(t *testing.T) {
	const other = "cccccccc-3333-4333-8333-333333333333"
	otherRow := `{"sessionId":"` + other + `","name":"snake-lead","peerProtocol":1,"kind":"interactive"}`
	cases := []struct {
		name string
		rows map[int]string
		want []string // session ids, sorted
	}{
		{"parked + its continuation = one, the continuation", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("bg", "deadbeef")}, []string{bgSID}},
		{"two unrelated live sessions = two", map[int]string{3001: parkedRow(""), 3003: otherRow}, []string{parkedSID, other}},
		{"parked without a continuation = one, the parked row", map[int]string{3001: parkedRow("deadbeef")}, []string{parkedSID}},
		{"a bg job of another job id = two", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("bg", "feedface")}, []string{parkedSID, bgSID}},
		{"parked + continuation + an unrelated holder = two", map[int]string{3001: parkedRow("deadbeef"), 3002: bgRow("bg", "deadbeef"), 3003: otherRow}, []string{bgSID, other}},
		{"no holder", map[int]string{3003: `{"sessionId":"` + other + `","name":"MAIN","peerProtocol":1}`}, nil},
	}
	for _, c := range cases {
		var got []string
		for _, e := range writeRows(t, c.rows).Named("snake-lead") {
			got = append(got, e.SessionID)
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// NameOf classifies a session's registry name the one way every self-name reader
// uses: not listed, listed without a name, a name failing the grammar, a usable name.
func TestNameOf(t *testing.T) {
	const (
		noName = "cccccccc-3333-4333-8333-333333333333"
		badSID = "dddddddd-4444-4444-8444-444444444444"
		absent = "eeeeeeee-5555-4555-8555-555555555555"
	)
	s := writeRows(t, map[int]string{
		3001: parkedRow(""),
		3003: `{"sessionId":"` + noName + `","peerProtocol":1}`,
		3004: `{"sessionId":"` + badSID + `","name":"bad  name","peerProtocol":1}`,
	})
	for _, c := range []struct {
		sid       string
		wantName  string
		wantState NameState
	}{
		{parkedSID, "snake-lead", NameValid},
		{noName, "", NameNone},
		{badSID, "bad  name", NameInvalid},
		{absent, "", NameUnlisted},
	} {
		if n, st := s.NameOf(c.sid); n != c.wantName || st != c.wantState {
			t.Errorf("%s: %q %v, want %q %v", c.sid, n, st, c.wantName, c.wantState)
		}
	}
}

// A session id held by two processes - the parked row and an independently resumed
// process without job fields - is judged per ENTRY: only the parked row collapses into
// the continuation, the resumed process stays, in every entry order.
func TestNamed_PerEntryInEveryOrder(t *testing.T) {
	parked := Entry{Pid: 3001, SessionID: parkedSID, Name: "snake-lead", NameOK: true, Kind: "interactive", ParkedJobID: "J"}
	cont := Entry{Pid: 3002, SessionID: bgSID, Name: "snake-lead", NameOK: true, Kind: "bg", JobID: "J"}
	resumed := Entry{Pid: 3003, SessionID: parkedSID, Name: "snake-lead", NameOK: true}
	rows := []Entry{parked, cont, resumed}
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		var es []Entry
		for _, i := range order {
			es = append(es, rows[i])
		}
		var pids []int
		for _, e := range (Snapshot{Entries: es}).Named("snake-lead") {
			pids = append(pids, e.Pid)
		}
		sort.Ints(pids)
		if len(pids) != 2 || pids[0] != 3002 || pids[1] != 3003 {
			t.Errorf("order %v: Named = pids %v, want [3002 3003]", order, pids)
		}
	}
}
