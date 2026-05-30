package statusline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInput_GitWorktreeVariants guards the lets-ds6bc regression: Claude Code
// sends workspace.git_worktree as a STRING (the worktree path) inside a
// worktree, not a bool. The field must NOT be decoded into a typed struct
// field, or json.Unmarshal fails and blanks the whole statusline. All three
// shapes (string / bool / absent) must decode without error.
func TestInput_GitWorktreeVariants(t *testing.T) {
	cases := []string{
		`{"workspace":{"git_worktree":"/some/path","current_dir":"/x"}}`, // string — the crash case
		`{"workspace":{"git_worktree":true,"current_dir":"/x"}}`,         // bool
		`{"workspace":{"current_dir":"/x"}}`,                             // absent
	}
	for _, payload := range cases {
		var in Input
		if err := json.Unmarshal([]byte(payload), &in); err != nil {
			t.Fatalf("git_worktree variant must not break Unmarshal: %v\n  payload: %s", err, payload)
		}
		if in.Workspace.CurrentDir != "/x" {
			t.Errorf("current_dir not decoded for %s: got %q", payload, in.Workspace.CurrentDir)
		}
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// splitNonEmptyLines splits buffered output on '\n' and drops the trailing
// empty element produced by a final newline. It does NOT drop interior blank
// lines — so a test can assert that none leaked.
func splitNonEmptyLines(out string) []string {
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	// renderRich uses Fprintln, so the last element is "" — trim exactly one.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// richTestInput builds a fully-populated Input that exercises every Full-tier
// segment (diff, worktree pill, PR, task, effort, gauges).
func richTestInput() Input {
	var in Input
	in.Model.DisplayName = "Opus 4.7"
	in.Effort.Level = "high"
	in.ContextWindow.UsedPercentage = 42.4
	in.Cost.TotalLinesAdded = 120
	in.Cost.TotalLinesRemoved = 30
	in.PR.Number = 91
	in.PR.ReviewState = "approved"
	in.RateLimits.FiveHour.UsedPercentage = 58.0
	in.RateLimits.FiveHour.ResetsAt = "2099-01-01T00:00:00Z"
	in.RateLimits.SevenDay.UsedPercentage = 73.0
	in.RateLimits.SevenDay.ResetsAt = "2099-01-02T00:00:00Z"
	return in
}

// writeTaskStatus writes a task-status cache line so the task line is
// deterministic, and returns the cacheDir.
func writeTaskStatus(t *testing.T, line string) string {
	t.Helper()
	dir := t.TempDir()
	if line != "" {
		if err := os.WriteFile(filepath.Join(dir, "task-status"), []byte(line), 0o600); err != nil {
			t.Fatalf("write task-status: %v", err)
		}
	}
	return dir
}

// ----------------------------------------------------------------------------
// (3) EMPTY-INPUT axis — the dominant live render on /reload-plugins + first
//     render. Zero-value Input{} at Full width must not panic, emit blank
//     lines, or leave dangling separators.
// ----------------------------------------------------------------------------

func TestRenderRich_EmptyInputFull(t *testing.T) {
	dir := t.TempDir() // no task-status file
	var buf bytes.Buffer
	if err := renderRich(&buf, Input{}, "", "", usage{}, bpFull, dir, false); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	out := buf.String()
	for _, line := range splitNonEmptyLines(out) {
		if strings.TrimSpace(stripANSI(line)) == "" {
			t.Errorf("blank line emitted: %q (full output %q)", line, out)
		}
		plain := stripANSI(line)
		// Dangling separators — " · " or " » " at a visible start/end.
		if strings.HasPrefix(plain, "·") || strings.HasPrefix(plain, "»") {
			t.Errorf("line starts with dangling separator: %q", plain)
		}
		if strings.HasSuffix(strings.TrimRight(plain, " "), "·") ||
			strings.HasSuffix(strings.TrimRight(plain, " "), "»") {
			t.Errorf("line ends with dangling separator: %q", plain)
		}
		// " · " or " » " followed by end-of-string (no content after sep).
		for _, sep := range []string{" · ", " » "} {
			if strings.HasSuffix(plain, sep) {
				t.Errorf("line ends with separator+space: %q", plain)
			}
			if strings.HasPrefix(plain, sep) {
				t.Errorf("line starts with separator+space: %q", plain)
			}
		}
		if vw := visibleWidth(line); vw > bpFull {
			t.Errorf("line spills width %d > %d: %q", vw, bpFull, plain)
		}
	}
}

func TestRenderRich_EmptyInputAllTiers(t *testing.T) {
	dir := t.TempDir()
	widths := []int{bpFull, bpMid, bpNarrow, 50}
	for _, w := range widths {
		var buf bytes.Buffer
		if err := renderRich(&buf, Input{}, "", "", usage{}, w, dir, false); err != nil {
			t.Fatalf("renderRich width=%d: %v", w, err)
		}
		for _, line := range splitNonEmptyLines(buf.String()) {
			if strings.TrimSpace(stripANSI(line)) == "" {
				t.Errorf("width=%d blank line: %q", w, line)
			}
			if vw := visibleWidth(line); vw > w {
				t.Errorf("width=%d line spills %d: %q", w, vw, stripANSI(line))
			}
		}
	}
}

// ----------------------------------------------------------------------------
// (2) RICH structure tests — line counts + no spill per tier.
// ----------------------------------------------------------------------------

func TestRenderRich_TierLineCounts(t *testing.T) {
	// task-status keyed to the id embedded in the branch name.
	dir := writeTaskStatus(t, "lets-ds6bc|Statusline 2.0 rich renderer|3|2099-01-01T00:00:00Z")
	branch := "feature/lets-ds6bc-statusline-2-0"
	in := richTestInput()

	tests := []struct {
		name      string
		width     int
		wantLines int
	}{
		{"Full", bpFull, 4},     // identity, task, budget, tip
		{"Mid", bpMid, 4},       // identity, task, budget, tip
		{"Narrow", bpNarrow, 2}, // branch+diff, gauges
		{"Min", 50, 1},          // single line
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := renderRich(&buf, in, branch, "folder", usage{}, tt.width, dir, false); err != nil {
				t.Fatalf("renderRich: %v", err)
			}
			lines := splitNonEmptyLines(buf.String())
			if len(lines) != tt.wantLines {
				t.Errorf("%s tier: got %d lines, want %d\noutput:\n%s",
					tt.name, len(lines), tt.wantLines, buf.String())
			}
			for i, line := range lines {
				if vw := visibleWidth(line); vw > tt.width {
					t.Errorf("%s tier line %d spills width %d > %d: %q",
						tt.name, i, vw, tt.width, stripANSI(line))
				}
			}
		})
	}
}

// TestRenderRich_NoTaskDropsLine verifies the task line is dropped entirely
// when the branch carries no task id (Full tier => identity, budget, tip = 3).
func TestRenderRich_NoTaskDropsLine(t *testing.T) {
	dir := t.TempDir()
	in := richTestInput()
	var buf bytes.Buffer
	if err := renderRich(&buf, in, "main", "folder", usage{}, bpFull, dir, false); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	lines := splitNonEmptyLines(buf.String())
	if len(lines) != 3 {
		t.Errorf("no-task Full tier: got %d lines, want 3:\n%s", len(lines), buf.String())
	}
}

// TestRenderRich_LightPaletteNoPanic exercises the light branch.
func TestRenderRich_LightPaletteNoPanic(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := renderRich(&buf, richTestInput(), "feature/lets-aaaaa-x", "f", usage{}, bpFull, dir, true); err != nil {
		t.Fatalf("renderRich light: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("light palette produced no output")
	}
}

// ----------------------------------------------------------------------------
// (4) UNIT PINS
// ----------------------------------------------------------------------------

func TestThreshold(t *testing.T) {
	p := paletteDark
	tests := []struct {
		pct  int
		want string
	}{
		{59, p.ok},
		{60, p.warn},
		{84, p.warn},
		{85, p.alert},
		{0, p.ok},
		{100, p.alert},
	}
	for _, tt := range tests {
		if got := p.threshold(tt.pct); got != tt.want {
			t.Errorf("threshold(%d): got %q, want %q", tt.pct, got, tt.want)
		}
	}
}

func TestMiniBar(t *testing.T) {
	p := paletteDark

	// Clamp: <0 behaves like 0 (zero filled), >100 like 100 (all filled).
	zero := p.miniBar(-10)
	full := p.miniBar(150)
	if strings.Count(stripANSI(zero), barFill) != 0 {
		t.Errorf("miniBar(-10) should clamp to 0 filled: %q", stripANSI(zero))
	}
	if strings.Count(stripANSI(full), barFill) != barWidth {
		t.Errorf("miniBar(150) should clamp to %d filled: %q", barWidth, stripANSI(full))
	}

	// filled-count == round(pct/100*8).
	roundFilled := func(pct int) int { return (pct*barWidth + 50) / 100 }
	for _, pct := range []int{41, 58, 73, 91} {
		bar := p.miniBar(pct)
		gotFilled := strings.Count(stripANSI(bar), barFill)
		gotEmpty := strings.Count(stripANSI(bar), barEmpty)
		wantFilled := roundFilled(pct)
		if gotFilled != wantFilled {
			t.Errorf("miniBar(%d): filled=%d, want %d", pct, gotFilled, wantFilled)
		}
		if gotFilled+gotEmpty != barWidth {
			t.Errorf("miniBar(%d): filled+empty=%d, want %d", pct, gotFilled+gotEmpty, barWidth)
		}
		// Exactly one color token (the threshold accent) precedes the sep token.
		idxThresh := strings.Index(bar, p.threshold(pct))
		idxSep := strings.Index(bar, p.sep)
		if idxThresh < 0 || idxSep < 0 || idxThresh >= idxSep {
			t.Errorf("miniBar(%d): expected threshold color before sep, bar=%q", pct, bar)
		}
	}
}

func TestPrStateColor(t *testing.T) {
	p := paletteDark
	tests := []struct {
		state string
		want  string
	}{
		{"approved", p.ok},
		{"changes_requested", p.alert},
		{"pending", p.warn},
		{"review_required", p.warn},
		{"", p.warn},
		{"totally_unknown_state", p.warn}, // default branch
	}
	for _, tt := range tests {
		if got := p.prStateColor(tt.state); got != tt.want {
			t.Errorf("prStateColor(%q): got %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestClip(t *testing.T) {
	// Within budget: unchanged.
	s := "hello"
	if got := clip(s, 10); got != s {
		t.Errorf("clip within budget changed string: got %q", got)
	}

	// Plain truncation adds "…".
	got := clip("abcdefghij", 5)
	if !strings.HasSuffix(got, "…"+ansiReset) {
		t.Errorf("clip should append ellipsis+reset: got %q", got)
	}
	if vw := visibleWidth(got); vw > 5 {
		t.Errorf("clipped visible width %d exceeds budget 5: %q", vw, got)
	}

	// ANSI-colored string keeps its escape codes when truncated.
	colored := ansiReset + paletteDark.clay + "abcdefghij" + ansiReset
	clipped := clip(colored, 5)
	if !strings.Contains(clipped, paletteDark.clay) {
		t.Errorf("clip dropped color escape: %q", clipped)
	}
	if !strings.HasSuffix(clipped, "…"+ansiReset) {
		t.Errorf("clip colored should append ellipsis+reset: %q", clipped)
	}

	// max <= 1 is a no-op guard.
	if got := clip("abcdef", 1); got != "abcdef" {
		t.Errorf("clip(max=1) should be no-op: got %q", got)
	}
}

func TestLevelForWidth(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{bpNarrow - 1, tierMin},
		{bpNarrow, tierNarrow},
		{bpMid - 1, tierNarrow},
		{bpMid, tierMid},
		{bpFull - 1, tierMid},
		{bpFull, tierFull},
		{500, tierFull},
		{0, tierMin},
	}
	for _, tt := range tests {
		if got := levelForWidth(tt.width); got != tt.want {
			t.Errorf("levelForWidth(%d): got %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestParseISO(t *testing.T) {
	tests := []struct {
		name   string
		iso    string
		wantOK bool
	}{
		{"valid plain", "2099-01-02T03:04:05", true},
		{"blank", "", false},
		{"garbage", "not-a-date", false},
		{"offset", "2099-01-02T03:04:05+02:00", true},
		{"Z suffix", "2099-01-02T03:04:05Z", true},
		{"fractional", "2099-01-02T03:04:05.123Z", true},
	}
	for _, tt := range tests {
		_, ok := parseISO(tt.iso)
		if ok != tt.wantOK {
			t.Errorf("parseISO(%q): ok=%v, want %v", tt.iso, ok, tt.wantOK)
		}
	}

	// offset/Z normalize to the same instant.
	tZ, _ := parseISO("2099-01-02T03:04:05Z")
	tPlain, _ := parseISO("2099-01-02T03:04:05")
	if !tZ.Equal(tPlain) {
		t.Errorf("Z suffix should equal plain: %v vs %v", tZ, tPlain)
	}
}

func TestRelAgo(t *testing.T) {
	// Future → "".
	if got := relAgo("2099-01-01T00:00:00Z"); got != "" {
		t.Errorf("relAgo(future): got %q, want \"\"", got)
	}
	// Blank/invalid → "".
	if got := relAgo(""); got != "" {
		t.Errorf("relAgo(blank): got %q, want \"\"", got)
	}
	if got := relAgo("garbage"); got != "" {
		t.Errorf("relAgo(garbage): got %q, want \"\"", got)
	}
	// <1m → "just now". Use a fixed past instant (year 2000) so the bucket
	// suffix is stable regardless of when the test runs.
	if got := relAgo("2000-01-01T00:00:00Z"); !strings.HasSuffix(got, "d ago") {
		t.Errorf("relAgo(year 2000): expected \"...d ago\", got %q", got)
	}
}

func TestReadTaskStatus(t *testing.T) {
	tests := []struct {
		name        string
		content     string // empty => no file written
		taskID      string
		wantOK      bool
		wantTitle   string
		wantNotes   int
		wantComment string
	}{
		{"missing file", "", "lets-aaaaa", false, "", 0, ""},
		{"wrong task-id", "lets-bbbbb|Other|2|x", "lets-aaaaa", false, "", 0, ""},
		{"short line id only", "lets-aaaaa", "lets-aaaaa", false, "", 0, ""},
		{"id+title only", "lets-aaaaa|My Title", "lets-aaaaa", true, "My Title", 0, ""},
		{"full 4-field", "lets-aaaaa|My Title|5|2099-01-01T00:00:00Z", "lets-aaaaa", true, "My Title", 5, "2099-01-01T00:00:00Z"},
		{"garbage", "this is not pipe delimited", "lets-aaaaa", false, "", 0, ""},
		{"bad note count", "lets-aaaaa|Title|notanint|x", "lets-aaaaa", true, "Title", 0, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				if err := os.WriteFile(filepath.Join(dir, "task-status"), []byte(tt.content), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			title, notes, comment, ok := readTaskStatus(dir, tt.taskID)
			if ok != tt.wantOK {
				t.Errorf("ok=%v, want %v", ok, tt.wantOK)
			}
			if title != tt.wantTitle {
				t.Errorf("title=%q, want %q", title, tt.wantTitle)
			}
			if notes != tt.wantNotes {
				t.Errorf("notes=%d, want %d", notes, tt.wantNotes)
			}
			if comment != tt.wantComment {
				t.Errorf("comment=%q, want %q", comment, tt.wantComment)
			}
		})
	}
}

func TestBrandEmoji(t *testing.T) {
	tests := []struct {
		linesAdded int
		want       string
	}{
		{0, "🌱"},
		{49, "🌱"},
		{50, "🪴"},
		{99, "🪴"},
		{100, "🌿"},
		{249, "🌿"},
		{250, "🌳"},
		{499, "🌳"},
		{500, "🌴"},
		{5000, "🌴"},
		{-10, "🌱"}, // never below stage 0
	}
	for _, tt := range tests {
		if got := brandEmoji(tt.linesAdded); got != tt.want {
			t.Errorf("brandEmoji(%d)=%q, want %q", tt.linesAdded, got, tt.want)
		}
	}
}
