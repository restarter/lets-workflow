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
	if err := renderRich(&buf, Input{}, "", "", usage{}, bpWide, dir, false, true, true, true); err != nil {
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
		if vw := visibleWidth(line); vw > bpWide {
			t.Errorf("line spills width %d > %d: %q", vw, bpWide, plain)
		}
	}
}

func TestRenderRich_EmptyInputAllTiers(t *testing.T) {
	dir := t.TempDir()
	widths := []int{bpWide, 95, 70, 45}
	for _, w := range widths {
		var buf bytes.Buffer
		if err := renderRich(&buf, Input{}, "", "", usage{}, w, dir, false, true, true, true); err != nil {
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
		{"Full-wide", 120, 7},   // ┌top, identity, budget, ├divider, task, tip, └bottom
		{"Full-min", bpWide, 7}, // Full at the breakpoint (box hugs the narrow width)
		{"Compact-68", 68, 7},   // ┌top, identity, gauges, ├divider, task, tip, └bottom
		{"Compact-45", 45, 7},   // narrowest sampled Compact
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := renderRich(&buf, in, branch, "folder", usage{}, tt.width, dir, false, true, true, true); err != nil {
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

// TestRenderRich_NoTaskDropsLine verifies ONLY the task line is dropped when the
// branch carries no task id — the frame (divider) stays for a consistent box.
func TestRenderRich_NoTaskDropsLine(t *testing.T) {
	dir := t.TempDir()
	in := richTestInput()
	var buf bytes.Buffer
	if err := renderRich(&buf, in, "main", "folder", usage{}, bpWide, dir, false, true, true, true); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	lines := splitNonEmptyLines(buf.String())
	if len(lines) != 6 {
		// ┌top, identity, budget, ├divider, tip, └bottom — task line dropped, divider kept.
		t.Errorf("no-task Full tier: got %d lines, want 6:\n%s", len(lines), buf.String())
	}
}

// TestRenderRich_LightPaletteNoPanic exercises the light branch.
func TestRenderRich_LightPaletteNoPanic(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := renderRich(&buf, richTestInput(), "feature/lets-aaaaa-x", "f", usage{}, bpWide, dir, true, true, true, true); err != nil {
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
		{49, p.ok},
		{50, p.warn},
		{74, p.warn},
		{75, p.alert},
		{0, p.ok},
		{100, p.alert},
	}
	for _, tt := range tests {
		if got := p.threshold(tt.pct); got != tt.want {
			t.Errorf("threshold(%d): got %q, want %q", tt.pct, got, tt.want)
		}
	}
}

func TestKfmt(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0k"},
		{500, "1k"},
		{499, "0k"},
		{380000, "380k"},
		{1000000, "1000k"},
		{375400, "375k"},
	}
	for _, tt := range tests {
		if got := kfmt(tt.n); got != tt.want {
			t.Errorf("kfmt(%d)=%q, want %q", tt.n, got, tt.want)
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

func TestLevelForWidth(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{bpWide - 1, tierCompact},
		{bpWide, tierFull},
		{500, tierFull},
		{0, tierCompact},
		{70, tierCompact},
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
	// Days-scale past → compact "<N>d<H>h ago" via fmtDur. Use a fixed past
	// instant (year 2000) so the day bucket is stable regardless of run time.
	if got := relAgo("2000-01-01T00:00:00Z"); !strings.HasSuffix(got, "h ago") || !strings.Contains(got, "d") {
		t.Errorf("relAgo(year 2000): expected \"...d..h ago\", got %q", got)
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
	// Growth ladder parked — brandEmoji is a static text mark for now.
	for _, n := range []int{-10, 0, 49, 50, 100, 250, 500, 5000} {
		if got := brandEmoji(n); got != glyphPlant {
			t.Errorf("brandEmoji(%d)=%q, want %q", n, got, glyphPlant)
		}
	}
}

func TestEffortColor(t *testing.T) {
	p := paletteDark
	// Each level must yield a distinct color; unknown falls back to dim.
	levels := []string{"low", "medium", "high", "xhigh", "max"}
	seen := map[string]string{}
	for _, l := range levels {
		c := p.effortColor(l)
		if c == "" {
			t.Errorf("effortColor(%q) empty", l)
		}
		for prev, pc := range seen {
			if pc == c {
				t.Errorf("effortColor(%q) collides with %q: %q", l, prev, c)
			}
		}
		seen[l] = c
	}
	if p.effortColor("bogus") != p.dim {
		t.Errorf("unknown effort should be dim")
	}
}

func TestCellWidth(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"abc", 3},
		{"", 0},
		{"⌂", 1},                // folder mark — text, 1 cell
		{"⌂ 7", 3},              // glyph + space + digit
		{"⎇ x", 3},              // monochrome symbol is 1 cell
		{"\033[1mhi\033[0m", 2}, // ANSI stripped
		{"¶✦✓", 3},              // note/model/task marks — all 1 cell
	}
	for _, tt := range tests {
		if got := cellWidth(tt.s); got != tt.want {
			t.Errorf("cellWidth(%q)=%d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestFitCell(t *testing.T) {
	// Pads short content with spaces to exactly w cells.
	if got := fitCell("ab", 5); cellWidth(got) != 5 {
		t.Errorf("fitCell pad: cellWidth=%d, want 5 (%q)", cellWidth(got), got)
	}
	// Truncates long content to exactly w cells (ellipsis included).
	if got := fitCell("abcdefghij", 5); cellWidth(got) != 5 {
		t.Errorf("fitCell trunc: cellWidth=%d, want 5 (%q)", cellWidth(got), got)
	}
	// Padding/truncation lands on exactly w cells for multi-rune input too.
	if got := fitCell("¶✦✓⌂", 5); cellWidth(got) != 5 {
		t.Errorf("fitCell emoji: cellWidth=%d, want 5 (%q)", cellWidth(got), got)
	}
}

// TestRenderRich_BoxAligned asserts every emitted line has the SAME cell width
// (the box's right border lines up) across tiers and widths.
func TestRenderRich_BoxAligned(t *testing.T) {
	dir := writeTaskStatus(t, "lets-ds6bc|StatusLine 2.0 with a deliberately long title to force truncation|7|2099-01-01T00:00:00Z")
	in := richTestInput()
	// Cover both palettes AND a pathologically long branch — the alignment
	// invariant (every row the same cellWidth) is the box's headline correctness.
	branches := map[string]string{
		"normal": "feature/lets-ds6bc-statusline-2-0",
		"long":   "feature/lets-ds6bc-a-very-long-branch-name-that-overflows-narrow-tiers",
	}
	for _, light := range []bool{false, true} {
		for bname, branch := range branches {
			for _, width := range []int{120, 106, 95, 70, 50} {
				var buf bytes.Buffer
				if err := renderRich(&buf, in, branch, "folder", usage{}, width, dir, light, true, true, true); err != nil {
					t.Fatalf("light=%v branch=%s width=%d: %v", light, bname, width, err)
				}
				lines := splitNonEmptyLines(buf.String())
				if len(lines) == 0 {
					t.Fatalf("light=%v branch=%s width=%d: no lines", light, bname, width)
				}
				want := cellWidth(lines[0])
				for i, ln := range lines {
					if got := cellWidth(ln); got != want {
						t.Errorf("light=%v branch=%s width=%d line %d cellWidth=%d, want %d (ragged box): %q",
							light, bname, width, i, got, want, stripANSI(ln))
					}
				}
			}
		}
	}
}

func TestLimit(t *testing.T) {
	// payload reset present -> payload authoritative (even at a genuine 0%).
	if p, r, ok := limit(58.4, "2099-01-01T00:00:00Z", 0, "", false); !ok || p != 58 || r != "2099-01-01T00:00:00Z" {
		t.Errorf("payload present: got (%d,%q,%v), want (58,reset,true)", p, r, ok)
	}
	if p, _, ok := limit(0, "2099-01-01T00:00:00Z", 99, "x", true); !ok || p != 0 {
		t.Errorf("live 0%% must stay 0/authoritative, not fall to cache: got (%d,_,%v)", p, ok)
	}
	// payload absent -> fall back to cache.
	if p, r, ok := limit(0, "", 73, "2099-02-02T00:00:00Z", true); !ok || p != 73 || r != "2099-02-02T00:00:00Z" {
		t.Errorf("cache fallback: got (%d,%q,%v), want (73,reset,true)", p, r, ok)
	}
	// both absent -> not ok.
	if _, _, ok := limit(0, "", 0, "", false); ok {
		t.Error("both absent should return ok=false")
	}
}

// TestRenderRich_TierContent asserts the tiers differ by CONTENT, not just line
// count: Full carries the long brand, PR, token detail, and the model's
// "(… context)" paren; Compact uses the short "LETS" brand and drops the PR,
// token detail, and paren — but KEEPS the model name + effort.
func TestRenderRich_TierContent(t *testing.T) {
	dir := writeTaskStatus(t, "lets-ds6bc|Statusline 2.0 rich renderer|3|2099-01-01T00:00:00Z")
	branch := "feature/lets-ds6bc-statusline-2-0"
	in := richTestInput()
	in.Model.DisplayName = "Opus 4.7 (1M context)"
	in.ContextWindow.ContextWindowSize = 1000000

	var full, compact bytes.Buffer
	if err := renderRich(&full, in, branch, "folder", usage{}, 160, dir, false, true, true, true); err != nil {
		t.Fatalf("full: %v", err)
	}
	if err := renderRich(&compact, in, branch, "folder", usage{}, 65, dir, false, true, true, true); err != nil {
		t.Fatalf("compact: %v", err)
	}
	fp, cp := stripANSI(full.String()), stripANSI(compact.String())

	for _, want := range []string{"LETS Workflow", "#91", "approved", "high", "/1000k)", "1M context"} {
		if !strings.Contains(fp, want) {
			t.Errorf("Full tier missing %q:\n%s", want, fp)
		}
	}
	if strings.Contains(cp, "LETS Workflow") {
		t.Errorf("Compact tier should use short 'LETS' brand, not 'LETS Workflow':\n%s", cp)
	}
	// Compact keeps the model head + effort, just without the paren.
	for _, want := range []string{"LETS", "Opus 4.7", "high"} {
		if !strings.Contains(cp, want) {
			t.Errorf("Compact tier missing %q:\n%s", want, cp)
		}
	}
	for _, drop := range []string{"#91", "approved", "1M context"} {
		if strings.Contains(cp, drop) {
			t.Errorf("Compact tier should drop %q:\n%s", drop, cp)
		}
	}
}

// TestRenderRich_NoDir: showDir gates the Full-tier location pill — false drops
// the folder/worktree badge, true keeps it (--no-dir / LETS_STATUSLINE_DIR=off).
func TestRenderRich_NoDir(t *testing.T) {
	dir := t.TempDir()
	in := richTestInput()
	branch := "feature/lets-ds6bc-x" // not a worktree → pill shows the folder name

	var on, off bytes.Buffer
	if err := renderRich(&on, in, branch, "myproj", usage{}, 160, dir, false, true, true, true); err != nil {
		t.Fatalf("showDir on: %v", err)
	}
	if err := renderRich(&off, in, branch, "myproj", usage{}, 160, dir, false, true, false, true); err != nil {
		t.Fatalf("showDir off: %v", err)
	}
	if !strings.Contains(stripANSI(on.String()), "myproj") {
		t.Errorf("showDir=true should show the location pill:\n%s", on.String())
	}
	if strings.Contains(stripANSI(off.String()), "myproj") {
		t.Errorf("showDir=false should hide the location pill:\n%s", off.String())
	}
}

// TestRenderRich_NoTask: showTask gates the task line — false drops it (the
// divider stays for a consistent frame), true keeps it (--no-task /
// LETS_STATUSLINE_TASK=off).
func TestRenderRich_NoTask(t *testing.T) {
	dir := writeTaskStatus(t, "lets-ds6bc|Statusline 2.0|3|2099-01-01T00:00:00Z")
	branch := "feature/lets-ds6bc-statusline-2-0"
	in := richTestInput()

	var on, off bytes.Buffer
	if err := renderRich(&on, in, branch, "folder", usage{}, 120, dir, false, true, true, true); err != nil {
		t.Fatalf("showTask on: %v", err)
	}
	if err := renderRich(&off, in, branch, "folder", usage{}, 120, dir, false, true, true, false); err != nil {
		t.Fatalf("showTask off: %v", err)
	}
	if !strings.Contains(stripANSI(on.String()), "Statusline 2.0") {
		t.Errorf("showTask=true should show the task line:\n%s", on.String())
	}
	if strings.Contains(stripANSI(off.String()), "Statusline 2.0") {
		t.Errorf("showTask=false should hide the task line:\n%s", off.String())
	}
	// Frame stays consistent: hiding the task line drops exactly one row.
	if onN, offN := len(splitNonEmptyLines(on.String())), len(splitNonEmptyLines(off.String())); offN != onN-1 {
		t.Errorf("showTask=false should drop exactly the task row: on=%d off=%d", onN, offN)
	}
}

// TestRenderRich_AllRowsOff: the all-off state (--no-task + --no-tip, no confirmed
// task) must still produce a valid box — top, identity, budget, bottom (4 lines)
// with NO dangling ├ divider above the └ bottom. This is the only path where
// rule() fires with no row after it; the conditional-divider guard must hold.
func TestRenderRich_AllRowsOff(t *testing.T) {
	dir := t.TempDir() // no task-status → no task line regardless
	in := richTestInput()
	for _, w := range []int{120, 65} { // Full + Compact
		var buf bytes.Buffer
		// light=false, showTip=false, showDir=true, showTask=false
		if err := renderRich(&buf, in, "feature/lets-ds6bc-x", "folder", usage{}, w, dir, false, false, true, false); err != nil {
			t.Fatalf("width=%d: %v", w, err)
		}
		if n := len(splitNonEmptyLines(buf.String())); n != 4 {
			t.Errorf("width=%d: want 4 lines (top,identity,budget,bottom), got %d:\n%s", w, n, buf.String())
		}
		if strings.Contains(stripANSI(buf.String()), "├") {
			t.Errorf("width=%d: dangling ├ divider with no row below it:\n%s", w, buf.String())
		}
	}
}

// TestRenderRich_WorktreePill: inside a worktree the location pill reads the
// literal "worktree", never the folder basename (which equals the branch).
func TestRenderRich_WorktreePill(t *testing.T) {
	dir := t.TempDir()
	in := richTestInput()
	in.Worktree.Name = "wt1" // inWorktree via Worktree.Name; branch has no "worktree" text
	var buf bytes.Buffer
	if err := renderRich(&buf, in, "feature/lets-ds6bc-x", "zzfolderzz", usage{}, 160, dir, false, true, true, true); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	out := stripANSI(buf.String())
	if !strings.Contains(out, "worktree") {
		t.Errorf("worktree session should show the 'worktree' pill:\n%s", out)
	}
	if strings.Contains(out, "zzfolderzz") {
		t.Errorf("worktree pill must NOT show the folder basename:\n%s", out)
	}
}

// TestRenderRich_LocationGlyph pins the location-pill marker to the
// font-portable » (U+00BB) and guards against the 2-cell ☰ (U+2630) that
// font-substituted to 2 cells in cmux/Ghostty and drifted the border (lets-6md86).
func TestRenderRich_LocationGlyph(t *testing.T) {
	dir := t.TempDir()
	in := richTestInput()
	var buf bytes.Buffer
	// Full tier, non-worktree branch → location pill shows the folder name + marker.
	if err := renderRich(&buf, in, "feature/lets-6md86-x", "myproj", usage{}, 160, dir, false, true, true, true); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	out := stripANSI(buf.String())
	if strings.Contains(out, "☰") {
		t.Errorf("location pill must not use the font-substituting ☰ (U+2630):\n%s", out)
	}
	if !strings.Contains(out, "»") {
		t.Errorf("location pill should use the » (U+00BB) marker:\n%s", out)
	}
}

// TestInWorktree pins both OR'd signals (Worktree.Name set, or a worktree- branch).
func TestInWorktree(t *testing.T) {
	cases := []struct {
		name, branch, wtName string
		want                 bool
	}{
		{"branch-prefix", "worktree-foo", "", true},
		{"worktree-name", "feature/x", "wt1", true},
		{"neither", "feature/x", "", false},
		{"main", "main", "", false},
	}
	for _, c := range cases {
		var in Input
		in.Worktree.Name = c.wtName
		if got := inWorktree(in, c.branch); got != c.want {
			t.Errorf("%s: inWorktree(%q, name=%q)=%v, want %v", c.name, c.branch, c.wtName, got, c.want)
		}
	}
}

// TestRenderRich_BareFolderSuppressed: a bare "." folder with no branch must not
// render a stray "☰ ." pill or "⎇ ." branch segment (empty-stdin / non-repo).
func TestRenderRich_BareFolderSuppressed(t *testing.T) {
	dir := t.TempDir()
	in := richTestInput()
	var buf bytes.Buffer
	if err := renderRich(&buf, in, "", ".", usage{}, 160, dir, false, false, true, false); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	out := stripANSI(buf.String())
	if strings.Contains(out, "⎇ .") || strings.Contains(out, "☰ .") {
		t.Errorf("bare '.' folder/branch must be suppressed, not rendered:\n%s", out)
	}
}

// TestRenderRich_FlexTitleKeepsSuffix: a long task title must clip (…) while the
// note-count + hint suffix stays inside the box, not get eaten by the title.
func TestRenderRich_FlexTitleKeepsSuffix(t *testing.T) {
	long := "lets-ds6bc|" + strings.Repeat("very long title segment ", 8) + "|9|2099-01-01T00:00:00Z"
	dir := writeTaskStatus(t, long)
	branch := "feature/lets-ds6bc-statusline-2-0"
	var buf bytes.Buffer
	if err := renderRich(&buf, richTestInput(), branch, "folder", usage{}, 120, dir, false, true, true, true); err != nil {
		t.Fatalf("renderRich: %v", err)
	}
	plain := stripANSI(buf.String())
	if !strings.Contains(plain, "…") {
		t.Errorf("long title should be clipped with an ellipsis:\n%s", plain)
	}
	if !strings.Contains(plain, "/lets:note") {
		t.Errorf("suffix (→ /lets:note) must survive a long title:\n%s", plain)
	}
	if !strings.Contains(plain, "9 comments") {
		t.Errorf("note count must survive a long title:\n%s", plain)
	}
}

func TestTaskIDFromBranch(t *testing.T) {
	tests := map[string]string{
		"feature/lets-ds6bc-statusline-2-0":  "lets-ds6bc",
		"worktree-lets-ds6bc-statusline-2-0": "lets-ds6bc",
		"bug/lets-asdsad-asdasd":             "lets-asdsad",
		"fix/lets-abc-foo":                   "lets-abc",
		"bugfix-2/lets-abc-foo":              "lets-abc", // hyphenated prefix stripped
		"lets-hdrdr.3-subtask":               "lets-hdrdr.3",
		"main":                               "",
		"":                                   "",
	}
	for branch, want := range tests {
		if got := taskIDFromBranch(branch); got != want {
			t.Errorf("taskIDFromBranch(%q)=%q, want %q", branch, got, want)
		}
	}
}

// TestRenderRich_TaskLineGated: the task line shows only when the cache confirms
// a real task (taskOK && title); a bogus/no-beads branch shows no task line.
func TestRenderRich_TaskLineGated(t *testing.T) {
	in := richTestInput()

	// Confirmed task in cache -> task line present.
	dir := writeTaskStatus(t, "lets-ds6bc|Real Title|2|2099-01-01T00:00:00Z")
	var ok bytes.Buffer
	if err := renderRich(&ok, in, "feature/lets-ds6bc-x", "f", usage{}, 120, dir, false, true, true, true); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	if !strings.Contains(stripANSI(ok.String()), "✓ lets-ds6bc Real Title") {
		t.Errorf("confirmed task should render the task line:\n%s", stripANSI(ok.String()))
	}

	// Branch yields a candidate id but no cache entry -> no task line.
	var bogus bytes.Buffer
	if err := renderRich(&bogus, in, "my-random-branch", "f", usage{}, 120, t.TempDir(), false, true, true, true); err != nil {
		t.Fatalf("bogus: %v", err)
	}
	if strings.Contains(stripANSI(bogus.String()), "✓ lets-") {
		t.Errorf("unconfirmed/bogus branch must NOT render a task line:\n%s", stripANSI(bogus.String()))
	}
}
