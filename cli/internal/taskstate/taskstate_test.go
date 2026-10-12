package taskstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const sid = "2f942be4-23e0-4b17-9eab-df0a4a7298f2"

func write(t *testing.T, letsDir, slug, content string) {
	t.Helper()
	p := Path(letsDir, slug)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, letsDir, slug string) string {
	t.Helper()
	data, err := os.ReadFile(Path(letsDir, slug))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMergeWrite_KeepsUnknownLinesAndOrder(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\nfuture: kept verbatim\nstart: abc1234\nsession: abc1234 old\n\n")
	st, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"session": "5b55fc9 " + sid, "orc": "MAIN-PWA"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "task: lets-x\nfuture: kept verbatim\nstart: abc1234\nsession: 5b55fc9 " + sid + "\norc: MAIN-PWA\n"
	if got := read(t, d, "b"); got != want {
		t.Errorf("file:\n%s\nwant:\n%s", got, want)
	}
	if st.Orc != "MAIN-PWA" || st.Task != "lets-x" || len(st.Other) != 1 {
		t.Errorf("state = %+v", st)
	}
	// an empty value deletes the key
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "", "task": ""}}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, d, "b"); strings.Contains(got, "orc:") || strings.Contains(got, "task:") || !strings.Contains(got, "future: kept verbatim") {
		t.Errorf("delete:\n%s", got)
	}
}

// A writer that decides on the current task must see what another writer recorded
// while it waited for the lock - not what it read before (codex review: A/start-B).
func TestMergeWrite_DeriveSeesStateWrittenWhileWaiting(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-a\nstart: aaaaaaa\n")
	old := acquire
	t.Cleanup(func() { acquire = old })
	acquire = func(letsDir, slug string, deadline time.Time) (func(), error) {
		unlock, err := lock(letsDir, slug, deadline)
		if err == nil { // the other writer held the lock first
			write(t, d, "b", "task: lets-b\nstart: bbbbbbb\n")
		}
		return unlock, err
	}
	errMismatch := errors.New("task mismatch")
	var seen State
	_, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"task": "lets-a"}, Derive: func(cur State, exists bool) (map[string]string, error) {
		seen = cur
		if exists && cur.Task != "lets-a" {
			return nil, errMismatch
		}
		return nil, nil
	}})
	if !errors.Is(err, errMismatch) || seen.Task != "lets-b" || seen.Start != "bbbbbbb" {
		t.Fatalf("derive must run on the state under the lock: err=%v seen=%+v", err, seen)
	}
	if got := read(t, d, "b"); got != "task: lets-b\nstart: bbbbbbb\n" {
		t.Errorf("a refused derive writes nothing:\n%s", got)
	}
	// derived keys are validated and merged over Set
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Derive: func(State, bool) (map[string]string, error) {
		return map[string]string{"start": "not a sha"}, nil
	}}); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("an invalid derived value must be refused: %v", err)
	}
	acquire = old
	st, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Derive: func(cur State, _ bool) (map[string]string, error) {
		return map[string]string{"start": "ccccccc"}, nil
	}})
	if err != nil || st.Start != "ccccccc" || st.Orc != "MAIN" || st.Task != "lets-b" {
		t.Errorf("merged: %+v %v", st, err)
	}
}

func TestMergeWrite_CreateFalseLeavesMissingFile(t *testing.T) {
	d := t.TempDir()
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"session": "5b55fc9 " + sid}}); !errors.Is(err, ErrFileAbsent) {
		t.Fatalf("err = %v, want ErrFileAbsent", err)
	}
	if _, err := os.Stat(Path(d, "b")); !os.IsNotExist(err) {
		t.Error("file must not be created")
	}
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Create: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, d, "b"); got != "orc: MAIN\n" {
		t.Errorf("created: %q", got)
	}
}

func TestMergeWrite_RefusesInvalidValues(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\n")
	for _, set := range []map[string]string{
		{"task": "-rf"},
		{"orc": "MAIN\nsession: evil"},
		{"start": "zzz"},
		{"origin": "x"},
		{"session": "abc1234 not-a-uuid"},
		{"bogus": "x"},
	} {
		if _, err := MergeWrite(d, "b", WriteOpts{Set: set}); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%v: err = %v, want ErrInvalidValue", set, err)
		}
	}
	if got := read(t, d, "b"); got != "task: lets-x\n" {
		t.Errorf("nothing may be written on a refused value: %q", got)
	}
	// SHA-256 object names are accepted
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"start": strings.Repeat("a", 64)}}); err != nil {
		t.Errorf("64-hex start: %v", err)
	}
	if _, err := MergeWrite(d, "", WriteOpts{Set: map[string]string{"task": "x"}}); !errors.Is(err, ErrEmptySlug) {
		t.Errorf("empty slug: %v", err)
	}
}

func TestMergeWrite_ConcurrentWritersBothLand(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\n")
	var wg sync.WaitGroup
	for _, set := range []map[string]string{{"orc": "MAIN"}, {"start": "abc1234"}} {
		wg.Add(1)
		go func(set map[string]string) {
			defer wg.Done()
			if _, err := MergeWrite(d, "b", WriteOpts{Set: set}); err != nil {
				t.Error(err)
			}
		}(set)
	}
	wg.Wait()
	got := read(t, d, "b")
	if !strings.Contains(got, "orc: MAIN") || !strings.Contains(got, "start: abc1234") {
		t.Errorf("a concurrent write was lost:\n%s", got)
	}
}

func TestMergeWrite_LockBusyDeadline(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\n")
	unlock, err := lock(d, "b", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	start := time.Now()
	_, err = MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Deadline: time.Now().Add(100 * time.Millisecond)})
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("err = %v, want ErrLockBusy", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("deadline overrun")
	}
}

func TestRemove_TempsOnly(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a", "task: lets-a\n")
	write(t, d, "a.b", "task: lets-ab\n")            // another branch whose slug extends "a"
	write(t, d, "a.1234", "task: lets-a.1234\n")     // a sub-task branch: same shape as a bash mktemp temp, so never removed
	write(t, d, "a.12345.tmp", "task: lets-a-tmp\n") // a legal branch name, NOT a temp
	tmp := filepath.Join(d, "sessions", ".tasktmp-a.98765")
	if err := os.WriteFile(tmp, []byte("stranded go temp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extends := filepath.Join(d, "sessions", ".tasktmp-a.b.98765") // the temp of slug "a.b": not a's
	if err := os.WriteFile(extends, []byte("other temp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(d, "a", time.Time{}); err != nil {
		t.Fatal(err)
	}
	for slug, want := range map[string]bool{"a": false, "a.12345.tmp": true, "a.b": true, "a.1234": true} {
		if _, err := os.Stat(Path(d, slug)); (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", slug, err == nil, want)
		}
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("the stranded .tasktmp- temp must be cleaned")
	}
	if _, err := os.Stat(extends); err != nil {
		t.Error("another slug's temp must be left alone")
	}
}

func TestRemoveIfTask(t *testing.T) {
	letsDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(letsDir, "sessions"), 0o755)
	p := Path(letsDir, "b")
	if err := os.WriteFile(p, []byte("task: lets-a\nsession: x y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveIfTask(letsDir, "b", "lets-b", time.Time{}); !errors.Is(err, ErrChanged) {
		t.Fatalf("another task must be kept: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("kept file vanished")
	}
	if err := RemoveIfTask(letsDir, "b", "lets-a", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("the recorded task's file must be removed")
	}
	if err := RemoveIfTask(letsDir, "b", "lets-a", time.Time{}); err != nil {
		t.Errorf("an absent file is not an error: %v", err)
	}
}

func TestAtomicWrite_TempOutsideTaskNamespace(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a", "task: lets-a\n")
	if err := atomicWrite(Path(d, "a"), "task: lets-b\n"); err != nil {
		t.Fatal(err)
	}
	got, err := Slugs(d)
	if err != nil || !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("slugs after a write = %v %v", got, err)
	}
	if data, _ := os.ReadFile(Path(d, "a")); string(data) != "task: lets-b\n" {
		t.Errorf("content = %q", data)
	}
}

func TestSlugs_EveryTaskEntry(t *testing.T) {
	d := t.TempDir()
	for _, slug := range []string{"feature", "feature.12345.tmp", "feature.locked", "feature.tmp"} {
		write(t, d, slug, "task: lets-a\n")
	}
	_ = os.WriteFile(filepath.Join(d, "sessions", ".tasktmp-feature.1"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(d, "sessions", "2026-09-22-1000-lets-a-snapshot.md"), []byte("x"), 0o600)
	got, err := Slugs(d)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if want := []string{"feature", "feature.12345.tmp", "feature.locked", "feature.tmp"}; !reflect.DeepEqual(got, want) {
		t.Errorf("slugs = %v, want %v", got, want)
	}
	if got, err := Slugs(filepath.Join(d, "absent")); err != nil || len(got) != 0 {
		t.Errorf("a missing sessions dir is empty: %v %v", got, err)
	}
}

func TestSlug(t *testing.T) {
	if s, ok := Slug("feature/lets-abc-x"); !ok || s != "feature-lets-abc-x" {
		t.Errorf("Slug = %q %v", s, ok)
	}
	if _, ok := Slug(""); ok {
		t.Error("detached HEAD has no slug")
	}
}

func TestMergeWrite_RefreshLeavesNoTrace(t *testing.T) {
	root := t.TempDir()
	letsDir := filepath.Join(root, ".lets")
	if _, err := MergeWrite(letsDir, "main", WriteOpts{Set: map[string]string{"session": "0123456 0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"}}); !errors.Is(err, ErrFileAbsent) {
		t.Fatalf("err = %v, want ErrFileAbsent", err)
	}
	if _, err := os.Lstat(letsDir); !os.IsNotExist(err) {
		t.Errorf("a refresh of a missing file must not create %s (err=%v)", letsDir, err)
	}
}

const (
	gsOwn     = "aaaaaaaa-0000-4000-8000-000000000001"
	gsForeign = "bbbbbbbb-0000-4000-8000-000000000002"
	gsThird   = "cccccccc-0000-4000-8000-000000000003"
)

func sessionState(sid string) State {
	if sid == "" {
		return State{Task: "lets-x"}
	}
	return State{Task: "lets-x", Session: "1111111 " + sid}
}

func TestMaySetSession(t *testing.T) {
	live := func(v Liveness) func(string) Liveness {
		return func(string) Liveness { return v }
	}
	rot := func(to string) func(string) (string, bool) {
		return func(string) (string, bool) { return to, to != "" }
	}
	lead := func(ok bool) func(string) (bool, string) {
		return func(string) (bool, string) { return ok, "snake-lead (session cccccccc)" }
	}
	cases := []struct {
		name   string
		stored string
		op     SessionOp
		g      SessionGuard
		want   error  // nil, ErrSessionUnchanged or ErrSessionHeld
		reason string // HeldError reason when held
	}{
		{"empty", "", OpRefresh, SessionGuard{Liveness: live(LiveAlive)}, nil, ""},
		{"own", gsOwn, OpRefresh, SessionGuard{Liveness: live(LiveAlive)}, ErrSessionUnchanged, ""},
		{"dead", gsForeign, OpRefresh, SessionGuard{Liveness: live(LiveDead)}, nil, ""},
		{"live-foreign", gsForeign, OpRefresh, SessionGuard{Liveness: live(LiveAlive)}, ErrSessionHeld, HeldLive},
		{"degraded", gsForeign, OpRefresh, SessionGuard{Liveness: live(LiveUnknown)}, ErrSessionHeld, HeldUnknown},
		{"rotated-own", gsForeign, OpRefresh, SessionGuard{Liveness: live(LiveDead), Rotated: rot(gsOwn)}, nil, ""},
		{"rotated-foreign", gsForeign, OpRefresh, SessionGuard{Liveness: live(LiveDead), Rotated: rot(gsThird)}, ErrSessionHeld, HeldRotatedForeign},
		{"carry-proven", gsForeign, OpCarry, SessionGuard{Liveness: live(LiveDead), Rotated: rot(gsOwn)}, nil, ""},
		{"carry-no-proof", gsForeign, OpCarry, SessionGuard{Liveness: live(LiveDead)}, ErrSessionHeld, HeldNoProof},
		{"carry-no-proof-nil-guard", gsForeign, OpCarry, SessionGuard{}, ErrSessionHeld, HeldNoProof},
		{"carry-foreign-live", gsForeign, OpCarry, SessionGuard{Liveness: live(LiveAlive)}, ErrSessionHeld, HeldLive},
		{"carry-nothing-recorded", "", OpCarry, SessionGuard{}, ErrSessionUnchanged, ""},
		{"lead-refused", "", OpRefresh, SessionGuard{Lead: lead(false)}, ErrSessionHeld, HeldLead},
		{"lead-refused-even-own", gsOwn, OpRefresh, SessionGuard{Lead: lead(false)}, ErrSessionHeld, HeldLead},
		{"lead-allowed-dead", gsForeign, OpRefresh, SessionGuard{Lead: lead(true), Liveness: live(LiveDead)}, nil, ""},
		{"lead-allowed-live-foreign", gsForeign, OpRefresh, SessionGuard{Lead: lead(true), Liveness: live(LiveAlive)}, ErrSessionHeld, HeldLive},
	}
	for _, c := range cases {
		err := MaySetSession(sessionState(c.stored), true, gsOwn, c.op, c.g)
		if !errors.Is(err, c.want) || (c.want == nil && err != nil) {
			t.Errorf("%s: err=%v, want %v", c.name, err, c.want)
			continue
		}
		var held *HeldError
		if c.reason != "" && (!errors.As(err, &held) || held.Reason != c.reason) {
			t.Errorf("%s: err=%v, want reason %s", c.name, err, c.reason)
		}
	}
	// A malformed recorded line guards nothing: it is treated as empty.
	if err := MaySetSession(State{Session: "OLDSHA OLDSID"}, true, gsOwn, OpRefresh,
		SessionGuard{Liveness: live(LiveAlive)}); err != nil {
		t.Errorf("malformed line: err=%v", err)
	}
}

// No session registry (a Claude Code without one): nil Liveness keeps today's
// refresh - a recorded foreign session is replaced.
func TestMaySetSession_RegistryAbsentWrites(t *testing.T) {
	if err := MaySetSession(sessionState(gsForeign), true, gsOwn, OpRefresh, SessionGuard{}); err != nil {
		t.Errorf("err=%v, want the unguarded write", err)
	}
}

// An unparseable live registry entry might be the holder: unknown, so held.
func TestMaySetSession_UnparseableHeld(t *testing.T) {
	g := SessionGuard{Liveness: func(string) Liveness { return LiveUnknown }}
	var held *HeldError
	if err := MaySetSession(sessionState(gsForeign), true, gsOwn, OpRefresh, g); !errors.As(err, &held) || held.Reason != HeldUnknown {
		t.Errorf("err=%v, want held (unknown)", err)
	}
}

// park / park_team are known keys: validated, merged and deleted like the others,
// and a park_team line is never read as park.
func TestParkKeys(t *testing.T) {
	letsDir := t.TempDir()
	for _, bad := range []map[string]string{{"park": "not-a-sha"}, {"park_team": "Bad Team"}, {"park_team": "run-abc"}} {
		if _, err := MergeWrite(letsDir, "b", WriteOpts{Set: bad, Create: true}); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%v: err=%v, want ErrInvalidValue", bad, err)
		}
	}
	st, err := MergeWrite(letsDir, "b", WriteOpts{Set: map[string]string{"task": "lets-a1", "park": "abc1234", "park_team": "snake"}, Create: true})
	if err != nil || st.Park != "abc1234" || st.ParkTeam != "snake" || st.Task != "lets-a1" || len(st.Other) != 0 {
		t.Fatalf("state = %+v, %v", st, err)
	}
	st, err = MergeWrite(letsDir, "b", WriteOpts{Set: map[string]string{"park": "", "park_team": ""}})
	if err != nil || st.Park != "" || st.ParkTeam != "" || st.Task != "lets-a1" {
		t.Errorf("after delete: %+v, %v", st, err)
	}
}

func strp(s string) *string { return &s }

func orchestratorsOf(t *testing.T, st State) []Orchestrator {
	t.Helper()
	recs, bad := Orchestrators(st)
	if len(bad) != 0 {
		t.Errorf("unexpected malformed lines: %q", bad)
	}
	return recs
}

// Two names round-trip beside the single-valued keys and an unknown line, in file
// order; every other line is left exactly as it was.
func TestOrchestrator_TwoNamesRoundTrip(t *testing.T) {
	letsDir := t.TempDir()
	write(t, letsDir, "main", "task: lets-a1\nfuture: keep me\nsession: abc1234 "+sid+"\n")
	if _, err := MergeWrite(letsDir, "main", WriteOpts{PatchOrchestrator: &OrchestratorPatch{Name: "MAIN-LETS", Scope: strp("docs"), Standing: strp("on")}}); err != nil {
		t.Fatal(err)
	}
	st, err := MergeWrite(letsDir, "main", WriteOpts{PatchOrchestrator: &OrchestratorPatch{Name: "MAIN ETH"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Orchestrator{{Name: "MAIN-LETS", Scope: "docs", Standing: "on"}, {Name: "MAIN ETH"}}
	if got := orchestratorsOf(t, st); !reflect.DeepEqual(got, want) {
		t.Errorf("records = %+v, want %+v", got, want)
	}
	data, _ := os.ReadFile(Path(letsDir, "main"))
	wantFile := "task: lets-a1\nfuture: keep me\nsession: abc1234 " + sid + "\norchestrator: MAIN-LETS scope=docs standing=on\norchestrator: MAIN ETH\n"
	if string(data) != wantFile {
		t.Errorf("file:\n%s\nwant:\n%s", data, wantFile)
	}
	if st.Task != "lets-a1" || st.Session != "abc1234 "+sid || !reflect.DeepEqual(st.Other, []string{"future: keep me"}) {
		t.Errorf("other keys touched: %+v", st)
	}
	// a single-valued write keeps both lines
	st, err = MergeWrite(letsDir, "main", WriteOpts{Set: map[string]string{"task": "lets-b2"}})
	if err != nil || st.Task != "lets-b2" || len(orchestratorsOf(t, st)) != 2 {
		t.Errorf("after a task write: %+v, %v", st, err)
	}
	read, _ := Read(letsDir, "main")
	if !reflect.DeepEqual(orchestratorsOf(t, read), want) {
		t.Errorf("Read: %+v", read.OrchestratorLines)
	}
}

// A patch merges only the fields it carries: re-recording keeps scope and standing,
// a scope-only patch keeps standing, a standing-only patch keeps scope.
func TestOrchestrator_PatchKeepsUnsetFields(t *testing.T) {
	letsDir := t.TempDir()
	rec := func(p OrchestratorPatch) Orchestrator {
		t.Helper()
		st, err := MergeWrite(letsDir, "main", WriteOpts{PatchOrchestrator: &p, Create: true})
		if err != nil {
			t.Fatal(err)
		}
		recs := orchestratorsOf(t, st)
		if len(recs) != 1 {
			t.Fatalf("records = %+v", recs)
		}
		return recs[0]
	}
	if got := rec(OrchestratorPatch{Name: "MAIN-LETS"}); got != (Orchestrator{Name: "MAIN-LETS"}) {
		t.Errorf("a new name with no standing reads \"\": %+v", got)
	}
	if got := rec(OrchestratorPatch{Name: "MAIN-LETS", Scope: strp("docs"), Standing: strp("on")}); got != (Orchestrator{Name: "MAIN-LETS", Scope: "docs", Standing: "on"}) {
		t.Errorf("full patch: %+v", got)
	}
	if got := rec(OrchestratorPatch{Name: "MAIN-LETS"}); got != (Orchestrator{Name: "MAIN-LETS", Scope: "docs", Standing: "on"}) {
		t.Errorf("re-recording with no fields keeps scope and standing: %+v", got)
	}
	if got := rec(OrchestratorPatch{Name: "MAIN-LETS", Scope: strp("cli")}); got != (Orchestrator{Name: "MAIN-LETS", Scope: "cli", Standing: "on"}) {
		t.Errorf("a scope-only patch keeps standing: %+v", got)
	}
	if got := rec(OrchestratorPatch{Name: "MAIN-LETS", Standing: strp("off")}); got != (Orchestrator{Name: "MAIN-LETS", Scope: "cli", Standing: "off"}) {
		t.Errorf("a standing-only patch keeps scope: %+v", got)
	}
	if got := rec(OrchestratorPatch{Name: "MAIN-LETS", Scope: strp("")}); got != (Orchestrator{Name: "MAIN-LETS", Standing: "off"}) {
		t.Errorf("an empty scope removes it: %+v", got)
	}
}

// Clear removes one name and keeps the others; a duplicate line of a patched name
// collapses into one.
func TestOrchestrator_ClearAndDuplicates(t *testing.T) {
	letsDir := t.TempDir()
	write(t, letsDir, "main", "orchestrator: A standing=on\norchestrator: B scope=docs\norchestrator: A\n")
	st, err := MergeWrite(letsDir, "main", WriteOpts{ClearOrchestrator: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if got := orchestratorsOf(t, st); !reflect.DeepEqual(got, []Orchestrator{{Name: "A", Standing: "on"}, {Name: "A"}}) {
		t.Errorf("after clear B: %+v", got)
	}
	st, err = MergeWrite(letsDir, "main", WriteOpts{PatchOrchestrator: &OrchestratorPatch{Name: "A", Scope: strp("cli")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := orchestratorsOf(t, st); !reflect.DeepEqual(got, []Orchestrator{{Name: "A", Scope: "cli", Standing: "on"}}) {
		t.Errorf("a patched name keeps one line: %+v", got)
	}
	st, err = MergeWrite(letsDir, "main", WriteOpts{ClearOrchestrator: "A"})
	if err != nil || len(st.OrchestratorLines) != 0 {
		t.Errorf("clear A: %+v, %v", st, err)
	}
	if st, err := MergeWrite(letsDir, "main", WriteOpts{ClearOrchestrator: "NOBODY"}); err != nil || len(st.OrchestratorLines) != 0 {
		t.Errorf("clearing an unrecorded name is a no-op: %+v, %v", st, err)
	}
}

// A malformed line stays on disk through every write and is skipped (and named) by
// Orchestrators.
func TestOrchestrator_MalformedKeptAndSkipped(t *testing.T) {
	letsDir := t.TempDir()
	bad := []string{"MAIN standing=maybe", "-x", "A scope=", "A color=red", "A standing=on standing=off", "A standing=on extra"}
	var content strings.Builder
	for _, b := range bad {
		content.WriteString("orchestrator: " + b + "\n")
	}
	write(t, letsDir, "main", content.String())
	st, err := MergeWrite(letsDir, "main", WriteOpts{PatchOrchestrator: &OrchestratorPatch{Name: "MAIN"}, ClearOrchestrator: "A"})
	if err != nil {
		t.Fatal(err)
	}
	recs, gotBad := Orchestrators(st)
	if !reflect.DeepEqual(recs, []Orchestrator{{Name: "MAIN"}}) || !reflect.DeepEqual(gotBad, bad) {
		t.Errorf("records=%+v bad=%q", recs, gotBad)
	}
	data, _ := os.ReadFile(Path(letsDir, "main"))
	if !strings.HasPrefix(string(data), content.String()) {
		t.Errorf("malformed lines must stay on disk:\n%s", data)
	}
}

// Standing on / off and a multi-word scope parse; anything else is refused before
// any write.
func TestOrchestrator_ParseAndValidate(t *testing.T) {
	for v, want := range map[string]Orchestrator{
		"MAIN-LETS standing=on":               {Name: "MAIN-LETS", Standing: "on"},
		"MAIN-LETS standing=off":              {Name: "MAIN-LETS", Standing: "off"},
		"MAIN PWA scope=web app standing=off": {Name: "MAIN PWA", Scope: "web app", Standing: "off"},
		"MAIN standing=on scope=docs":         {Name: "MAIN", Scope: "docs", Standing: "on"},
	} {
		if got, ok := parseOrchestrator(v); !ok || got != want {
			t.Errorf("%q: %+v ok=%v, want %+v", v, got, ok, want)
		}
	}
	letsDir := t.TempDir()
	for _, o := range []WriteOpts{
		{PatchOrchestrator: &OrchestratorPatch{Name: "x --auto"}},
		{PatchOrchestrator: &OrchestratorPatch{Name: "MAIN", Standing: strp("yes")}},
		{PatchOrchestrator: &OrchestratorPatch{Name: "MAIN", Standing: strp("")}},
		{PatchOrchestrator: &OrchestratorPatch{Name: "MAIN", Scope: strp("a=b")}},
		{ClearOrchestrator: "a$b"},
	} {
		if _, err := MergeWrite(letsDir, "main", WriteOpts{PatchOrchestrator: o.PatchOrchestrator, ClearOrchestrator: o.ClearOrchestrator, Create: true}); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%+v: err=%v, want ErrInvalidValue", o, err)
		}
	}
	if _, err := os.Stat(Path(letsDir, "main")); !os.IsNotExist(err) {
		t.Error("a refused value must write nothing")
	}
}
