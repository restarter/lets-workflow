package statusline

import "testing"

// TestBackgroundSpawnsDisabledUnderTest pins the contract that the detached
// self-exec usage/task refreshers are suppressed while running under `go test`.
//
// If this regresses, the render tests re-exec the test binary (os.Executable()),
// which re-runs the whole suite and spawns more detached copies — an exponential
// fork bomb that hangs `go test -race ./...` on Linux CI (SIGTERM / exit 143).
// Assert the guard signal directly so a regression fails cleanly here instead.
func TestBackgroundSpawnsDisabledUnderTest(t *testing.T) {
	if !backgroundSpawnsDisabled() {
		t.Fatal("background self-exec spawns must be disabled under `go test` " +
			"(else statusline render tests fork-bomb the test binary — see lets-ijpw4)")
	}
}
