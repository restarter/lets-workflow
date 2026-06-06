//go:build unix

package cmuxcmd

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// B4: the create arg vector (the core behavior + injection-relevant surface) is asserted.
func TestOpen_CreateArgs(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	stubList(t) // empty -> guard passes -> reach create
	var got []string
	old := createWorkspaceRaw
	createWorkspaceRaw = func(_ context.Context, _ string, a []string) ([]byte, error) { got = a; return []byte("OK"), nil }
	t.Cleanup(func() { createWorkspaceRaw = old })

	res, err := Open(context.Background(), OpenOptions{Path: dir, Name: "my-slug", Command: "claude '/lets:start lets-x'"})
	if err != nil || !res.Launch.Launched {
		t.Fatalf("want launched, got %+v err=%v", res.Launch, err)
	}
	want := []string{"workspace", "create", "--cwd", dir, "--name", "my-slug", "--command", "claude '/lets:start lets-x'"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("create args:\n got %v\nwant %v", got, want)
	}

	// Empty Name/Command -> those flags must be absent (no --focus ever).
	stubList(t)
	if _, err := Open(context.Background(), OpenOptions{Path: dir}); err != nil {
		t.Fatal(err)
	}
	want2 := []string{"workspace", "create", "--cwd", dir}
	if !reflect.DeepEqual(got, want2) {
		t.Fatalf("create args (no name/command):\n got %v\nwant %v", got, want2)
	}
}

// B5: the post-exec cmux_error fallback (the never-hard-fail invariant) is exercised.
func TestOpen_CmuxError_Fallback(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	stubList(t)
	old := createWorkspaceRaw
	createWorkspaceRaw = func(context.Context, string, []string) ([]byte, error) { return []byte("boom"), errors.New("exit 1") }
	t.Cleanup(func() { createWorkspaceRaw = old })

	res, err := Open(context.Background(), OpenOptions{Path: dir})
	if err != nil || !res.OK || res.Launch.Launched || res.Launch.Reason != "cmux_error" || res.Launch.FallbackCommand == "" {
		t.Fatalf("want cmux_error fallback (no hard error), got %+v err=%v", res.Launch, err)
	}
}

func TestRename_CmuxError_Fallback(t *testing.T) {
	setMacOSCmux(t)
	stubList(t, workspaceEntry{Ref: "workspace:1", Title: "x", Selected: true})
	old := renameWorkspaceRaw
	renameWorkspaceRaw = func(context.Context, string, string, string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	}
	t.Cleanup(func() { renameWorkspaceRaw = old })

	res, err := Rename(context.Background(), RenameOptions{Title: "new"})
	if err != nil || !res.OK || res.Rename.Renamed || res.Rename.Reason != "cmux_error" {
		t.Fatalf("want cmux_error fallback (no hard error), got %+v err=%v", res.Rename, err)
	}
}

// S3: a listWorkspaces failure during the guard is non-fatal - fall through and create.
func TestOpen_ListFails_FallsThroughToCreate(t *testing.T) {
	setMacOSCmux(t)
	dir := t.TempDir()
	oldL := listWorkspacesRaw
	listWorkspacesRaw = func(context.Context, string) ([]byte, error) { return nil, errors.New("list boom") }
	t.Cleanup(func() { listWorkspacesRaw = oldL })
	var created bool
	oldC := createWorkspaceRaw
	createWorkspaceRaw = func(context.Context, string, []string) ([]byte, error) { created = true; return []byte("OK"), nil }
	t.Cleanup(func() { createWorkspaceRaw = oldC })

	res, err := Open(context.Background(), OpenOptions{Path: dir})
	if err != nil || !res.Launch.Launched || !created {
		t.Fatalf("list failure should fall through to create, got %+v err=%v created=%v", res.Launch, err, created)
	}
}

// S4: shellQuote produces a runnable single-quoted segment for awkward paths.
func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/tmp/wt":    "'/tmp/wt'",
		"/tmp/a b":   "'/tmp/a b'",
		"/x/o'brien": `'/x/o'\''brien'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// B2: the guard matches a symlinked path against cmux's resolved current_directory.
// On macOS t.TempDir() is itself symlinked (/var -> /private/var), so passing the
// unresolved path while cmux reports the resolved one exercises EvalSymlinks.
func TestOpen_AlreadyOpen_Guard_Symlink(t *testing.T) {
	setMacOSCmux(t)
	base := t.TempDir()
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	// cmux reports the RESOLVED path; we open via the (possibly symlinked) base.
	stubList(t, workspaceEntry{Ref: "workspace:7", Title: "existing", CurrentDirectory: realBase})

	res, oerr := Open(context.Background(), OpenOptions{Path: base})
	if oerr != nil || res.Launch.Launched || res.Launch.Reason != "already_open" {
		t.Fatalf("symlinked path must match resolved current_directory (got %+v err=%v); base=%q realBase=%q", res.Launch, oerr, base, realBase)
	}
}
