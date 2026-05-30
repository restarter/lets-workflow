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
// 4 lines, width-responsive (levelForWidth / spec §4):
//   1 Identity: 🌱 LETS Workflow <ver> » branch · +A -D · [worktree] · PR
//   2 Task:     task-id (title) · note-count age → /lets:note
//   3 Budget:   model effort · window <bar> % · 5h <bar> % reset · 7d <bar> % reset
//   4 Tip:      rotating workflow hint
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
	pillBg: "\033[48;2;230;226;216m",
}

// Glyphs — universal Unicode/emoji set that renders on macOS and Linux without
// a special font (deliberately NOT Nerd Font: no font dependency to maintain).
// model uses ✦ (U+2726); ↻ ⇄ ⎇ are monochrome symbols, the rest are emoji.
const (
	glyphSprout   = "🌱"
	glyphBranch   = "⎇"
	glyphTask     = "☑"
	glyphNote     = "📝"
	glyphModel    = "✦"
	glyphTip      = "💡"
	glyphWorktree = "🗂"
	glyphPR       = "⇄"
	glyphReset    = "↻"
	glyphArrow    = "→"
	barFill       = "█"
	barEmpty      = "░"
)

const barWidth = 8 // gauge cells (spec §5)

// Usage thresholds, inclusive lower bound (spec §5).
const (
	threshMid  = 60 // pct >= 60 -> warn
	threshHigh = 85 // pct >= 85 -> alert
)

// Width breakpoints on COLUMNS (spec §4).
const (
	bpFull    = 160
	bpMid     = 110
	bpNarrow  = 80
	bpDefault = bpFull // DEC-2: fail open to Full when COLUMNS is unknown (CC statusline subprocesses historically don't inherit COLUMNS) rather than pinning everyone to the degraded Narrow tier
)

// Render tiers (ascending detail).
const (
	tierMin = iota
	tierNarrow
	tierMid
	tierFull
)

func levelForWidth(width int) int {
	switch {
	case width >= bpFull:
		return tierFull
	case width >= bpMid:
		return tierMid
	case width >= bpNarrow:
		return tierNarrow
	default:
		return tierMin
	}
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

// clip end-truncates to a visible-width budget, preserving ANSI escapes so the
// kept prefix stays colored. Adds an ellipsis + reset on overflow.
func clip(s string, max int) string {
	if max <= 1 || visibleWidth(s) <= max {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	vis := 0
	for i := 0; i < len(runes); {
		if runes[i] == '\x1b' { // copy the whole \x1b[...m verbatim, uncounted
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
		if vis >= max-1 {
			break
		}
		b.WriteRune(runes[i])
		vis++
		i++
	}
	b.WriteString("…" + ansiReset)
	return b.String()
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

// miniBar renders the 8-cell gauge: filled cells in the threshold color, empty
// cells in sep (spec §5 / A3).
func (p palette) miniBar(pct int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := (pct*barWidth + 50) / 100
	var b strings.Builder
	b.WriteString(p.threshold(pct))
	b.WriteString(strings.Repeat(barFill, filled))
	b.WriteString(p.sep)
	b.WriteString(strings.Repeat(barEmpty, barWidth-filled))
	b.WriteString(ansiReset)
	return b.String()
}

var taskIDRe = regexp.MustCompile(`[a-z][a-z0-9]*-[a-z0-9]+(?:\.[0-9]+)?`)

// taskIDFromBranch extracts a beads task id from the branch name (detect-task
// pattern <prefix>-<alphanum>[.N]). Free — no bd call.
func taskIDFromBranch(branch string) string {
	b := strings.TrimPrefix(branch, "feature/")
	b = strings.TrimPrefix(b, "worktree-")
	return taskIDRe.FindString(b)
}

// inWorktree reports whether we're inside a git worktree, cheaply: prefer the
// stdin signal, fall back to the branch-name convention. No git fork.
func inWorktree(in Input, branch string) bool {
	return in.Worktree.Name != "" || strings.HasPrefix(branch, "worktree-")
}

// relAgo renders a past ISO timestamp as "N min ago" / "Nh ago" / "Nd ago".
// Empty for blank/invalid/future input.
func relAgo(iso string) string {
	t, ok := parseISO(iso)
	if !ok {
		return ""
	}
	diff := time.Since(t)
	switch {
	case diff < 0:
		return ""
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%d min ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(diff.Hours())/24)
	}
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
func renderRich(w io.Writer, in Input, branch, folder string, u usage, width int, cacheDir string, light bool) error {
	p := paletteDark
	if light {
		p = paletteLight
	}
	tier := levelForWidth(width)
	R := ansiReset
	marker := p.sage + ansiBold + separatorAngle + R // " » "
	segSep := p.sep + separatorMidDot + R            // " · "

	emit := func(line string) {
		if visibleWidth(line) == 0 {
			return
		}
		fmt.Fprintln(w, clip(line, width))
	}
	// join concatenates non-empty parts with the " · " separator.
	join := func(parts ...string) string {
		kept := make([]string, 0, len(parts))
		for _, x := range parts {
			if visibleWidth(x) > 0 {
				kept = append(kept, x)
			}
		}
		return strings.Join(kept, segSep)
	}

	// ----- shared values -----
	bf := branch
	if bf == "" {
		bf = folder
	}
	if inWorktree(in, branch) {
		bf = strings.TrimPrefix(bf, "worktree-")
	}
	id := taskIDFromBranch(branch)
	title, notes, lastComment, taskOK := readTaskStatus(cacheDir, id)
	winPct := int(in.ContextWindow.UsedPercentage + 0.5)
	fiveP, fiveReset, fiveOK := limit(in.RateLimits.FiveHour.UsedPercentage, in.RateLimits.FiveHour.ResetsAt, u.fiveHour, u.fiveHourReset, u.fiveHourOK)
	sevenP, sevenReset, sevenOK := limit(in.RateLimits.SevenDay.UsedPercentage, in.RateLimits.SevenDay.ResetsAt, u.sevenDay, u.sevenDayReset, u.sevenDayOK)

	diffSeg := ""
	if a, d := in.Cost.TotalLinesAdded, in.Cost.TotalLinesRemoved; a > 0 || d > 0 {
		diffSeg = p.ok + "+" + strconv.Itoa(a) + R + " " + p.alert + "-" + strconv.Itoa(d) + R
	}

	// gauge variants
	gaugeFull := func(label string, pct int, resetISO string) string {
		s := p.label + label + R + " " + p.miniBar(pct) + " " + p.threshold(pct) + strconv.Itoa(pct) + "%" + R
		if resetISO != "" {
			if dl := computeDelta(resetISO); dl != "" {
				s += " " + p.dim + glyphReset + " " + dl + R
			}
		}
		return s
	}
	gaugeLP := func(label string, pct int) string { // label + pct, no bar
		return p.label + label + R + " " + p.threshold(pct) + strconv.Itoa(pct) + "%" + R
	}

	// ===== Min tier: one line — branch · window% · 5h% =====
	if tier == tierMin {
		parts := []string{p.clay + bf + R, p.threshold(winPct) + strconv.Itoa(winPct) + "%" + R}
		if fiveOK {
			parts = append(parts, p.threshold(fiveP)+strconv.Itoa(fiveP)+"%"+R)
		}
		emit(join(parts...))
		return nil
	}

	// ===== Narrow tier: two lines =====
	if tier == tierNarrow {
		emit(join(p.clay+glyphBranch+" "+bf+R, diffSeg))
		g := []string{gaugeLP("window", winPct)}
		if fiveOK {
			g = append(g, gaugeLP("5h", fiveP))
		}
		if sevenOK {
			g = append(g, gaugeLP("7d", sevenP))
		}
		emit(join(g...))
		return nil
	}

	// ===== Full / Mid tiers: four lines =====
	full := tier == tierFull

	// --- Line 1: identity ---
	brand := p.sage + glyphSprout + " " + ansiBold + "LETS Workflow" + R
	if full {
		ver := version.Version
		if !version.IsDev() {
			ver = "v" + ver
		}
		brand += " " + p.dim + ver + R
	}
	pillSeg := ""
	if inWorktree(in, branch) {
		pillSeg = p.pillBg + p.label + " " + glyphWorktree + " worktree " + R
	}
	prSeg := ""
	if full && in.PR.Number > 0 {
		prSeg = p.label + glyphPR + " #" + strconv.Itoa(in.PR.Number) + R
		if in.PR.ReviewState != "" {
			prSeg += " " + p.prStateColor(in.PR.ReviewState) + in.PR.ReviewState + R
		}
	}
	emit(brand + marker + join(p.clay+glyphBranch+" "+bf+R, diffSeg, pillSeg, prSeg))

	// --- Line 2: task (dropped entirely if no active task) ---
	if id != "" {
		head := p.clay + glyphTask + " " + id + R
		if taskOK && title != "" {
			head += " " + p.text + title + R
		}
		noteSeg := ""
		if taskOK && notes > 0 {
			noteSeg = p.label + glyphNote + " " + strconv.Itoa(notes) + R
			if full {
				if age := relAgo(lastComment); age != "" {
					noteSeg += " " + p.dim + age + R
				}
			}
		}
		tail := noteSeg
		if full {
			hint := p.sage + glyphArrow + R + " " + p.dim + "/lets:note" + R
			if tail != "" {
				tail += " " + hint
			} else {
				tail = hint
			}
		}
		if tail != "" {
			head += segSep + tail
		}
		emit(head)
	}

	// --- Line 3: budget ---
	budget := p.gold + glyphModel + " " + ansiBold + in.Model.DisplayName + R
	if full && in.Effort.Level != "" {
		budget += " " + p.dim + in.Effort.Level + R
	}
	var gauges []string
	if full {
		gauges = append(gauges, gaugeFull("window", winPct, ""))
		if fiveOK {
			gauges = append(gauges, gaugeFull("5h", fiveP, fiveReset))
		}
		if sevenOK {
			gauges = append(gauges, gaugeFull("7d", sevenP, sevenReset))
		}
	} else {
		gauges = append(gauges, gaugeLP("window", winPct))
		if fiveOK {
			gauges = append(gauges, gaugeLP("5h", fiveP))
		}
		if sevenOK {
			gauges = append(gauges, gaugeLP("7d", sevenP))
		}
	}
	emit(budget + segSep + join(gauges...))

	// --- Line 4: rotating tip ---
	if t := tipOfMoment(time.Now()); t != "" {
		emit(p.sage + glyphTip + R + " " + p.dim + ansiItalic + t + R)
	}
	return nil
}
