package statusline

import (
	"bytes"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

func TestRenderLines_HasBranchAndModel(t *testing.T) {
	var in Input
	in.Model.DisplayName = "Opus 4.7"
	in.ContextWindow.UsedPercentage = 35.5
	in.ContextWindow.ContextWindowSize = 200000
	in.ContextWindow.CurrentUsage.InputTokens = 70000

	var buf bytes.Buffer
	if err := renderLines(&buf, in, "feature/test", "fallback-folder", usage{}); err != nil {
		t.Fatalf("renderLines: %v", err)
	}
	out := buf.String()
	checks := []string{"LETS Workflow", "feature/test", "Opus 4.7", "window 36%", "(70k/200k)"}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %q", want, out)
		}
	}
	if strings.Contains(out, "fallback-folder") {
		t.Errorf("folder leaked when branch present: %q", out)
	}
}

func TestRenderLines_FallbackToFolder(t *testing.T) {
	var buf bytes.Buffer
	if err := renderLines(&buf, Input{}, "", "myproject", usage{}); err != nil {
		t.Fatalf("renderLines: %v", err)
	}
	if !strings.Contains(buf.String(), "myproject") {
		t.Errorf("missing folder fallback: %q", buf.String())
	}
}

func TestRenderLines_UsageStatsColored(t *testing.T) {
	var buf bytes.Buffer
	u := usage{fiveHour: 85, fiveHourOK: true, sevenDay: 30, sevenDayOK: true}
	if err := renderLines(&buf, Input{}, "br", "", u); err != nil {
		t.Fatalf("renderLines: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "5h 85%") || !strings.Contains(out, "7d 30%") {
		t.Errorf("missing usage values: %q", out)
	}
	if !strings.Contains(out, ansiRed) {
		t.Errorf("expected red color for 85%%, got %q", out)
	}
	if !strings.Contains(out, ansiGreen) {
		t.Errorf("expected green color for 30%%, got %q", out)
	}
}

func TestRenderLines_NoContextNoUsage(t *testing.T) {
	var buf bytes.Buffer
	if err := renderLines(&buf, Input{}, "br", "", usage{}); err != nil {
		t.Fatalf("renderLines: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "window") {
		t.Errorf("window block should not render with 0%%: %q", out)
	}
	if strings.Contains(out, "5h ") || strings.Contains(out, "7d ") {
		t.Errorf("usage stats should not render without OK flags: %q", out)
	}
}

func TestRenderLines_VersionInHeader(t *testing.T) {
	// N12 (review 2026-05-08): old assertion `Contains(out, "v")` was a
	// tautology - any output containing letter v passed. Bind to the actual
	// version string so a regression that drops the version entirely fails.
	var buf bytes.Buffer
	_ = renderLines(&buf, Input{}, "br", "", usage{})
	out := buf.String()
	want := "v" + version.Version
	if !strings.Contains(out, want) {
		t.Errorf("expected %q in output: %q", want, out)
	}
}

func TestUsageColor(t *testing.T) {
	tests := []struct {
		pct  int
		want string
	}{
		{0, ansiGreen},
		{49, ansiGreen},
		{50, ansiYellow},
		{79, ansiYellow},
		{80, ansiRed},
		{100, ansiRed},
	}
	for _, tt := range tests {
		if got := usageColor(tt.pct); got != tt.want {
			t.Errorf("usageColor(%d): got %q, want %q", tt.pct, got, tt.want)
		}
	}
}
