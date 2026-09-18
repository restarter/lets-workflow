//go:build unix

package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testMarker  = "/w/.lets/handoffs/b.md"
	liveSession = "01a0b4e3-d92f-7671-899b-87e658914765"
)

var testSince = time.Date(2026, 9, 18, 14, 33, 43, 0, time.UTC)

// useClock replaces now and sleep with a fake clock sleep advances.
func useClock(t *testing.T) {
	t.Helper()
	oldNow, oldSleep := now, sleep
	clock := time.Now()
	now = func() time.Time { return clock }
	sleep = func(d time.Duration) { clock = clock.Add(d) }
	t.Cleanup(func() { now, sleep = oldNow, oldSleep })
}

// useCodexHome points the Codex home at a temp dir and returns it.
func useCodexHome(t *testing.T) string {
	t.Helper()
	old := codexHome
	home := t.TempDir()
	codexHome = func() string { return home }
	t.Cleanup(func() { codexHome = old })
	return home
}

// placeRollout copies a fixture into home's session store under a date directory.
func placeRollout(t *testing.T, home, fixture, day, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, "sessions", "2026", "09", day, name)
	write(t, p, string(b))
	return p
}

func awaitRequest(t *testing.T) AwaitRequest {
	t.Helper()
	return AwaitRequest{Marker: testMarker, Since: testSince, OutBase: filepath.Join(t.TempDir(), "b")}
}

func TestScanRollout(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		marker  []byte
		state   turnState
		message string
		root    bool
	}{
		{"rollout-live.jsonl", []byte(testMarker), stateComplete, "ROOT FINAL", true},
		{"rollout-tool-output.jsonl", []byte(testMarker), stateNoMarker, "", true},
		{"rollout-other-turn.jsonl", []byte(testMarker), statePending, "", true},
		{"rollout-subagent.jsonl", []byte(testMarker), stateComplete, "SUB", false},
		{"rollout-before-since.jsonl", []byte(testMarker), stateNoMarker, "", true},
		{"rollout-aborted.jsonl", []byte(testMarker), stateAborted, "", true},
		{"rollout-live.jsonl", nil, stateComplete, "ROOT FINAL", true},
	} {
		s := scanRollout(filepath.Join("testdata", tc.fixture), tc.marker, testSince)
		if s.state != tc.state || s.message != tc.message || s.root != tc.root {
			t.Errorf("%s (marker %v): %+v", tc.fixture, tc.marker != nil, s)
		}
	}
	if s := scanRollout(filepath.Join("testdata", "rollout-live.jsonl"), []byte(testMarker), testSince); s.sessionID != liveSession {
		t.Errorf("session id %q", s.sessionID)
	}
}

func TestAwait_Complete(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	p := placeRollout(t, home, "rollout-live.jsonl", "18", "rollout-2026-09-18T16-20-26-"+liveSession+".jsonl")
	res := codex{}.Await(context.Background(), awaitRequest(t))
	if !res.Complete || res.SessionID != liveSession || res.RolloutPath != p {
		t.Fatalf("result: %+v", res)
	}
	if b, _ := os.ReadFile(res.ReportPath); string(b) != "ROOT FINAL" {
		t.Errorf("report %q", b)
	}
}

func TestAwait_SessionStartedDaysAgo(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	placeRollout(t, home, "rollout-live.jsonl", "11", "rollout-2026-09-11T09-00-00-"+liveSession+".jsonl")
	if res := (codex{}).Await(context.Background(), awaitRequest(t)); !res.Complete {
		t.Errorf("a tab started a week ago must be found (the v1 gap): %+v", res)
	}
}

func TestAwait_Ambiguous(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	a := placeRollout(t, home, "rollout-live.jsonl", "18", "rollout-a.jsonl")
	b := placeRollout(t, home, "rollout-live.jsonl", "18", "rollout-b.jsonl")
	res := codex{}.Await(context.Background(), awaitRequest(t))
	if res.Complete || res.Reason != ReasonMarkerAmbiguous || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], a) || !strings.Contains(res.Warnings[0], b) {
		t.Errorf("result: %+v", res)
	}
}

func TestAwait_SubagentOnly(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	placeRollout(t, home, "rollout-subagent.jsonl", "18", "rollout-sub.jsonl")
	r := awaitRequest(t)
	r.Timeout = 50 * time.Millisecond
	if res := (codex{}).Await(context.Background(), r); res.Complete || res.Reason != ReasonMarkerNotFound {
		t.Errorf("result: %+v", res)
	}
}

func TestAwait_PendingTimesOut(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	p := placeRollout(t, home, "rollout-other-turn.jsonl", "18", "rollout-pending.jsonl")
	r := awaitRequest(t)
	r.Timeout = 50 * time.Millisecond
	if res := (codex{}).Await(context.Background(), r); res.Reason != ReasonTimeout || res.RolloutPath != p {
		t.Errorf("result: %+v", res)
	}
}

func TestAwait_Aborted(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	placeRollout(t, home, "rollout-aborted.jsonl", "18", "rollout-aborted.jsonl")
	if res := (codex{}).Await(context.Background(), awaitRequest(t)); res.Reason != ReasonTurnAborted {
		t.Errorf("result: %+v", res)
	}
}

func TestAwait_DriftWarns(t *testing.T) {
	useClock(t)
	home := useCodexHome(t)
	placeRollout(t, home, "rollout-live.jsonl", "18", "rollout-live.jsonl")
	old := gitOut
	gitOut = func(string, ...string) ([]byte, error) { return []byte("changed"), nil }
	t.Cleanup(func() { gitOut = old })
	r := awaitRequest(t)
	r.Dir, r.Fingerprint = t.TempDir(), "send-time"
	res := codex{}.Await(context.Background(), r)
	if !res.Complete || !res.WorkspaceChanged || len(res.Warnings) != 1 {
		t.Errorf("result: %+v", res)
	}
}

func TestAwait_NeverOverwritesReport(t *testing.T) {
	useClock(t)
	useCodexHome(t)
	r := awaitRequest(t)
	report, _, _ := Outputs(r.OutBase)
	write(t, report, "old")
	if res := (codex{}).Await(context.Background(), r); res.Reason != ReasonReportExists {
		t.Errorf("result: %+v", res)
	}
	if b, _ := os.ReadFile(report); string(b) != "old" {
		t.Errorf("report overwritten: %q", b)
	}
}
