//go:build unix

package notifycmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// writeEnv creates <root>/.lets/.env with LETS_LAUNCHER=<launcher> (omitted when
// empty) and returns root.
func writeEnv(t *testing.T, launcher string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".lets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "LETS_LANGUAGE=English\n"
	if launcher != "" {
		body += "LETS_LAUNCHER=" + launcher + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNotify_TitleRequired(t *testing.T) {
	res, err := Notify(context.Background(), Options{})
	if err == nil || res.OK {
		t.Fatal("missing --title must be a hard error")
	}
	if ec, ok := err.(interface{ ExitCode() int }); !ok || ec.ExitCode() != 2 {
		t.Errorf("want ExitUsage(2), got %v", err)
	}
}

func TestResolveLauncher(t *testing.T) {
	if got := ResolveLauncher(writeEnv(t, "tmux"), ""); got != "tmux" {
		t.Errorf("ResolveLauncher = %q, want tmux", got)
	}
	// Absent LETS_LAUNCHER falls back to the canonical default (terminal).
	if got := ResolveLauncher(writeEnv(t, ""), ""); got != "terminal" {
		t.Errorf("ResolveLauncher (unset) = %q, want terminal", got)
	}
}

func TestNotify_TerminalIsNoop(t *testing.T) {
	root := writeEnv(t, "terminal")
	res, err := Notify(context.Background(), Options{Title: "gate", ProjectRoot: root})
	if err != nil || !res.OK {
		t.Fatalf("terminal must degrade gracefully: err=%v ok=%v", err, res.OK)
	}
	if res.Notify.Notified || res.Notify.Reason != "launcher_terminal" {
		t.Fatalf("notify = %+v", res.Notify)
	}
	if res.Notify.Launcher != "terminal" {
		t.Errorf("Launcher = %q, want terminal", res.Notify.Launcher)
	}
}

func TestNotify_UnknownLauncher(t *testing.T) {
	root := writeEnv(t, "zellij")
	res, err := Notify(context.Background(), Options{Title: "gate", ProjectRoot: root})
	if err != nil || !res.OK {
		t.Fatalf("unknown launcher must degrade: err=%v ok=%v", err, res.OK)
	}
	if res.Notify.Notified || res.Notify.Reason != "launcher_unknown" {
		t.Fatalf("notify = %+v", res.Notify)
	}
	if res.Notify.Launcher != "zellij" {
		t.Errorf("Launcher = %q, want zellij (echoed verbatim)", res.Notify.Launcher)
	}
}

func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; a bump is a breaking change for the gate snippets", SchemaVersion)
	}
	res := &Result{SchemaVersion: SchemaVersion, OK: true, Subcommand: "notify", Steps: []Step{}, Notify: &Info{Notified: false, Launcher: "terminal", Reason: "launcher_terminal"}}
	b, _ := json.Marshal(res)
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"schema_version", "ok", "subcommand", "steps", "notify"} {
		if _, ok := got[k]; !ok {
			t.Errorf("envelope missing key %q", k)
		}
	}
}

func TestNotify_OrcaDispatchesThroughSeam(t *testing.T) {
	old := orcacmd.NotifyFunc
	t.Cleanup(func() { orcacmd.NotifyFunc = old })
	var got orcacmd.NotifyOptions
	orcacmd.NotifyFunc = func(_ context.Context, o orcacmd.NotifyOptions) (*orcacmd.NotifyResult, error) {
		got = o
		return &orcacmd.NotifyResult{
			Envelope: orcacmd.Envelope{OK: true, Steps: []orcacmd.Step{{Status: "ok", Message: "card comment set"}}},
			Notify:   &orcacmd.NotifyInfo{Notified: true, Target: "r::/w", Title: o.Title},
		}, nil
	}
	res, err := Notify(context.Background(), Options{Title: "Plan ready", Body: "b", Cwd: "/w", ProjectRoot: writeEnv(t, "orca")})
	if err != nil || !res.OK || !res.Notify.Notified || res.Notify.Launcher != "orca" || res.Notify.Target != "r::/w" || got.Cwd != "/w" {
		t.Fatalf("err=%v notify=%+v got=%+v", err, res.Notify, got)
	}
	if len(res.Steps) != 1 || res.Steps[0].Message != "card comment set" {
		t.Errorf("steps not converted: %+v", res.Steps)
	}

	orcacmd.NotifyFunc = func(context.Context, orcacmd.NotifyOptions) (*orcacmd.NotifyResult, error) { return nil, nil }
	res, _ = Notify(context.Background(), Options{Title: "t", ProjectRoot: writeEnv(t, "orca")})
	if res.Notify.Notified || res.Notify.Reason != "launcher_error" {
		t.Errorf("nil result must degrade to launcher_error: %+v", res.Notify)
	}
}
