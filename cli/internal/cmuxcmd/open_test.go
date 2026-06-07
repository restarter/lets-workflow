//go:build unix

package cmuxcmd

import (
	"context"
	"testing"
)

func TestOpen_NonMacOS_Fallback(t *testing.T) {
	defer func(g string) { runtimeGOOS = g }(runtimeGOOS)
	runtimeGOOS = "linux"
	res, err := Open(context.Background(), OpenOptions{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !res.OK || res.Launch == nil || res.Launch.Launched || res.Launch.Reason != "not_macos" {
		t.Fatalf("expected OK fallback not_macos, got %+v", res.Launch)
	}
	if res.Launch.FallbackCommand == "" {
		t.Fatal("expected a fallback command")
	}
}

func TestOpen_CmuxNotFound_Fallback(t *testing.T) {
	defer func(g string) { runtimeGOOS = g }(runtimeGOOS)
	defer func(f func() (string, bool)) { lookCmux = f }(lookCmux)
	runtimeGOOS = "darwin"
	lookCmux = func() (string, bool) { return "", false }
	res, err := Open(context.Background(), OpenOptions{Path: t.TempDir()})
	if err != nil || !res.OK || res.Launch == nil || res.Launch.Reason != "cmux_not_found" {
		t.Fatalf("expected OK fallback cmux_not_found, got res=%+v err=%v", res.Launch, err)
	}
}

func TestOpen_PathMissing_HardError(t *testing.T) {
	res, err := Open(context.Background(), OpenOptions{Path: ""})
	if err == nil || res.OK || ExitCode(err) != ExitPathInvalid {
		t.Fatalf("expected ExitPathInvalid hard error, got res=%+v err=%v", res, err)
	}
}

func TestOpen_PathInvalid_HardError(t *testing.T) {
	res, err := Open(context.Background(), OpenOptions{Path: "/no/such/dir/xyz-cmuxcmd"})
	if err == nil || res.OK || ExitCode(err) != ExitPathInvalid {
		t.Fatalf("expected ExitPathInvalid hard error, got res=%+v err=%v", res, err)
	}
}

func TestOpen_PassesDescription(t *testing.T) {
	defer func(g string) { runtimeGOOS = g }(runtimeGOOS)
	defer func(f func() (string, bool)) { lookCmux = f }(lookCmux)
	defer func(f func(context.Context, string) ([]byte, error)) { listWorkspacesRaw = f }(listWorkspacesRaw)
	defer func(f func(context.Context, string, []string) ([]byte, error)) { createWorkspaceRaw = f }(createWorkspaceRaw)

	runtimeGOOS = "darwin"
	lookCmux = func() (string, bool) { return "/usr/bin/cmux", true }
	listWorkspacesRaw = func(context.Context, string) ([]byte, error) { return []byte(`{"workspaces":[]}`), nil }
	var gotArgs []string
	createWorkspaceRaw = func(_ context.Context, _ string, args []string) ([]byte, error) {
		gotArgs = args
		return nil, nil
	}

	res, err := Open(context.Background(), OpenOptions{
		Path: t.TempDir(), Name: "slug", Description: "lets-x · Title", Command: "claude",
	})
	if err != nil || !res.OK || res.Launch == nil || !res.Launch.Launched {
		t.Fatalf("expected launched, got res=%+v err=%v", res, err)
	}
	if !argsHavePair(gotArgs, "--description", "lets-x · Title") {
		t.Fatalf("expected --description pair in args, got %v", gotArgs)
	}
	if res.Launch.Description != "lets-x · Title" {
		t.Fatalf("expected description echoed in launch, got %q", res.Launch.Description)
	}
}

// argsHavePair reports whether args contains flag immediately followed by val.
func argsHavePair(args []string, flag, val string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}

func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion changed to %d - bump intentionally + update consumers", SchemaVersion)
	}
}
