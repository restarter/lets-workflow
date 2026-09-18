//go:build unix

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"testing"
)

func TestHandoffCmd_Wiring(t *testing.T) {
	root := NewRootCmd()
	h, _, err := root.Find([]string{"handoff"})
	if err != nil || h.Name() != "handoff" {
		t.Fatalf("handoff not wired: %v", err)
	}
	want := map[string][]string{
		"targets": {"json", "match"},
		"send":    {"json", "brief", "terminal", "new"},
		"codex":   {"json", "brief", "timeout"},
		"await":   {"json", "brief", "agent", "since", "fingerprint", "timeout"},
	}
	for sub, flags := range want {
		c, _, err := root.Find([]string{"handoff", sub})
		if err != nil || c.Name() != sub {
			t.Errorf("handoff %s: %v", sub, err)
			continue
		}
		for _, f := range flags {
			if c.Flags().Lookup(f) == nil {
				t.Errorf("handoff %s lacks --%s", sub, f)
			}
		}
	}
}

// runHandoff executes `lets handoff ...` in a fresh git repo and decodes the JSON.
func runHandoff(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	t.Chdir(dir)
	root := NewRootCmd()
	root.SetArgs(append([]string{"handoff"}, args...))
	var out bytes.Buffer
	root.SetOut(&out)
	runErr := root.Execute()
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	return env, runErr
}

type exitCoder interface{ ExitCode() int }

func TestHandoff_SendBriefInvalid(t *testing.T) {
	env, err := runHandoff(t, "send", "--brief", "/etc/hosts", "--new", "codex", "--json")
	var ec exitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != 10 {
		t.Fatalf("exit: %v", err)
	}
	e, _ := env["error"].(map[string]any)
	if env["ok"] != false || e["kind"] != "brief_invalid" {
		t.Errorf("envelope: %v", env)
	}
}

// What cobra rejects before RunE - a bad flag value, a stray argument, an unknown
// flag (--json after it included) - is a usage envelope with exit 2, not a bare
// exit 1 (review of lets-w5tm5, C6).
func TestHandoff_ParseErrorsAreEnvelopes(t *testing.T) {
	for _, args := range [][]string{
		{"codex", "--json", "--timeout", "nonsense"},
		{"targets", "extra", "--json"},
		{"send", "--bogus", "--json"},
	} {
		old := handoffArgs
		handoffArgs = func() []string { return append([]string{"handoff"}, args...) }
		env, err := runHandoff(t, args...)
		handoffArgs = old
		var ec exitCoder
		e, _ := env["error"].(map[string]any)
		if !errors.As(err, &ec) || ec.ExitCode() != 2 || env["ok"] != false || e["kind"] != "usage" {
			t.Errorf("%v: exit %v envelope %v", args, err, env)
		}
	}
	old := handoffArgs
	handoffArgs = func() []string { return []string{"handoff", "send", "--bogus"} }
	defer func() { handoffArgs = old }()
	dir := t.TempDir()
	t.Chdir(dir)
	root := NewRootCmd()
	root.SetArgs([]string{"handoff", "send", "--bogus"})
	var out bytes.Buffer
	root.SetOut(&out)
	err := root.Execute()
	var ec exitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != 2 || out.Len() != 0 {
		t.Errorf("without --json: exit %v, stdout %q", err, out.String())
	}
}

func TestHandoff_AwaitBadSince(t *testing.T) {
	env, err := runHandoff(t, "await", "--brief", "x", "--since", "yesterday", "--json")
	var ec exitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Fatalf("exit: %v", err)
	}
	e, _ := env["error"].(map[string]any)
	if e["kind"] != "usage" {
		t.Errorf("envelope: %v", env)
	}
}
