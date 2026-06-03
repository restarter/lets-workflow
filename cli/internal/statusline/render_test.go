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
	//
	// Dev builds render as "dev" (no v prefix); tagged builds as "vX.Y.Z".
	var buf bytes.Buffer
	_ = renderLines(&buf, Input{}, "br", "", usage{})
	out := buf.String()
	want := version.Version
	if !version.IsDev() {
		want = "v" + want
	}
	if !strings.Contains(out, want) {
		t.Errorf("expected %q in output: %q", want, out)
	}
}

// TestRenderLines_ByteIdentityGolden locks the EXACT bytes of the frozen
// 2-line compact output. This is the byte-lock that the Contains-based tests
// above cannot enforce: any reordering, recoloring, or whitespace change in
// renderLines fails here. The version segment is the only environment-variant
// piece (dev vs tagged), so it is composed from version.Version/IsDev() rather
// than hard-coded — everything else is a literal byte sequence.
func TestRenderLines_ByteIdentityGolden(t *testing.T) {
	var in Input
	in.Model.DisplayName = "Opus 4.7"
	in.ContextWindow.UsedPercentage = 35.5
	in.ContextWindow.ContextWindowSize = 200000
	in.ContextWindow.CurrentUsage.InputTokens = 70000
	u := usage{fiveHour: 85, fiveHourOK: true, sevenDay: 30, sevenDayOK: true}

	var buf bytes.Buffer
	if err := renderLines(&buf, in, "feature/test", "fallback-folder", u); err != nil {
		t.Fatalf("renderLines: %v", err)
	}

	verDisplay := version.Version
	if !version.IsDev() {
		verDisplay = "v" + verDisplay
	}

	// Golden captured from current behavior. Pieces correspond 1:1 to the
	// Fprintf sequence in renderLines (render.go). \x1b == ESC.
	const (
		esc      = "\x1b"
		reset    = esc + "[0m"
		boldGold = esc + "[1;38;2;255;215;0m"
		sepGold  = esc + "[38;2;153;122;0m"
		branch   = esc + "[38;2;232;160;144m"
		boldOrng = esc + "[1;38;2;255;175;50m"
		tan      = esc + "[38;2;190;176;140m"
		tanDim   = esc + "[2;38;2;190;176;140m"
		gray     = esc + "[90m"
		green    = esc + "[38;2;130;200;130m"
		red      = esc + "[38;2;255;100;100m"
		leaf     = "\xf0\x9f\x8c\xb1" // 🌱
	)
	sep := sepGold + " \xc2\xbb " + reset // " » "
	dot := gray + " \xc2\xb7 " + reset    // " · "

	want := leaf + " " + boldGold + "LETS Workflow " + verDisplay + reset + sep +
		branch + "feature/test" + reset + "\n" +
		boldOrng + "Opus 4.7" + reset + sep +
		tan + "window 36%" + reset + " " + tanDim + "(70k/200k)" + reset + dot +
		red + "5h 85%" + reset + dot +
		green + "7d 30%" + reset

	if got := buf.String(); got != want {
		t.Errorf("byte-identity mismatch.\n got: %q\nwant: %q", got, want)
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

// TestRender_Dispatch guards the rich-vs-compact seam in Render: the DEFAULT
// (compact=false) emits the boxed rich output; --compact (compact=true) emits
// the legacy 2-line output (no box).
func TestRender_Dispatch(t *testing.T) {
	const payload = `{"workspace":{"current_dir":"/tmp"},"model":{"display_name":"Opus"},"context_window":{"used_percentage":10,"context_window_size":1000000}}`

	var compact bytes.Buffer
	if err := Render(strings.NewReader(payload), &compact, false, true, true, true); err != nil { // compact=true
		t.Fatalf("compact Render: %v", err)
	}
	if strings.Contains(compact.String(), "┌") {
		t.Errorf("--compact output should not be boxed:\n%s", compact.String())
	}

	var rich bytes.Buffer
	if err := Render(strings.NewReader(payload), &rich, false, false, true, true); err != nil { // default = rich
		t.Fatalf("rich Render: %v", err)
	}
	if !strings.Contains(rich.String(), "┌") {
		t.Errorf("default output should be boxed (contain ┌):\n%s", rich.String())
	}
}

// TestRender_NoTip: showTip=false drops the bottom tip row from the box.
// (Asserts on line count, not glyph: the tip glyph is plain text and could also
// appear inside a tip's wording.)
func TestRender_NoTip(t *testing.T) {
	const payload = `{"workspace":{"current_dir":"/tmp"},"model":{"display_name":"Opus"},"context_window":{"used_percentage":10,"context_window_size":1000000}}`
	var on, off bytes.Buffer
	if err := Render(strings.NewReader(payload), &on, false, false, true, true); err != nil {
		t.Fatalf("tip-on: %v", err)
	}
	if err := Render(strings.NewReader(payload), &off, false, false, false, true); err != nil {
		t.Fatalf("tip-off: %v", err)
	}
	onLines := len(splitNonEmptyLines(on.String()))
	offLines := len(splitNonEmptyLines(off.String()))
	if onLines <= offLines {
		t.Errorf("showTip=true should render more rows than showTip=false: on=%d off=%d\non:\n%s\noff:\n%s",
			onLines, offLines, on.String(), off.String())
	}
}
