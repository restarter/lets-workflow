//go:build unix

package peerscmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

type regRow struct {
	pid       int
	sid, name string
	cwd       string
}

// claudeHome builds a Claude config dir with registry rows (all alive) plus
// unknown-protocol live pids, and points the registry seams at it.
func claudeHome(t *testing.T, rows []regRow, unknown ...int) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	alive := map[int]bool{}
	for _, r := range rows {
		b, _ := json.Marshal(map[string]any{"sessionId": r.sid, "name": r.name, "cwd": r.cwd, "peerProtocol": 1, "status": "idle"})
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoa(r.pid)+".json"), b, 0o600)
		alive[r.pid] = true
	}
	for _, pid := range unknown {
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoa(pid)+".json"), []byte(`{"peerProtocol":2}`), 0o600)
		alive[pid] = true
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return dir }
	ccregistry.ProcAlive = func(pid int) bool { return alive[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
	return dir
}

// projectSlug is Claude Code's project directory name for cwd: every byte outside
// [A-Za-z0-9] becomes '-' (peertail's transcriptSlug).
func projectSlug(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

// writeTranscript writes JSONL lines for sid as Claude Code would store them.
func writeTranscript(t *testing.T, home, cwd, sid string, lines ...map[string]any) string {
	t.Helper()
	dir := filepath.Join(home, "projects", projectSlug(cwd))
	_ = os.MkdirAll(dir, 0o700)
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteByte('\n')
	}
	p := filepath.Join(dir, sid+".jsonl")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func userText(ts, text string) map[string]any {
	return map[string]any{"type": "user", "timestamp": ts, "message": map[string]any{"role": "user", "content": text}}
}
func assistantText(ts, text string) map[string]any {
	return map[string]any{"type": "assistant", "timestamp": ts, "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}}}
}
func turnEnd(ts string) map[string]any {
	return map[string]any{"type": "system", "subtype": "turn_duration", "timestamp": ts}
}

const msg1 = "0123456789abcdef"

// header is a legacy v0.10.0 message header from sidMain to `to`.
func header(id, to string) string {
	return "[lets-peer id=" + id + " kind=ask from_sid=" + sidMain + " to_sid=" + to + ` from="peer/MAIN" to="MAIN-FABLE"]`
}

// nativeDelivery wraps text the way Claude Code delivers a SendMessage: the prefix
// line, the cross-session-message wrapper with the sender's name, the text.
func nativeDelivery(from, text string) string {
	return "Another Claude session sent a message:\n<cross-session-message from=\"uds:/tmp/cc-socks/1.sock\" from-name=\"" + from + "\" from-mode=\"default\">\n" + text + "\n</cross-session-message>"
}

// repoWithLets is a git repo with a .lets dir and an isolated HOME.
func repoWithLets(t *testing.T, launcher string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := gitRepo(t)
	_ = os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755)
	if launcher != "" {
		_ = os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_LAUNCHER="+launcher+"\n"), 0o644)
	}
	return repo
}
