//go:build unix

package orcacmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLookOrca_BundleWinsAndPathNeedsEnvelope(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "bundle-orca")
	pathBin := filepath.Join(dir, "path-orca")
	for _, p := range []string{bundle, pathBin} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldBundle, oldLook, oldProbe := bundlePaths, lookPath, probeOrca
	t.Cleanup(func() { bundlePaths, lookPath, probeOrca = oldBundle, oldLook, oldProbe })
	lookPath = func(string) (string, error) { return pathBin, nil }

	bundlePaths = func() []string { return []string{bundle} }
	probeOrca = func(string) bool { return true }
	if got, ok := lookOrca(); !ok || got != bundle {
		t.Errorf("bundle must win: %q %v", got, ok)
	}

	bundlePaths = func() []string { return []string{filepath.Join(dir, "missing")} }
	if got, ok := lookOrca(); !ok || got != pathBin {
		t.Errorf("PATH hit with envelope: %q %v", got, ok)
	}
	probeOrca = func(string) bool { return false } // e.g. the GNOME screen reader
	if _, ok := lookOrca(); ok {
		t.Error("a PATH hit that fails the probe must not be used")
	}
}

func TestProbeOrca_RejectsNonOrcaBinary(t *testing.T) {
	old := probeTimeout
	probeTimeout = 20 * time.Second // exec of a fresh script can exceed 2s while the full suite runs in parallel
	t.Cleanup(func() { probeTimeout = old })
	dir := t.TempDir()
	liar := filepath.Join(dir, "orca")
	if err := os.WriteFile(liar, []byte("#!/bin/sh\necho ok\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if probeOrca(liar) {
		t.Error("an exit-0 binary without Orca's envelope must be rejected")
	}
	real := filepath.Join(dir, "orca-real")
	if err := os.WriteFile(real, []byte("#!/bin/sh\necho '"+statusRunning+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !probeOrca(real) {
		t.Error("Orca's status envelope must be accepted")
	}
}

func TestStatus_Shapes(t *testing.T) {
	cases := []struct {
		out, reason, version string
		running              bool
	}{
		{statusRunning, "", "1.4.203", true},
		{`{"ok":true,"result":{"app":{"running":false}}}`, ReasonAppNotRunning, "", false},
		{`{"ok":true,"result":{"something":1}}`, ReasonStatusUnrecognized, "", false},
		{`not json`, ReasonStatusUnrecognized, "", false},
	}
	for _, c := range cases {
		useFakeOrca(t, func([]string) (string, string, bool) { return c.out, "", false })
		info, f := (&Client{Bin: "/fake/orca"}).Status(context.Background())
		if (f == nil) != (c.reason == "") || (f != nil && f.Reason != c.reason) || info.Running != c.running || info.Version != c.version {
			t.Errorf("%s: info=%+v f=%v", c.out, info, f)
		}
	}
}

func TestClassify(t *testing.T) {
	jsonUsage := `{"ok":false,"error":{"code":"invalid_argument","message":"Unknown flag --bogus for command: status"}}`
	if f := classify("status", []byte(jsonUsage), nil); f.Reason != ReasonCapabilityMissing {
		t.Errorf("json usage: %+v", f)
	}
	if f := classify("terminal read", nil, []byte("Unknown command: terminal read")); f.Reason != ReasonCapabilityMissing {
		t.Errorf("stderr usage: %+v", f)
	}
	if f := classify("terminal send", nil, []byte("error: terminal_handle_stale")); f.Reason != ReasonHandleStale {
		t.Errorf("stale: %+v", f)
	}
	f := classify("x", nil, []byte("boom token=ghp_abcdefghijklmnopqrstuvwxyz0123 "+strings.Repeat("y", 400)))
	if f.Reason != ReasonError || strings.Contains(f.Detail, "ghp_") || len(f.Detail) > detailCap {
		t.Errorf("detail must be redacted and capped: %+v", f)
	}
}

func TestWithHandleRetry(t *testing.T) {
	n := 0
	call := func(h string) *Failure {
		n++
		if h == "old" {
			return &Failure{Reason: ReasonHandleStale, Verb: "send"}
		}
		return nil
	}
	if f := WithHandleRetry(context.Background(), "old", func() (string, error) { return "new", nil }, call); f != nil || n != 2 {
		t.Errorf("stale then relist then success: f=%v calls=%d", f, n)
	}
	n = 0
	if f := WithHandleRetry(context.Background(), "old", func() (string, error) { return "old", nil }, call); f == nil || f.Reason != ReasonHandleStale || n != 2 {
		t.Errorf("stale twice: f=%v calls=%d", f, n)
	}
}

func TestSelfWorktree_RejectsAnotherPath(t *testing.T) {
	wd, _ := os.Getwd()
	top := strings.TrimSuffix(wd, "/cli/internal/orcacmd")
	t.Setenv("ORCA_WORKTREE_ID", "repo::"+top)
	useFakeOrca(t, func(args []string) (string, string, bool) {
		return `{"ok":true,"result":{"worktrees":[{"worktreeId":"repo::` + top + `","path":"/somewhere/else"}]}}`, "", false
	})
	if _, f := (&Client{Bin: "/fake/orca"}).SelfWorktree(context.Background()); f == nil || f.Reason != ReasonEnvMismatch {
		t.Errorf("an id whose path is another worktree must be rejected: %v", f)
	}
	t.Setenv("ORCA_WORKTREE_ID", "")
	if _, f := (&Client{Bin: "/fake/orca"}).SelfWorktree(context.Background()); f == nil || f.Reason != ReasonEnvAbsent {
		t.Errorf("absent env: %v", f)
	}
}

func TestParseReceipt(t *testing.T) {
	r, f := ParseReceipt([]byte(`{"result":{"send":{"accepted":true,"prompt":{"stages":["input_accepted"]}}}}`))
	if f != nil || !r.InputAccepted || r.TurnStarted {
		t.Errorf("queued send: %+v %v", r, f)
	}
	r, _ = ParseReceipt([]byte(`{"result":{"send":{"accepted":true,"prompt":{"stages":["input_accepted","turn_started"]}}}}`))
	if !r.TurnStarted {
		t.Errorf("started: %+v", r)
	}
}
