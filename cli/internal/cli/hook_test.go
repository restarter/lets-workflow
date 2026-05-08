package cli

// Internal test for the hook size-guard helper. Stays in `package cli`
// (not cli_test) so it can call renderHookOutput directly without going
// through the cobra wrapper - we want to test the size-guard logic in
// isolation from sessionstart.Run output.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTestCmd produces a minimal cobra.Command with redirectable stdout/stderr.
func newTestCmd(out, errOut *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd
}

func TestRenderHookOutput_Normal_NoWarning(t *testing.T) {
	body := []byte("## LETS Config\n\nLETS_PROJECT_ROOT=/foo\n")

	var stdout, stderr bytes.Buffer
	cmd := newTestCmd(&stdout, &stderr)

	if err := renderHookOutput(cmd, body); err != nil {
		t.Fatalf("renderHookOutput: %v", err)
	}

	if got := stdout.String(); got != string(body) {
		t.Errorf("stdout = %q, want %q", got, string(body))
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr, got %q", stderr.String())
	}
}

func TestRenderHookOutput_Warn_FullBodyToStdoutPlusStderrWarning(t *testing.T) {
	// Body is ≥ hookSizeWarn but < hookSizeError → full body emitted, warning to stderr.
	body := bytes.Repeat([]byte("x"), hookSizeWarn+10)

	var stdout, stderr bytes.Buffer
	cmd := newTestCmd(&stdout, &stderr)

	if err := renderHookOutput(cmd, body); err != nil {
		t.Fatalf("renderHookOutput: %v", err)
	}

	if !bytes.Equal(stdout.Bytes(), body) {
		t.Errorf("stdout was not the full body (len=%d, want %d)", stdout.Len(), len(body))
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("expected stderr warning, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "approaching") {
		t.Errorf("expected stderr to mention 'approaching', got %q", stderr.String())
	}
}

func TestRenderHookOutput_Oversize_TruncatesWithMarker_AndStderrError(t *testing.T) {
	// Body is ≥ hookSizeError → truncate with inline marker, error to stderr.
	body := bytes.Repeat([]byte("x"), hookSizeError+500)

	var stdout, stderr bytes.Buffer
	cmd := newTestCmd(&stdout, &stderr)

	if err := renderHookOutput(cmd, body); err != nil {
		t.Fatalf("renderHookOutput: %v", err)
	}

	out := stdout.String()
	if len(out) > hookSizeError {
		t.Errorf("stdout len %d exceeds cap %d (defeats the size guard)", len(out), hookSizeError)
	}
	if !strings.HasSuffix(out, truncationMarker) {
		// Show last 100 bytes for diagnostic; built-in max guards underflow.
		tailStart := max(0, len(out)-100)
		t.Errorf("stdout must end with truncation marker, got tail %q", out[tailStart:])
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("expected stderr error, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "exceeds") {
		t.Errorf("expected stderr to mention 'exceeds', got %q", stderr.String())
	}
}
