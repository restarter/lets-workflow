//go:build unix

package cmuxcmd

import (
	"context"
	"encoding/json"
	"testing"
)

// setMacOSCmux makes the package believe it's on macOS with cmux on PATH.
func setMacOSCmux(t *testing.T) {
	t.Helper()
	oldGOOS, oldLook := runtimeGOOS, lookCmux
	runtimeGOOS = "darwin"
	lookCmux = func() (string, bool) { return "/fake/cmux", true }
	t.Cleanup(func() { runtimeGOOS = oldGOOS; lookCmux = oldLook })
}

// stubList replaces the cmux `workspace list --json` output.
func stubList(t *testing.T, entries ...workspaceEntry) {
	t.Helper()
	old := listWorkspacesRaw
	listWorkspacesRaw = func(context.Context, string) ([]byte, error) {
		return json.Marshal(workspaceList{Workspaces: entries})
	}
	t.Cleanup(func() { listWorkspacesRaw = old })
}

func TestRename_TitleMissing_HardError(t *testing.T) {
	res, err := Rename(context.Background(), RenameOptions{Title: ""})
	if err == nil || res.OK || ExitCode(err) != ExitUsage {
		t.Fatalf("want ExitUsage hard error, got res=%+v err=%v", res, err)
	}
}

func TestRename_NonMacOS(t *testing.T) {
	defer func(g string) { runtimeGOOS = g }(runtimeGOOS)
	runtimeGOOS = "linux"
	res, err := Rename(context.Background(), RenameOptions{Title: "x"})
	if err != nil || !res.OK || res.Rename.Reason != "not_macos" {
		t.Fatalf("want not_macos fallback, got res=%+v err=%v", res.Rename, err)
	}
}

func TestRename_ResolveSelected(t *testing.T) {
	setMacOSCmux(t)
	stubList(t,
		workspaceEntry{Ref: "workspace:1", Title: "old", Selected: false},
		workspaceEntry{Ref: "workspace:6", Title: "active", Selected: true},
	)
	var gotRef, gotTitle string
	old := renameWorkspaceRaw
	renameWorkspaceRaw = func(_ context.Context, _, ref, title string) ([]byte, error) {
		gotRef, gotTitle = ref, title
		return []byte("OK " + ref), nil
	}
	t.Cleanup(func() { renameWorkspaceRaw = old })

	res, err := Rename(context.Background(), RenameOptions{Title: "new-name"})
	if err != nil || !res.OK || !res.Rename.Renamed {
		t.Fatalf("want renamed, got res=%+v err=%v", res.Rename, err)
	}
	if gotRef != "workspace:6" || gotTitle != "new-name" {
		t.Fatalf("resolved wrong workspace: ref=%q title=%q", gotRef, gotTitle)
	}
	if res.Rename.OldTitle != "active" {
		t.Fatalf("want OldTitle=active, got %q", res.Rename.OldTitle)
	}
}

func TestRename_ResolveByCwd(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	stubList(t,
		workspaceEntry{Ref: "workspace:2", Title: "a", CurrentDirectory: dir},
		workspaceEntry{Ref: "workspace:3", Title: "b", Selected: true},
	)
	var gotRef string
	old := renameWorkspaceRaw
	renameWorkspaceRaw = func(_ context.Context, _, ref, _ string) ([]byte, error) { gotRef = ref; return nil, nil }
	t.Cleanup(func() { renameWorkspaceRaw = old })

	res, _ := Rename(context.Background(), RenameOptions{Title: "x", Cwd: dir})
	if !res.Rename.Renamed || gotRef != "workspace:2" {
		t.Fatalf("cwd resolution failed: ref=%q res=%+v", gotRef, res.Rename)
	}
}

func TestRename_WorkspaceNotFound(t *testing.T) {
	setMacOSCmux(t)
	stubList(t) // empty list
	res, _ := Rename(context.Background(), RenameOptions{Title: "x"})
	if res.Rename.Renamed || res.Rename.Reason != "workspace_not_found" {
		t.Fatalf("want workspace_not_found, got %+v", res.Rename)
	}
}

func TestRename_ExplicitRef_SkipsResolution(t *testing.T) {
	setMacOSCmux(t)
	old := listWorkspacesRaw
	listWorkspacesRaw = func(context.Context, string) ([]byte, error) {
		t.Fatal("list should not be called when --ref is set")
		return nil, nil
	}
	t.Cleanup(func() { listWorkspacesRaw = old })
	oldR := renameWorkspaceRaw
	renameWorkspaceRaw = func(_ context.Context, _, ref, _ string) ([]byte, error) { return []byte("OK " + ref), nil }
	t.Cleanup(func() { renameWorkspaceRaw = oldR })

	res, err := Rename(context.Background(), RenameOptions{Title: "x", Ref: "workspace:9"})
	if err != nil || !res.Rename.Renamed || res.Rename.Ref != "workspace:9" {
		t.Fatalf("explicit ref failed: res=%+v err=%v", res.Rename, err)
	}
}

// --- open duplicate-session guard ---

func TestOpen_AlreadyOpen_Guard(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	stubList(t, workspaceEntry{Ref: "workspace:4", Title: "existing", CurrentDirectory: dir})

	res, err := Open(context.Background(), OpenOptions{Path: dir})
	if err != nil || !res.OK || res.Launch.Launched || res.Launch.Reason != "already_open" {
		t.Fatalf("want already_open guard, got res=%+v err=%v", res.Launch, err)
	}
	if res.Launch.ExistingRef != "workspace:4" {
		t.Fatalf("want ExistingRef workspace:4, got %q", res.Launch.ExistingRef)
	}
}

func TestOpen_Force_BypassesGuard(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	stubList(t, workspaceEntry{Ref: "workspace:4", Title: "existing", CurrentDirectory: dir})
	var created bool
	oldC := createWorkspaceRaw
	createWorkspaceRaw = func(context.Context, string, []string) ([]byte, error) { created = true; return []byte("OK"), nil }
	t.Cleanup(func() { createWorkspaceRaw = oldC })

	res, err := Open(context.Background(), OpenOptions{Path: dir, Force: true})
	if err != nil || !res.OK || !res.Launch.Launched {
		t.Fatalf("force should launch, got res=%+v err=%v", res.Launch, err)
	}
	if !created {
		t.Fatal("force should reach createWorkspaceRaw")
	}
}
