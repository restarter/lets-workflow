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

func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion changed to %d - bump intentionally + update consumers", SchemaVersion)
	}
}
