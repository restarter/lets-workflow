//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
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

// writeTranscript writes JSONL lines for sid as Claude Code would store them.
func writeTranscript(t *testing.T, home, cwd, sid string, lines ...map[string]any) string {
	t.Helper()
	dir := filepath.Join(home, "projects", transcriptSlug(cwd))
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
func toolUse(ts, id, name string) map[string]any {
	return map[string]any{"type": "assistant", "timestamp": ts, "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{"command": "sleep 1"}}}}}
}
func turnEnd(ts string) map[string]any {
	return map[string]any{"type": "system", "subtype": "turn_duration", "timestamp": ts}
}

// fakeOps is an in-memory Orca.
type fakeOps struct {
	mu         sync.Mutex
	terms      []orcaTerm
	source     string
	screen     []string
	sends      []string
	sendResult []*orcacmd.Failure // consumed per Send
	onSend     func(text string)
	termsCalls int
}

func (f *fakeOps) Terminals(context.Context) ([]orcaTerm, *orcacmd.Failure) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.termsCalls++
	return append([]orcaTerm(nil), f.terms...), nil
}
func (f *fakeOps) Screen(context.Context, string) (string, []string, *orcacmd.Failure) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.source
	if src == "" {
		src = "screen"
	}
	return src, append([]string(nil), f.screen...), nil
}
func (f *fakeOps) Send(_ context.Context, _ string, text string) (orcacmd.Receipt, *orcacmd.Failure) {
	f.mu.Lock()
	var fail *orcacmd.Failure
	if len(f.sendResult) > 0 {
		fail, f.sendResult = f.sendResult[0], f.sendResult[1:]
	}
	if fail == nil {
		f.sends = append(f.sends, text)
	}
	cb := f.onSend
	f.mu.Unlock()
	if fail != nil {
		return orcacmd.Receipt{}, fail
	}
	if cb != nil {
		cb(text)
	}
	return orcacmd.Receipt{InputAccepted: true, TurnStarted: true}, nil
}

func useOrca(t *testing.T, ops orcaOps) *int {
	t.Helper()
	calls := 0
	old := newOrcaOps
	newOrcaOps = func() (orcaOps, *orcacmd.Failure) { calls++; return ops, nil }
	t.Cleanup(func() { newOrcaOps = old })
	return &calls
}

func fastLoops(t *testing.T) {
	t.Helper()
	oldSleep, oldObs, oldWait := sleep, observeTimeout, sendLockWait
	sleep = func(time.Duration) {}
	observeTimeout, sendLockWait = 50*time.Millisecond, 2*time.Second
	oldLock := sendLockDir
	lock := t.TempDir()
	sendLockDir = func() string { return lock }
	t.Cleanup(func() { sleep, observeTimeout, sendLockWait, sendLockDir = oldSleep, oldObs, oldWait, oldLock })
}

var idleScreen = []string{"⏺ done", "────────", "❯", "────────"}

// repoWithLets is a git repo with a .lets dir and an isolated HOME.
func repoWithLets(t *testing.T, launcher string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(orcaTerminalVar, "")
	repo := gitRepo(t)
	_ = os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755)
	if launcher != "" {
		_ = os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_LAUNCHER="+launcher+"\n"), 0o644)
	}
	return repo
}
