package statusline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// rich.go — "Quiet Rails" rich statusline (lets-ds6bc), Direction A spec
// (reference/LETS Statusline Spec.md). Behind LETS_STATUSLINE_LEVEL=rich; the
// default compact path (renderLines, render.go) is untouched and frozen.
//
// Two tiers (levelForWidth): Full (>= bpWide) and Compact (below), both wrapped
// in a closed box frame (┌─┐ / │ / ├─┤ / └─┘) sized cell-accurately (fitCell)
// so the right border aligns despite double-width emoji. Layout:
//   ┌──────────────────────────────────────────────────────────┐
//   │ <plant> LETS[ Workflow] <ver> » branch · +A -D · [pill] · PR │  identity
//   │ <model> <effort> » window % (used/total) · 5h % (Δ) · 7d % (Δ) │  budget
//   ├──────────────────────────────────────────────────────────┤
//   │ task-id [title] [· note-count age → /lets:note (Full only)]  │  task
//   │ <tip>                                                       │  tip
//   └──────────────────────────────────────────────────────────┘
// Compact keeps brand+version but drops the location pill, PR and effort, and
// shows a shorter "LETS" brand; its task line is id+title only (no note-count/
// age/hint). Neither tier has progress bars — usage is shown as percentages
// with token counts (window) and paren reset deltas (5h/7d). The box is sized
// by the identity + budget rows only; the task title and tip are fit into that
// width (emitFlex) so they never stretch the box. No bd/network per render.
//
// ansiReset / separatorAngle / separatorMidDot live in render.go (reused here).

// ---- SGR attributes ----
const (
	ansiBold   = "\033[1m"
	ansiItalic = "\033[3m"
)

// palette holds the resolved tokens for one terminal background (spec §1).
// pillBg is a 48;2 background; all other fields are 38;2 foregrounds.
type palette struct {
	sage, clay, gold, ok, warn, alert string // accents
	text, label, dim, sep, pillBg     string // neutrals
}

var paletteDark = palette{
	sage:   "\033[38;2;123;166;137m",
	clay:   "\033[38;2;207;142;128m",
	gold:   "\033[38;2;205;166;92m",
	ok:     "\033[38;2;127;169;139m",
	warn:   "\033[38;2;203;162;78m",
	alert:  "\033[38;2;201;119;107m",
	text:   "\033[38;2;215;217;224m",
	label:  "\033[38;2;138;143;163m",
	dim:    "\033[38;2;91;96;114m",
	sep:    "\033[38;2;60;65;80m",
	pillBg: "\033[48;2;42;46;58m",
}

var paletteLight = palette{
	sage:   "\033[38;2;78;131;105m",
	clay:   "\033[38;2;181;112;95m",
	gold:   "\033[38;2;169;132;47m",
	ok:     "\033[38;2;78;131;105m",
	warn:   "\033[38;2;169;132;47m",
	alert:  "\033[38;2;184;91;77m",
	text:   "\033[38;2;44;46;54m",
	label:  "\033[38;2;107;113;133m",
	dim:    "\033[38;2;154;159;176m",
	sep:    "\033[38;2;213;209;198m",
	pillBg: "\033[48;2;238;234;225m",
}

// Glyphs — universal Unicode/emoji set that renders on macOS and Linux without
// a special font (deliberately NOT Nerd Font: no font dependency to maintain).
// model uses ✦ (U+2726); ↻ ⇄ ⎇ are monochrome symbols, the rest are emoji.
const (
	glyphSprout = "🌱"
	glyphBranch = "⎇"
	glyphTask   = "✓"
	glyphNote   = "¶"
	glyphModel  = "✦"
	glyphTip    = "*"
	glyphPR     = "⇄"
	glyphArrow  = "←"
	glyphFolder = "☰" // location-pill mark (U+2630, text, 1 cell — takes palette color)
	glyphPlant  = "⚘" // static brand mark (text, 1 cell) — growth ladder parked below
)

// growthLadder maps a monotonic session growth score (cost.total_lines_added)
// to the brand emoji on Line 1 (spec §8.2, tropical finale). The plant matures
// as you edit more this session; it is session-scoped — total_lines_added resets
// each new session, so a fresh session starts back at 🌱. E1 caveat: emoji ignore
// ANSI color, so the brand color comes from the bold "LETS Workflow" text, not
// the glyph.
// growthLadder — PARKED. The session-growth brand ladder (🌱→🪴→🌿→🌳→🌴 by
// cost.total_lines_added) is currently disabled in favor of a single static
// text mark (glyphPlant ⚘) for a consistent, monochrome identity line. To
// restore the growth animation: un-comment this var + the laddered brandEmoji
// below, drop the static brandEmoji, and re-add 🌱🪴🌿🌳🌴 to wideRunes.
//
// var growthLadder = []struct {
// 	min   int
// 	emoji string
// }{
// 	{0, glyphSprout}, // 🌱 sprout
// 	{50, "🪴"},        // potted plant
// 	{100, "🌿"},       // leafy bush
// 	{250, "🌳"},       // tree
// 	{500, "🌴"},       // palm (tropical finale)
// }
//
// func brandEmoji(linesAdded int) string {
// 	e := growthLadder[0].emoji
// 	for _, g := range growthLadder {
// 		if linesAdded >= g.min {
// 			e = g.emoji
// 		}
// 	}
// 	return e
// }

// brandEmoji returns the static brand mark. (Growth ladder parked — see above.)
func brandEmoji(int) string { return glyphPlant }

// Usage thresholds, inclusive lower bound (spec §5).
const (
	threshMid  = 50 // pct >= 50 -> warn
	threshHigh = 75 // pct >= 75 -> alert
)

// Width breakpoint on COLUMNS — two tiers only. Full (reset timers, PR, location
// pill, task notes/age/hint) shows at >= bpWide; below that, Compact: trimmed
// lines — short "LETS" brand, id+title task line, no PR/effort/note detail. The
// box always hugs the content and is capped at min(fullMaxLine, COLUMNS-4), so
// at the lower Full widths (72-100) the box simply hugs the terminal and the
// identity/budget rows clip — Full degrades gracefully rather than needing 100+.
const (
	bpWide    = 72     // Full at >= this; Compact below
	bpDefault = bpWide // DEC-2: COLUMNS confirmed passed by CC; fail open to Full when it is somehow absent
	// bpFill: below this width the box fills the whole window so the tip/content
	// get the full width; at/above it the box hugs the content instead of
	// stretching across a wide screen.
	bpFill = 90
	// boxRightMargin: cells left empty on the right so the box never reaches the
	// last column. Claude Code's actual statusline area is a few cells narrower
	// than the COLUMNS it passes (left indent + reserved gutter), and ambiguous-
	// width glyphs (☑, →, ⎇) render wider than cellWidth counts — so without a
	// margin CC truncates each line with its own "…". 4 covers both.
	boxRightMargin = 4
)

// fullMaxLine bounds the box's inner width so a wide terminal doesn't stretch
// the bar full width. Long rows (branch, title, tip) are not pre-truncated —
// the box's fitCell end-truncates whatever overflows this width.
const fullMaxLine = 120

// effortColor maps an effort level to a color mirroring the /effort picker
// gradient (faster→smarter): low gold, medium green, high blue, xhigh purple,
// max/ultra red. Blue/purple aren't palette tokens (the palette is earthy), so
// they're fixed mid-tones that read on both backgrounds. Unknown → dim.
func (p palette) effortColor(level string) string {
	switch level {
	case "low":
		return p.gold
	case "medium":
		return p.ok
	case "high":
		return "\033[38;2;108;153;217m" // blue
	case "xhigh":
		return "\033[38;2;176;130;217m" // purple
	case "max", "ultra", "ultracode":
		return p.alert
	default:
		return p.dim
	}
}

// Render tiers.
const (
	tierCompact = iota
	tierFull
)

func levelForWidth(width int) int {
	if width >= bpWide {
		return tierFull
	}
	return tierCompact
}

// detectWidth reads terminal width from the COLUMNS env var Claude Code sets;
// falls back to bpDefault when absent/unparseable.
func detectWidth() int {
	if c := strings.TrimSpace(os.Getenv("COLUMNS")); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n >= 20 && n <= 400 {
			return n
		}
	}
	return bpDefault
}

var ansiSGRRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiSGRRe.ReplaceAllString(s, "") }
func visibleWidth(s string) int { return len([]rune(stripANSI(s))) }

// wideRunes are the glyphs this renderer emits that occupy 2 terminal cells
// (emoji). Everything else is treated as 1 cell. Keep this in sync with the
// glyph set + growthLadder, or the box's right border will drift by a cell on
// lines containing a missing entry. CAVEAT: ☑ (U+2611) and → (U+2192) are
// East-Asian *Ambiguous* width — 1 cell on most terminals (incl. the default
// macOS set) but 2 in some CJK/legacy configs; on those the right border can
// drift. Perfect alignment across all terminals isn't achievable with a static
// map; verify on your terminal (or swap those glyphs) if the border looks off.
var wideRunes = map[rune]bool{
	// All emitted glyphs are currently 1-cell text — brand ⚘, folder ☰, notes ¶,
	// model ✦, task ✓, tip *, branch ⎇, PR ⇄, arrow ←. No emoji, so this map is
	// empty and the border never drifts. Parked emoji that would belong here if
	// the growth ladder is restored:
	// '🌱': true, '🪴': true, '🌿': true, '🌳': true, '🌴': true, // brand ladder (parked; static ⚘ now)
}

func runeCells(r rune) int {
	if wideRunes[r] {
		return 2
	}
	return 1
}

// cellWidth is the visible terminal-cell width (ANSI-stripped, emoji counted as
// 2). Used for box framing where the right border must align across lines that
// hold different numbers of double-width emoji.
func cellWidth(s string) int {
	n := 0
	for _, r := range stripANSI(s) {
		n += runeCells(r)
	}
	return n
}

// clipCell cell-accurately truncates s to AT MOST w cells, appending an ellipsis
// on overflow (no padding). Returns "" for w < 1. ANSI escapes are preserved
// (uncounted). The truncatable flex segment of a row goes through this.
func clipCell(s string, w int) string {
	if w < 1 {
		return ""
	}
	if cellWidth(s) <= w {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	cells := 0
	for i := 0; i < len(runes); {
		if runes[i] == '\x1b' { // copy the whole escape verbatim, uncounted
			j := i
			for j < len(runes) && runes[j] != 'm' {
				j++
			}
			if j < len(runes) {
				j++
			}
			b.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		rw := runeCells(runes[i])
		if cells+rw > w-1 { // leave 1 cell for the ellipsis
			break
		}
		b.WriteRune(runes[i])
		cells += rw
		i++
	}
	b.WriteString("…" + ansiReset)
	return b.String()
}

// fitCell adjusts s to EXACTLY w terminal cells: pad short with trailing spaces,
// clip long with clipCell. ANSI escapes preserved. Used to square off box rows.
func fitCell(s string, w int) string {
	cw := cellWidth(s)
	if cw < w {
		return s + strings.Repeat(" ", w-cw)
	}
	if cw == w {
		return s
	}
	out := clipCell(s, w)
	for cellWidth(out) < w { // a broken-before-wide-rune can leave a gap
		out += " "
	}
	return out
}

// richRow is one box line. A plain row drives the box width and is fit whole.
// A flex row has a fixed prefix + suffix and a truncatable middle that absorbs
// overflow — so a long task title shrinks before the notes/age/hint suffix is
// lost — and it does NOT drive the box width.
type richRow struct {
	plain  string // plain row: the whole line (drives box width)
	prefix string // flex row: fixed left
	mid    string // flex row: truncatable middle (shrinks first)
	suffix string // flex row: fixed right
	flex   bool   // true => flex row (does not drive width)
}

// threshold maps a usage pct to its accent token (spec §5).
func (p palette) threshold(pct int) string {
	switch {
	case pct >= threshHigh:
		return p.alert
	case pct >= threshMid:
		return p.warn
	default:
		return p.ok
	}
}

// kfmt renders a token count in thousands, rounded: 380000 -> "380k".
func kfmt(n int) string {
	return strconv.Itoa((n+500)/1000) + "k"
}

// modelName colors a Claude Code model display name: the head (e.g. "Opus 4.8")
// stays bold/accent, while a trailing parenthetical the harness appends (e.g.
// "(1M context)") is dimmed — nothing is dropped. lead is the accent applied to
// the head; the parenthetical always uses p.dim. The first "(" splits the two.
// modelName formats the model display name. With showParen=false the trailing
// "(… context)" suffix is dropped entirely (used in the Compact tier, where the
// paren is the first thing to shed); with true the head is bold and the paren
// is dimmed (Full tier).
func (p palette) modelName(lead, name string, showParen bool) string {
	if i := strings.IndexByte(name, '('); i >= 0 {
		head := strings.TrimRight(name[:i], " ")
		if !showParen {
			return lead + ansiBold + head + ansiReset
		}
		paren := name[i:]
		return lead + ansiBold + head + ansiReset + " " + p.dim + paren + ansiReset
	}
	return lead + ansiBold + name + ansiReset
}

var taskIDRe = regexp.MustCompile(`[a-z][a-z0-9]*-[a-z0-9]+(?:\.[0-9]+)?`)

// taskIDFromBranch extracts a candidate beads task id from the branch name
// (convention <prefix>-<alphanum>[.N]). Free — no bd call; the id is only
// CANDIDATE here, the renderer shows the task line only once bd confirms it.
// Strips any "<word>/" prefix (feature/, bug/, fix/, bugfix-2/, ...) and the
// "worktree-" prefix so the first match is the real id, not the prefix.
func taskIDFromBranch(branch string) string {
	if i := strings.LastIndex(branch, "/"); i >= 0 {
		branch = branch[i+1:]
	}
	branch = strings.TrimPrefix(branch, "worktree-")
	return taskIDRe.FindString(branch)
}

// inWorktree reports whether we're inside a git worktree, cheaply: prefer the
// stdin signal, fall back to the branch-name convention. No git fork.
func inWorktree(in Input, branch string) bool {
	return in.Worktree.Name != "" || strings.HasPrefix(branch, "worktree-")
}

// relAgo renders a past ISO timestamp as a compact age — "37m ago", "1h52m ago",
// "4d22h ago" — sharing fmtDur with the reset deltas so the two never drift.
// "just now" under a minute; empty for blank/invalid/future input.
func relAgo(iso string) string {
	t, ok := parseISO(iso)
	if !ok {
		return ""
	}
	diff := time.Since(t)
	if diff < 0 {
		return ""
	}
	if diff < time.Minute {
		return "just now"
	}
	return fmtDur(diff) + " ago"
}

// readTaskStatus reads a cheap on-change cache (PROTOTYPE: written by hand; real
// wiring = /lets:start /lets:done /lets:note). Single line:
//
//	<task-id>|<title>|<note-count>|<last-comment-iso>
//
// Returns ok=false (graceful degrade) when the file is missing or the cached
// task != active task.
func readTaskStatus(cacheDir, taskID string) (title string, notes int, lastComment string, ok bool) {
	b, err := os.ReadFile(filepath.Join(cacheDir, "task-status"))
	if err != nil {
		return
	}
	parts := strings.SplitN(strings.TrimSpace(string(b)), "|", 4)
	if len(parts) < 2 || parts[0] != taskID {
		return
	}
	title = parts[1]
	if len(parts) > 2 {
		notes, _ = strconv.Atoi(parts[2])
	}
	if len(parts) > 3 {
		lastComment = parts[3]
	}
	ok = true
	return
}

// prStateColor maps pr.review_state to an accent (spec §6 / A5).
func (p palette) prStateColor(state string) string {
	switch state {
	case "approved":
		return p.ok
	case "changes_requested":
		return p.alert
	default: // pending, review_required, ""
		return p.warn
	}
}

// limit resolves a rate-limit gauge: prefer the live payload, fall back to the
// usage cache. ok=false when neither has data.
func limit(payloadPct float64, payloadReset string, cacheP int, cacheReset string, cacheOK bool) (pct int, reset string, ok bool) {
	// resets_at present => the live rate-limit block is in the payload, so even
	// a genuine 0% reading is authoritative — don't conflate it with "absent".
	if payloadReset != "" {
		return int(payloadPct + 0.5), payloadReset, true
	}
	if cacheOK {
		return cacheP, cacheReset, true
	}
	return 0, "", false
}

// tips are loading-screen-style workflow hints. tipOfMoment rotates through them
// so the bottom line cycles as you work. English-only (written artifact).
var tips = []string{
	"Need to save progress? Use /lets:note to record it on the active task.",
	"Quick sanity check before committing? Run /lets:check (~30s, no agents).",
	"Want a deep multi-agent review? /lets:review pulls up to 12 experts.",
	"Stuck on a decision? /lets:opinion gathers expert angles in parallel.",
	"Just need one expert's take? /lets:ask is a quick ping to a colleague.",
	"Ready to commit? /lets:commit enforces the format and links the task.",
	"Task finished? /lets:done opens the PR (or merges) and closes it.",
	"Wrapping up? /lets:end saves a session summary for next time.",
	"Planning a medium task? /lets:plan, then /lets:execute to run it.",
	"Lost track of where you are? /lets:status shows the overview.",
	"Starting fresh? /lets:start restores context and picks a task.",
	"Working in parallel? /lets:worktree create spins up an isolated tree.",
	"Big task? /lets:team runs implementers in parallel worktrees.",
	"Reviewing a GitHub PR? /lets:github-pr drives the full lifecycle.",
	"Exploring ideas? /lets:brainstorm reviews the backlog with you.",
	"Plan looks risky? /lets:check --plan is a fast plan sanity pass.",
	"Never work without a task — /lets:start picks or creates one.",
	"Out of sync with the latest release? /lets:update self-heals config + rules.",
	"New project? /lets:init sets up .lets/, config, statusline and beads.",
	"Found a gotcha? /lets:note captures it so future-you remembers.",
	"Commit early, commit often — /lets:commit keeps diffs reviewable.",
	"One branch per task — every feature gets its own.",
	"Never edit on the merge branch — start a task first.",
	"Quick fix? Pick \"Stay on current branch\" in /lets:start for trunk-mode.",
	"bd ready shows what you can work on right now.",
	"Group beads tasks with labels, not epics.",
	"bd comments add appends context without overwriting the description.",
	"Tasks span sessions; sessions don't. /lets:end any time.",
	"Significant change? /lets:check first, then /lets:review --local.",
	"PR already open? /lets:review <PR> comments straight on GitHub.",
	"Need the lighter review? /lets:check is /lets:review without the agents.",
	"/lets:ask is the lighter /lets:opinion — one expert, fast.",
	"Worktrees share .lets/ — config, plans and sessions stay in sync.",
	"Done in a worktree? /lets:done, then /lets:worktree remove from main.",
	"Long session? Finish current work and /lets:end for a fresh window.",
	"Curious about context use? Run /context — don't guess.",
	"Rename your session: /rename <slug> keeps the statusline meaningful.",
	"/lets:plan --fast skips subagents for a quick orchestrator-only plan.",
	"Plan ready? /lets:execute runs it step by step in plan mode.",
	"Mention a task with its title, not just the id.",
	"/lets:opinion shows a cost note before it launches the agents.",
	"Read the codebase first — match the patterns already there.",
	"Smallest change that solves the problem — easier to review and revert.",
	"Branch piling up unrelated work? Split it into separate PRs.",
	"/lets:status overview is a compact read of the whole board.",
	"Repeated blocker 3x? Stop patching — find the root cause.",
	"Keep written artifacts in English, even when we chat in another language.",
	"/lets:review --file <path> audits an existing file's quality.",
	"bd search <keywords> before creating a task — avoid duplicates.",
	"Pushed your branch? /lets:done already opened the PR for you.",
	"Big review? /lets:review --workflow fans the experts out off your context window.",
	"/lets:review --workflow has skeptic agents refute each finding before you see it.",
	"Torn between options? /lets:opinion --workflow adds an adversarial challenge round.",
	"/lets:explore --workflow runs web-research -> ideate -> cluster as a background fan-out.",
	"--workflow (review/opinion/explore) moves the heavy fan-out off-context — same result, lighter window.",
	"Scoping a new area? /lets:explore scouts the codebase + current community standards.",
	"/lets:explore --no-web keeps it local — skip the web-research stage.",
	"Statusline too busy? --no-task / --no-dir / --no-tip trim it (env LETS_STATUSLINE_*=off too).",
	"About to /compact a long session? /lets:note --pre-compact snapshots it to the task first.",
}

// tipOfMoment cycles tips sequentially, advancing one step every tipPeriod
// seconds — a free rotation with no persisted counter (time is the clock).
func tipOfMoment(now time.Time) string {
	if len(tips) == 0 {
		return ""
	}
	const tipPeriod = 10 // seconds per tip
	idx := int((now.Unix() / tipPeriod) % int64(len(tips)))
	return tips[idx]
}

// renderRich draws the width-responsive Quiet Rails layout. Never calls
// bd/network per render.
func renderRich(w io.Writer, in Input, branch, folder string, u usage, width int, cacheDir string, light, showTip, showDir, showTask bool) error {
	p := paletteDark
	if light {
		p = paletteLight
	}
	tier := levelForWidth(width)
	R := ansiReset
	// Single neutral separator everywhere — the gray middot · . No » marker on any
	// row (the brand/model/title still lead their rows, just without a glyph gap).
	segSep := p.sep + separatorMidDot + R // " · "

	// Closed box frame that HUGS the content: rows are collected, then the box
	// is sized to the widest row (capped to fullMaxLine and width-4) so the right
	// border sits just past the longest line, not at a fixed column. Cell-
	// accurate (fitCell) so the border aligns despite double-width emoji.
	var rows []richRow
	dividerAfter := -1
	emit := func(line string) {
		if visibleWidth(line) == 0 {
			return
		}
		rows = append(rows, richRow{plain: line})
	}
	// emitFlex appends a flex row (prefix + truncatable mid + suffix): the box is
	// sized only by the plain rows (identity + budget/gauges), and at flush the
	// mid is clipped so prefix+mid+suffix fits the box. The title (mid) shrinks
	// first; the id (prefix) and the notes/age/hint (suffix) stay in the frame.
	emitFlex := func(prefix, mid, suffix string) {
		if visibleWidth(prefix)+visibleWidth(mid)+visibleWidth(suffix) == 0 {
			return
		}
		rows = append(rows, richRow{prefix: prefix, mid: mid, suffix: suffix, flex: true})
	}
	rule := func() { dividerAfter = len(rows) - 1 } // ├ divider after the last row so far
	flush := func() {
		// Narrow window (< bpFill): fill the full width so the tip/content get
		// all the room. Wide window: hug the content (sized by the plain rows;
		// title/tip never set the width) so the box doesn't stretch the screen.
		lim := width - 4 - boxRightMargin // keep a right margin (see boxRightMargin)
		boxW := lim                       // narrow window: fill to the margin
		if width >= bpFill {
			boxW = 1
			for _, r := range rows {
				if r.flex {
					continue
				}
				if cw := cellWidth(r.plain); cw > boxW {
					boxW = cw
				}
			}
		}
		if boxW > lim {
			boxW = lim
		}
		if boxW > fullMaxLine {
			boxW = fullMaxLine
		}
		if boxW < 1 {
			boxW = 1
		}
		renderRow := func(r richRow) string {
			if !r.flex {
				return fitCell(r.plain, boxW)
			}
			budget := boxW - cellWidth(r.prefix) - cellWidth(r.suffix)
			return fitCell(r.prefix+clipCell(r.mid, budget)+r.suffix, boxW)
		}
		border := func(left, right string) {
			fmt.Fprintln(w, p.sep+left+strings.Repeat("─", boxW+2)+right+R)
		}
		border("┌", "┐")
		for i, r := range rows {
			fmt.Fprintln(w, p.sep+"│ "+R+renderRow(r)+p.sep+" │"+R)
			// Draw the divider only if there's a row below it — never ├ right
			// above └ (e.g. no task AND no tip).
			if i == dividerAfter && i < len(rows)-1 {
				border("├", "┤")
			}
		}
		border("└", "┘")
	}
	// joinSep concatenates non-empty parts with the given separator.
	joinSep := func(sep string, parts ...string) string {
		kept := make([]string, 0, len(parts))
		for _, x := range parts {
			if visibleWidth(x) > 0 {
				kept = append(kept, x)
			}
		}
		return strings.Join(kept, sep)
	}

	// ----- shared values -----
	// Show the branch name VERBATIM — no "worktree-" prefix stripping. The bar is
	// a search anchor: trimming it means a user grepping for the displayed name
	// wouldn't find the real branch. The location pill already signals "worktree".
	bf := branch
	if bf == "" {
		bf = folder
	}
	// Drop the branch segment entirely for a meaningless value (empty / bare cwd)
	// rather than rendering a stray "⎇ ." on empty-stdin or a non-repo dir.
	branchSeg := ""
	if bf != "" && bf != "." && bf != "/" {
		branchSeg = p.clay + glyphBranch + " " + bf + R
	}
	id := taskIDFromBranch(branch)
	title, notes, lastComment, taskOK := readTaskStatus(cacheDir, id)
	winPct := int(in.ContextWindow.UsedPercentage + 0.5)
	fiveP, fiveReset, fiveOK := limit(in.RateLimits.FiveHour.UsedPercentage, string(in.RateLimits.FiveHour.ResetsAt), u.fiveHour, u.fiveHourReset, u.fiveHourOK)
	sevenP, sevenReset, sevenOK := limit(in.RateLimits.SevenDay.UsedPercentage, string(in.RateLimits.SevenDay.ResetsAt), u.sevenDay, u.sevenDayReset, u.sevenDayOK)

	diffSeg := ""
	if a, d := in.Cost.TotalLinesAdded, in.Cost.TotalLinesRemoved; a > 0 || d > 0 {
		diffSeg = p.ok + "+" + strconv.Itoa(a) + R + " " + p.alert + "-" + strconv.Itoa(d) + R
	}

	// gauge variants
	// The label shares the percentage's threshold color so the whole "window 44%"
	// pair reads as one colored unit at a glance (the reset delta stays dim).
	gaugeParens := func(label string, pct int, resetISO string) string { // label + pct + (delta), no bar (Full)
		tc := p.threshold(pct)
		s := tc + label + " " + strconv.Itoa(pct) + "%" + R
		if resetISO != "" {
			if dl := computeDelta(resetISO); dl != "" {
				s += " " + p.dim + "(" + dl + ")" + R
			}
		}
		return s
	}
	gaugeCompact := func(label string, pct int, resetISO string) string { // label + pct + timer, no bar (Compact)
		tc := p.threshold(pct)
		s := tc + label + " " + strconv.Itoa(pct) + "%" + R
		if resetISO != "" {
			if dl := computeDeltaCompact(resetISO); dl != "" {
				s += " " + p.dim + dl + R
			}
		}
		return s
	}

	ver := version.Version
	if !version.IsDev() {
		ver = "v" + ver
	}

	// ===== Compact tier: 4 trimmed lines, designed for ~70 cols =====
	if tier == tierCompact {
		// Line 1: brand+version » branch · diff (no worktree pill, no PR; short "LETS").
		brandC := p.sage + brandEmoji(in.Cost.TotalLinesAdded) + " " + ansiBold + "LETS" + R + " " + p.dim + ver + R
		if restC := joinSep(segSep, branchSeg, diffSeg); restC != "" {
			brandC += segSep + restC
		}
		emit(brandC)
		// Line 2: model + effort (without the "(… context)" paren — the paren is
		// the first thing to shed at this width; model name + effort stay because
		// they're high-signal) » window·5h·7d label+pct+timer, no bars.
		budgetC := p.gold + glyphModel + " " + p.modelName(p.gold, in.Model.DisplayName, false)
		if in.Effort.Level != "" {
			budgetC += " " + p.effortColor(in.Effort.Level) + in.Effort.Level + R
		}
		g := []string{gaugeCompact("w", winPct, "")}
		if fiveOK {
			g = append(g, gaugeCompact("5h", fiveP, fiveReset))
		}
		if sevenOK {
			g = append(g, gaugeCompact("7d", sevenP, sevenReset))
		}
		emit(budgetC + segSep + joinSep(segSep, g...))
		rule() // divider ALWAYS after gauges — consistent frame with/without task
		// Line 3: task — only when bd CONFIRMED a real task (taskOK && title); a
		// bare/bogus branch id or a no-beads project drops ONLY this line (the
		// divider stays). id is the prefix, title the truncatable mid.
		if showTask && taskOK && title != "" {
			emitFlex(p.clay+glyphTask+" "+id+R, " "+p.text+title+R, "")
		}
		// Line 4: rotating tip — glyph (prefix) + text (truncatable mid).
		if t := tipOfMoment(time.Now()); showTip && t != "" {
			emitFlex(p.dim+glyphTip+R+" ", p.dim+ansiItalic+t+R, "")
		}
		flush() // size + draw the box around the collected rows
		return nil
	}

	// ===== Full tier: 4 lines, everything (each capped to fullMaxLine) =====
	emitFull := emit // rows are sized to the box at flush() time

	// --- Line 1: identity (branch truncated so the line stays under the cap) ---
	brand := p.sage + brandEmoji(in.Cost.TotalLinesAdded) + " " + ansiBold + "LETS Workflow" + R + " " + p.dim + ver + R
	// Location badge right after the marker (Full tier only — Compact drops it for
	// width; --no-dir / LETS_STATUSLINE_DIR=off hides it too). Inside a worktree
	// it's just the word "worktree" — the worktree dir name equals the branch
	// name, so repeating it here would be noise. Outside a worktree it's the
	// project root folder name.
	locName := ""
	if inWorktree(in, branch) {
		locName = "worktree"
	} else if folder != "" && folder != "." && folder != "/" {
		locName = folder
	}
	locSeg := ""
	if showDir && locName != "" {
		locSeg = p.pillBg + p.label + " " + glyphFolder + " " + locName + " " + R
	}
	prSeg := ""
	if in.PR.Number > 0 {
		prSeg = p.label + glyphPR + " #" + strconv.Itoa(in.PR.Number) + R
		if in.PR.ReviewState != "" {
			prSeg += " " + p.prStateColor(in.PR.ReviewState) + in.PR.ReviewState + R
		}
	}
	identity := brand
	if rest := joinSep(segSep, locSeg, branchSeg, diffSeg, prSeg); rest != "" {
		identity += segSep + rest
	}
	emitFull(identity)

	// --- Line 2: budget (model + colored effort » window/5h/7d, no bars) ---
	budget := p.gold + glyphModel + " " + p.modelName(p.gold, in.Model.DisplayName, true)
	if in.Effort.Level != "" {
		budget += " " + p.effortColor(in.Effort.Level) + in.Effort.Level + R
	}
	// window: pct + (used/total tokens); 5h/7d: pct + (reset delta). Label shares
	// the threshold color with the pct (matches the 5h/7d gauges above).
	winSeg := p.threshold(winPct) + "window " + strconv.Itoa(winPct) + "%" + R
	if sz := in.ContextWindow.ContextWindowSize; sz > 0 {
		used := int(in.ContextWindow.UsedPercentage/100*float64(sz) + 0.5)
		winSeg += " " + p.dim + "(" + kfmt(used) + "/" + kfmt(sz) + ")" + R
	}
	gauges := []string{winSeg}
	if fiveOK {
		gauges = append(gauges, gaugeParens("5h", fiveP, fiveReset))
	}
	if sevenOK {
		gauges = append(gauges, gaugeParens("7d", sevenP, sevenReset))
	}
	emitFull(budget + segSep + joinSep(segSep, gauges...))

	rule() // divider ALWAYS after budget — consistent frame with/without task
	// --- Line 3: task — only when bd CONFIRMED a real task (taskOK && title);
	//     id (prefix) + title (truncatable mid) + notes/age/hint (suffix). A
	//     bare/bogus branch id or no-beads drops ONLY this line (divider stays).
	//     --no-task / LETS_STATUSLINE_TASK=off hides it regardless. ---
	if showTask && taskOK && title != "" {
		noteSeg := ""
		if notes > 0 {
			label := " comments"
			if notes == 1 {
				label = " comment"
			}
			noteSeg = p.label + strconv.Itoa(notes) + label + R
			if age := relAgo(lastComment); age != "" {
				noteSeg += " " + p.dim + "(" + age + ")" + R
			}
		}
		// Neutral gray middot after the title (no » outside the identity row); the
		// → /lets:note hint is fully dim too.
		hint := p.dim + glyphArrow + " /lets:note" + R
		tail := hint
		if noteSeg != "" {
			tail = noteSeg + " " + hint
		}
		emitFlex(p.clay+glyphTask+" "+id+R, " "+p.text+title+R, segSep+tail)
	}

	// --- Line 4: rotating tip — glyph (prefix) + text (truncatable mid) ---
	if t := tipOfMoment(time.Now()); showTip && t != "" {
		emitFlex(p.dim+glyphTip+R+" ", p.dim+ansiItalic+t+R, "")
	}
	flush() // size + draw the box around the collected rows
	return nil
}
