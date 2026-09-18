//go:build unix

package handoffcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frame reads a T0 fixture (testdata/frames, captured live 2026-09-18, scrubbed).
func frame(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "frames", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestInputLine(t *testing.T) {
	for _, tc := range []struct {
		agent, fixture, want string
	}{
		{"codex", "codex-empty", lineReady},
		{"codex", "codex-typed", lineNotClear},
		{"codex", "codex-working", lineBusy},
		{"claude", "claude-empty", lineReady}, // a fresh composer shows `Try "..."`
		{"claude", "claude-idle", lineReady},
		{"claude", "claude-working", lineBusy},
		{"claude", "claude-hooks", lineBusy},
		// Typed Claude text lives in Orca's draft, not on screen: the screen reads
		// ready and gate refuses on the draft (TestSend_DraftRefused).
		{"claude", "claude-typed", lineReady},
		{"antigravity", "codex-empty", lineUnknown},
		{"gemini", "claude-idle", lineUnknown},
	} {
		if got := inputLine(tc.agent, frame(t, tc.fixture)); got != tc.want {
			t.Errorf("%s / %s: %s, want %s", tc.agent, tc.fixture, got, tc.want)
		}
	}
	dialog := append(frame(t, "claude-idle"), "  Do you want to proceed?", "  ❯ 1. Yes")
	if got := inputLine("claude", dialog); got != lineBusy {
		t.Errorf("claude dialog: %s", got)
	}
	ag := []string{"> /plan Read the hand-off brief at /w/.lets/handoffs/b.md and follow it exactly.", "  plan · Gemini 3.8 Flash · high"}
	if got := inputLine("antigravity", ag); got != lineUnknown {
		t.Errorf("antigravity mode echo: %s", got)
	}
	// History above the composer repeats the glyph; the lowest prompt line wins.
	lines := frame(t, "codex-empty")
	history := append([]string{"› Read the hand-off brief at /w/.lets/handoffs/b.md and follow it exactly.", "• Reviewed."}, lines...)
	if got := inputLine("codex", history); got != lineReady {
		t.Errorf("codex history: %s", got)
	}
}
