//go:build unix

package orcacmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

const wakeSid = "aaaaaaaa-2222-4222-8222-000000000001"

// registryWith writes registry entries and marks the given pids alive.
func registryWith(t *testing.T, entries map[int]string, unknown []int, alive ...int) {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	for pid, sid := range entries {
		b, _ := json.Marshal(map[string]any{"sessionId": sid, "name": "MAIN", "peerProtocol": 1})
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoaT(pid)+".json"), b, 0o600)
	}
	for _, pid := range unknown {
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoaT(pid)+".json"), []byte(`{"peerProtocol":2}`), 0o600)
	}
	set := map[int]bool{}
	for _, p := range alive {
		set[p] = true
	}
	oh, oa := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return dir }
	ccregistry.ProcAlive = func(pid int) bool { return set[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oh, oa })
}

func itoaT(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestWake_Liveness(t *testing.T) {
	repo := mainCheckout(t)
	calls := useFakeOrca(t, func([]string) (string, string, bool) { return statusRunning, "", false })
	registryWith(t, map[int]string{501: wakeSid}, nil, 501)
	if res, _ := Wake(context.Background(), WakeOptions{Repo: repo, Session: wakeSid, Pid: 501, Title: "MAIN"}); res.Wake.Reason != "main_alive" || len(*calls) != 0 {
		t.Errorf("alive: %+v calls=%d", res.Wake, len(*calls))
	}
	registryWith(t, nil, []int{502}, 502)
	if res, _ := Wake(context.Background(), WakeOptions{Repo: repo, Session: wakeSid, Pid: 502, Title: "MAIN"}); res.Wake.Reason != "liveness_unknown" || len(*calls) != 0 {
		t.Errorf("unknown: %+v calls=%d", res.Wake, len(*calls))
	}
	if _, err := Wake(context.Background(), WakeOptions{Repo: t.TempDir(), Session: wakeSid, Title: "MAIN"}); err == nil {
		t.Error("a non-checkout repo must be refused")
	}
}

func TestWake_ArgvAndStaleRetry(t *testing.T) {
	repo := mainCheckout(t)
	registryWith(t, nil, []int{601}, 601) // other live entries unrecognized; the recorded pid 600 is dead
	waits := 0
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch strings.Join(args[:2], " ") {
		case "status --json":
			return statusRunning, "", false
		case "terminal create":
			return `{"ok":true,"result":{"terminal":{"handle":"term_old"}}}`, "", false
		case "terminal list":
			return `{"ok":true,"result":{"terminals":[{"handle":"term_new","title":"MAIN"},{"handle":"term_other","title":"x"}]}}`, "", false
		case "terminal wait":
			waits++
			if waits == 1 {
				return `{"ok":false,"error":{"code":"terminal_handle_stale","message":"stale"}}`, "", true
			}
			return `{"ok":true,"result":{"wait":{"satisfied":true}}}`, "", false
		}
		return "{}", "", false
	})
	res, err := Wake(context.Background(), WakeOptions{Repo: repo, Session: wakeSid, Pid: 600, Title: "MAIN"})
	if err != nil || !res.Wake.Woken || !res.Wake.Satisfied || res.Wake.Handle != "term_new" {
		t.Fatalf("wake: %+v %v", res.Wake, err)
	}
	var create []string
	for _, c := range *calls {
		if c.args[0] == "terminal" && c.args[1] == "create" {
			create = c.args
		}
	}
	want := []string{"terminal", "create", "--worktree", "path:" + repo, "--title", "MAIN", "--command", "claude -r " + wakeSid, "--json"}
	if strings.Join(create, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("argv:\n got %q\nwant %q", create, want)
	}
}

func TestRepos_DropsNonCheckout(t *testing.T) {
	good := mainCheckout(t)
	notRepo := t.TempDir()
	useFakeOrca(t, func(args []string) (string, string, bool) {
		if args[0] == "status" {
			return statusRunning, "", false
		}
		b, _ := json.Marshal(map[string]any{"ok": true, "result": map[string]any{"repos": []map[string]string{
			{"path": notRepo, "displayName": "plain dir"}, {"path": good, "displayName": "good"}, {"path": "/does/not/exist", "displayName": "gone"},
		}}})
		return string(b), "", false
	})
	info, f := ListRepos(context.Background())
	if f != nil || len(info.Repos) != 1 || info.Repos[0].Index != 1 || info.Repos[0].Path != good || len(info.Dropped) != 2 {
		t.Errorf("repos: %+v %v", info, f)
	}
	if p, f := RepoByIndex(context.Background(), 1); f != nil || p != good {
		t.Errorf("by index: %q %v", p, f)
	}
	if _, f := RepoByIndex(context.Background(), 0); f == nil {
		t.Error("a dropped index must not resolve")
	}
}
