//go:build unix

package ccregistry

import (
	"errors"
	"os"
	"testing"
)

// fakeTree makes ParentOf answer from parents (a pid missing from it is an error).
func fakeTree(t *testing.T, parents map[int]int) {
	t.Helper()
	old := ParentOf
	ParentOf = func(pid int) (int, error) {
		if p, ok := parents[pid]; ok {
			return p, nil
		}
		return 0, errors.New("no such process")
	}
	t.Cleanup(func() { ParentOf = old })
}

const (
	sidA = "aaaaaaaa-0000-4000-8000-00000000000a"
	sidB = "bbbbbbbb-0000-4000-8000-00000000000b"
)

// The chain above this test process: pp (shell) -> 500 (Claude B) -> 600 (make) ->
// 700 (Claude A) -> 1. Each case adjusts the snapshot or the tree.
func TestCallerSession(t *testing.T) {
	pp := os.Getppid()
	chain := map[int]int{pp: 500, 500: 600, 600: 700, 700: 1}
	a := Entry{Pid: 700, SessionID: sidA, Name: "A", NameOK: true}
	b := Entry{Pid: 500, SessionID: sidB, Name: "B", NameOK: true}
	for _, tc := range []struct {
		name     string
		parents  map[int]int
		snap     Snapshot
		want     string // the session id; "" = an error
		notFound bool   // the error is ErrNoCallerSession
	}{
		{"nested session: the nearest wins", chain, Snapshot{Entries: []Entry{a, b}}, sidB, false},
		{"wrappers beneath A are transparent", chain, Snapshot{Entries: []Entry{a}}, sidA, false},
		{"direct parent is the session", map[int]int{pp: 1}, Snapshot{Entries: []Entry{{Pid: pp, SessionID: sidA}}}, sidA, false},
		{"unidentified Claude process in between", chain, Snapshot{Entries: []Entry{a}, Unrecognized: map[int]string{500: "peerProtocol=2"}}, "", false},
		{"loose-only Claude process in between", chain, Snapshot{Entries: []Entry{a}, Loose: map[string]int{sidB: 500}}, "", false},
		{"no registered ancestor", chain, Snapshot{}, "", true},
		{"self-parented stops", map[int]int{pp: 900, 900: 900}, Snapshot{Entries: []Entry{a}}, "", true},
		{"unreadable hop", map[int]int{pp: 500}, Snapshot{Entries: []Entry{a}}, "", false},
		{"cycle hits the bound", map[int]int{pp: 800, 800: 801, 801: 800}, Snapshot{Entries: []Entry{a}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeTree(t, tc.parents)
			e, err := tc.snap.CallerSession()
			switch {
			case tc.want != "" && (err != nil || e.SessionID != tc.want):
				t.Errorf("got %q, %v; want %q", e.SessionID, err, tc.want)
			case tc.want == "" && err == nil:
				t.Errorf("got %q; want an error", e.SessionID)
			case tc.want == "" && errors.Is(err, ErrNoCallerSession) != tc.notFound:
				t.Errorf("err=%v; ErrNoCallerSession=%v", err, tc.notFound)
			}
		})
	}
}

// The real platform walker runs: outside a Claude Code session (an empty snapshot)
// it walks to init and answers ErrNoCallerSession, never another error.
func TestCallerSession_RealWalk(t *testing.T) {
	if _, err := (Snapshot{}).CallerSession(); !errors.Is(err, ErrNoCallerSession) {
		t.Errorf("an empty registry over the real process tree: %v, want ErrNoCallerSession", err)
	}
	if p, err := parentOf(os.Getpid()); err != nil || p != os.Getppid() {
		t.Errorf("parentOf(self) = %d, %v; want %d", p, err, os.Getppid())
	}
	if e, err := (Snapshot{Entries: []Entry{{Pid: os.Getppid(), SessionID: sidA}}}).CallerSession(); err != nil || e.SessionID != sidA {
		t.Errorf("the real direct parent as the session: %q, %v", e.SessionID, err)
	}
}
