//go:build unix

package cmuxcmd

import (
	"context"
	"errors"
	"testing"
)

func TestNotify_TitleMissing_HardError(t *testing.T) {
	res, err := Notify(context.Background(), NotifyOptions{Title: ""})
	if err == nil || res.OK || ExitCode(err) != ExitUsage {
		t.Fatalf("want ExitUsage hard error, got res=%+v err=%v", res, err)
	}
}

func TestNotify_NonMacOS(t *testing.T) {
	defer func(g string) { runtimeGOOS = g }(runtimeGOOS)
	runtimeGOOS = "linux"
	res, err := Notify(context.Background(), NotifyOptions{Title: "x"})
	if err != nil || !res.OK || res.Notify.Notified || res.Notify.Reason != "not_macos" {
		t.Fatalf("want not_macos fallback, got res=%+v err=%v", res.Notify, err)
	}
}

func TestNotify_CmuxNotFound(t *testing.T) {
	defer func(g string, l func() (string, bool)) { runtimeGOOS = g; lookCmux = l }(runtimeGOOS, lookCmux)
	runtimeGOOS = "darwin"
	lookCmux = func() (string, bool) { return "", false }
	res, err := Notify(context.Background(), NotifyOptions{Title: "x"})
	if err != nil || !res.OK || res.Notify.Reason != "cmux_not_found" {
		t.Fatalf("want cmux_not_found, got res=%+v err=%v", res.Notify, err)
	}
}

func TestNotify_WorkspaceNotFound(t *testing.T) {
	setMacOSCmux(t)
	stubList(t) // empty list
	res, _ := Notify(context.Background(), NotifyOptions{Title: "x"})
	if res.Notify.Notified || res.Notify.Reason != "workspace_not_found" {
		t.Fatalf("want workspace_not_found, got %+v", res.Notify)
	}
}

func TestNotify_ResolveByCwd(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	stubList(t,
		workspaceEntry{Ref: "workspace:2", Title: "a", CurrentDirectory: dir},
		workspaceEntry{Ref: "workspace:3", Title: "b", Selected: true},
	)
	var gotRef, gotTitle, gotBody string
	old := notifyRaw
	notifyRaw = func(_ context.Context, _, ref, title, _, body string) ([]byte, error) {
		gotRef, gotTitle, gotBody = ref, title, body
		return []byte("OK"), nil
	}
	t.Cleanup(func() { notifyRaw = old })

	res, err := Notify(context.Background(), NotifyOptions{Title: "Plan ready", Body: "lets-x", Cwd: dir})
	if err != nil || !res.Notify.Notified || gotRef != "workspace:2" {
		t.Fatalf("cwd resolution failed: ref=%q res=%+v err=%v", gotRef, res.Notify, err)
	}
	if gotTitle != "Plan ready" || gotBody != "lets-x" {
		t.Fatalf("title/body not forwarded: title=%q body=%q", gotTitle, gotBody)
	}
}

func TestNotify_ExplicitRef_SkipsResolution(t *testing.T) {
	setMacOSCmux(t)
	old := listWorkspacesRaw
	listWorkspacesRaw = func(context.Context, string) ([]byte, error) {
		t.Fatal("list should not be called when --ref is set")
		return nil, nil
	}
	t.Cleanup(func() { listWorkspacesRaw = old })
	oldN := notifyRaw
	notifyRaw = func(_ context.Context, _, ref, _, _, _ string) ([]byte, error) { return []byte("OK " + ref), nil }
	t.Cleanup(func() { notifyRaw = oldN })

	res, err := Notify(context.Background(), NotifyOptions{Title: "x", Ref: "workspace:9"})
	if err != nil || !res.Notify.Notified || res.Notify.Ref != "workspace:9" {
		t.Fatalf("explicit ref failed: res=%+v err=%v", res.Notify, err)
	}
}

func TestNotify_CmuxError(t *testing.T) {
	setMacOSCmux(t)
	stubList(t, workspaceEntry{Ref: "workspace:1", Selected: true})
	oldN := notifyRaw
	notifyRaw = func(_ context.Context, _, _, _, _, _ string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	}
	t.Cleanup(func() { notifyRaw = oldN })

	res, err := Notify(context.Background(), NotifyOptions{Title: "x"})
	if err != nil || !res.OK || res.Notify.Notified || res.Notify.Reason != "cmux_error" {
		t.Fatalf("want cmux_error degrade, got res=%+v err=%v", res.Notify, err)
	}
}
