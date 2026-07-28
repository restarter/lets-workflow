//go:build unix

package notifycmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
