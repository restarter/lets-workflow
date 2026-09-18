//go:build unix

package handoffcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/agentrun"
)

// briefIn writes a brief into a fresh checkout's .lets/handoffs.
func briefIn(t *testing.T) (root, brief string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "w")
	brief = filepath.Join(handoffsDir(t, root), "2026-09-18-1600-lets-w5tm5-handoff.md")
	if err := os.WriteFile(brief, []byte("# brief\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, brief
}

func TestAwait_UnsupportedAgent(t *testing.T) {
	root, brief := briefIn(t)
	res, err := Await(context.Background(), AwaitOptions{Root: root, Brief: brief, Agent: "Bad Name", Since: time.Now()})
	if err != nil || !res.OK || res.Run.Ran || res.Run.Reason != "await_unsupported_agent" {
		t.Errorf("await: %+v %v", res.Run, err)
	}
}

func TestAwait_NeedsAgentAndSince(t *testing.T) {
	root, brief := briefIn(t)
	for _, o := range []AwaitOptions{{Agent: "codex"}, {Since: time.Now()}} {
		o.Root, o.Brief = root, brief
		res, err := Await(context.Background(), o)
		var he *Error
		if res.OK || !errors.As(err, &he) || he.ExitCode() != ExitUsage {
			t.Errorf("%+v: %v", o, err)
		}
	}
}

func TestAwait_RoutesByAgent(t *testing.T) {
	root, brief := briefIn(t)
	base := OutBase(brief)
	for path, text := range map[string]string{agentrun.AgentReport(base): "AG FINDINGS", agentrun.AgentDone(base): ""} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Await(context.Background(), AwaitOptions{Root: root, Brief: brief, Agent: "antigravity", Since: time.Now().Add(-time.Minute), Timeout: time.Second})
	if err != nil || !res.Run.Complete || res.Run.Provider != "antigravity" {
		t.Fatalf("await: %+v %v", res.Run, err)
	}
	if b, _ := os.ReadFile(res.Run.ReportPath); string(b) != "AG FINDINGS" {
		t.Errorf("report %q", b)
	}
}

func TestCodex_BriefInvalid(t *testing.T) {
	root, _ := briefIn(t)
	res, err := Codex(context.Background(), RunOptions{Root: root, Brief: "/etc/hosts"})
	var he *Error
	if res.OK || !errors.As(err, &he) || he.ExitCode() != ExitBriefInvalid || res.Error.Kind != "brief_invalid" {
		t.Errorf("codex: %+v %v", res.Envelope, err)
	}
}

func TestCodex_NoResultFile(t *testing.T) {
	root, brief := briefIn(t)
	t.Setenv("PATH", t.TempDir()) // no codex on PATH: a named degrade, still no result file
	res, err := Codex(context.Background(), RunOptions{Root: root, Brief: brief})
	if err != nil || !res.OK || res.Run.Ran || res.Run.Reason != "codex_not_found" {
		t.Fatalf("codex: %+v %v", res.Run, err)
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(brief), "*-result.json")); len(m) != 0 {
		t.Errorf("result files: %v", m)
	}
}
