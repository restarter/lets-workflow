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

// rich.go — PROTOTYPE rich statusline (lets-ds6bc), variant "B + task line".
//
// 4 lines, high-signal only (the kitchen-sink dump was trimmed):
//   1: 🌱 LETS vX » branch [worktree] · +/-diff
//   2: Task » task-id (title) · 📝 notes (last-comment ago) · → /lets:note   (cheap cache read, no bd)
//   3: model ·effort » ctx N% (toks) · 5h N% (reset) · 7d N% (reset)
//   4: Tip » rotating workflow hint (loading-screen style, time-bucketed)
//
// Gated behind LETS_STATUSLINE_LEVEL=rich; the default compact path
// (renderLines, render.go) is untouched and frozen. NOT final — still iterating.

// detectWidth reads terminal width from the COLUMNS env var Claude Code sets
// (CC >= 2.1.153); falls back to 80 when absent/unparseable.
func detectWidth() int {
	if c := strings.TrimSpace(os.Getenv("COLUMNS")); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n >= 20 && n <= 400 {
			return n
		}
	}
	return 80
}

var ansiSGRRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiSGRRe.ReplaceAllString(s, "") }
func visibleWidth(s string) int { return len([]rune(stripANSI(s))) }

// middleEllipsis trims to a visible-width budget. PROTOTYPE caveat: on overflow
// it falls back to plain (ANSI-stripped) text. Lines fit on a normal terminal.
func middleEllipsis(s string, max int) string {
	if max <= 1 {
		return s
	}
	plain := []rune(stripANSI(s))
	if len(plain) <= max {
		return s
	}
	keep := max - 1
	left := keep / 2
	right := keep - left
	return string(plain[:left]) + "…" + string(plain[len(plain)-right:]) + ansiReset
}

type rgb struct{ r, g, b int }

func lerp(a, b rgb, t float64) rgb {
	f := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t) }
	return rgb{f(a.r, b.r), f(a.g, b.g), f(a.b, b.b)}
}

// gradientAt maps t in [0,1] across green->yellow->red, pinned to the usageColor
// thresholds (0.5 yellow, 0.8 red) so the bar agrees with the numeric %.
func gradientAt(t float64) rgb {
	green, yellow, red := rgb{130, 200, 130}, rgb{255, 200, 80}, rgb{255, 100, 100}
	switch {
	case t < 0.5:
		return lerp(green, yellow, t/0.5)
	case t < 0.8:
		return lerp(yellow, red, (t-0.5)/0.3)
	default:
		return red
	}
}

func fgRGB(c rgb) string { return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.r, c.g, c.b) }

func gradientBar(pct float64, width int) string {
	if width < 4 {
		width = 4
	}
	filled := int(pct/100*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		t := float64(i) / float64(width-1)
		if i < filled {
			b.WriteString(fgRGB(gradientAt(t)))
			b.WriteString("█")
		} else {
			b.WriteString(ansiTanDim)
			b.WriteString("░")
		}
	}
	b.WriteString(ansiReset)
	return b.String()
}

var taskIDRe = regexp.MustCompile(`[a-z][a-z0-9]*-[a-z0-9]+(?:\.[0-9]+)?`)

// taskIDFromBranch extracts a beads task id from the branch name, mirroring the
// detect-task SKILL pattern <prefix>-<alphanum>[.N]. Free — no bd call.
func taskIDFromBranch(branch string) string {
	b := strings.TrimPrefix(branch, "feature/")
	b = strings.TrimPrefix(b, "worktree-")
	return taskIDRe.FindString(b)
}

func branchKind(branch, merge string) string {
	switch {
	case branch == "":
		return "none"
	case branch == merge:
		return "merge"
	case strings.HasPrefix(branch, "worktree-"):
		return "worktree"
	case strings.HasPrefix(branch, "feature/"):
		return "feature"
	default:
		return "other"
	}
}

func phaseHint(kind string, hasTask bool) string {
	switch kind {
	case "merge", "none":
		return "/lets:start"
	case "worktree":
		return "/lets:done"
	case "feature":
		return "/lets:check"
	default:
		if hasTask {
			return "/lets:check"
		}
		return "/lets:start"
	}
}

func ktok(n int) string { return strconv.Itoa(n/1000) + "k" }

// relAgo renders a past ISO timestamp as a compact "N min ago" / "Nh ago" /
// "Nd ago". Empty for blank/invalid/future input. Mirrors computeDelta's parse
// but counts backwards (stdlib time.Parse — no extra dep, Windows-safe).
func relAgo(iso string) string {
	if iso == "" {
		return ""
	}
	s := iso
	if i := strings.IndexByte(s, '.'); i != -1 {
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		s = s[:i] + s[j:]
	}
	s = strings.TrimSuffix(s, "Z")
	if len(s) >= 6 && (s[len(s)-6] == '+' || s[len(s)-6] == '-') && s[len(s)-3] == ':' {
		s = s[:len(s)-6]
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.UTC)
	if err != nil {
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

// readTaskStatus reads a cheap on-change cache (PROTOTYPE: written by hand;
// real wiring = /lets:start /lets:done /lets:note). Single line:
//
//	<task-id>|<status>|<title>|<note-count>|<last-comment-iso>
//
// This is the per-render-cheap alternative to a bd network call. Returns ok=false
// (graceful degrade) when the file is missing or the cached task != active task.
func readTaskStatus(cacheDir, taskID string) (status, title string, notes int, lastComment string, ok bool) {
	b, err := os.ReadFile(filepath.Join(cacheDir, "task-status"))
	if err != nil {
		return
	}
	parts := strings.SplitN(strings.TrimSpace(string(b)), "|", 5)
	if len(parts) < 2 || parts[0] != taskID {
		return
	}
	status = parts[1]
	if len(parts) > 2 {
		title = parts[2]
	}
	if len(parts) > 3 {
		notes, _ = strconv.Atoi(parts[3])
	}
	if len(parts) > 4 {
		lastComment = parts[4]
	}
	ok = true
	return
}

// gitBranchIcon is the Powerline/Nerd Font git-branch glyph (needs a Nerd Font;
// swap to "⎇" (⎇) if your terminal shows tofu). worktreeMark flags a worktree.
const gitBranchIcon = ""
// wtBadge is a "worktree" label shown right after the branch when inside a
// worktree — a slate pill (cool bg, readable fg) so it stays legible without
// competing with the branch name.
const wtBadge = "\033[48;2;56;59;68m\033[38;2;168;174;188m worktree \033[0m"

// agoBadge renders the last-comment age as dim italic grey in parens — no
// background block; reads as passive, disabled-looking metadata.
func agoBadge(text string) string {
	return "\033[2;3;38;2;140;140;145m(" + text + ")\033[0m"
}

// inWorktree reports whether we're inside a git worktree, cheaply: prefer the
// stdin signals (workspace.git_worktree / worktree.name), fall back to the
// branch-name convention. No git fork.
func inWorktree(in Input, branch string) bool {
	return in.Workspace.GitWorktree || in.Worktree.Name != "" || strings.HasPrefix(branch, "worktree-")
}

// ansiTip styles the rotating hint line: dim italic grey, reads as a quote.
const ansiTip = "\033[2;3;38;2;150;150;155m"

// Per-line label colors (Task / Model / Tip) — distinct muted hues so the
// left-edge label reads as a colour-coded category, not data.
const (
	ansiLabelTask  = "\033[38;2;140;185;150m" // muted green
	ansiLabelModel = "\033[38;2;185;155;215m" // muted violet
	ansiLabelTip   = "\033[38;2;140;170;205m" // steel blue
)

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

// renderRich draws the 4-line "B + task line" layout. Each segment is omitted
// when empty; each line is width-ellipsised. Never calls bd/network per render.
func renderRich(w io.Writer, in Input, branch, folder string, u usage, width int, cacheDir string) error {
	const reset = ansiReset
	sep := ansiSepGold + separatorAngle + reset
	mid := ansiGray + separatorMidDot + reset

	emit := func(line string) {
		if visibleWidth(line) == 0 {
			return
		}
		fmt.Fprintln(w, middleEllipsis(line, width))
	}
	seg := func(parts ...string) string {
		out := parts[:0]
		for _, p := range parts {
			if visibleWidth(p) > 0 {
				out = append(out, p)
			}
		}
		return strings.Join(out, mid)
	}

	// --- Line 1: header + branch ---
	verCLI := version.Version
	if !version.IsDev() {
		verCLI = "v" + verCLI
	}
	bf := branch
	if bf == "" {
		bf = folder
	}
	if inWorktree(in, branch) {
		bf = strings.TrimPrefix(bf, "worktree-") // the worktree pill already says it
	}
	l1 := fmt.Sprintf("%s %sLETS Workflow %s%s", leafEmoji, ansiBoldGold, verCLI, reset)
	l1 += sep + ansiBranch + gitBranchIcon + " " + bf + reset
	var l1r []string
	if a, d := in.Cost.TotalLinesAdded, in.Cost.TotalLinesRemoved; a > 0 || d > 0 {
		l1r = append(l1r, ansiGreen+fmt.Sprintf("+%d", a)+reset+ansiTanDim+"/"+reset+ansiRed+fmt.Sprintf("-%d", d)+reset)
	}
	if inWorktree(in, branch) {
		l1r = append(l1r, wtBadge)
	}
	if r := seg(l1r...); r != "" {
		l1 += mid + r
	}
	emit(l1)

	// --- Line 2: active task status + notes + /lets:note (cheap cache read) ---
	id := taskIDFromBranch(branch)
	if id != "" {
		_, title, notes, lastComment, ok := readTaskStatus(cacheDir, id)
		l2 := ansiLabelTask + "Task " + reset + sep + ansiBranch + id + reset
		if ok && title != "" {
			l2 += " " + ansiTanDim + "(" + title + ")" + reset
		}
		var l2r []string
		if ok && notes > 0 {
			notesSeg := ansiTanDim + fmt.Sprintf("📝 %d", notes) + reset
			if ago := relAgo(lastComment); ago != "" {
				notesSeg += " " + agoBadge(ago)
			}
			l2r = append(l2r, notesSeg)
		}
		l2r = append(l2r, ansiTanDim+"→ /lets:note"+reset)
		if r := seg(l2r...); r != "" {
			l2 += mid + r
		}
		emit(l2)
	}

	// --- Line 3: Model » name · effort · ctx N% (toks) · 5h · 7d ---
	l3 := ansiLabelModel + "Model" + reset + sep + ansiBoldOrange + in.Model.DisplayName + reset
	var l3r []string
	if in.Effort.Level != "" {
		l3r = append(l3r, ansiTanDim+in.Effort.Level+reset)
	}
	if pct := in.ContextWindow.UsedPercentage; pct > 0 {
		ctx := ansiTan + fmt.Sprintf("window %d%%", int(pct+0.5)) + reset
		used := in.ContextWindow.CurrentUsage.CacheReadInputTokens +
			in.ContextWindow.CurrentUsage.CacheCreationInputTokens +
			in.ContextWindow.CurrentUsage.InputTokens +
			in.ContextWindow.CurrentUsage.OutputTokens
		if used > 0 && in.ContextWindow.ContextWindowSize > 0 {
			ctx += " " + ansiTanDim + "(" + ktok(used) + "/" + ktok(in.ContextWindow.ContextWindowSize) + ")" + reset
		}
		l3r = append(l3r, ctx)
	}
	fiveP, fiveReset, fiveOK := 0, "", false
	if in.RateLimits.FiveHour.UsedPercentage > 0 {
		fiveP, fiveReset, fiveOK = int(in.RateLimits.FiveHour.UsedPercentage+0.5), in.RateLimits.FiveHour.ResetsAt, true
	} else if u.fiveHourOK {
		fiveP, fiveReset, fiveOK = u.fiveHour, u.fiveHourReset, true
	}
	if fiveOK {
		s := usageColor(fiveP) + fmt.Sprintf("5h %d%%", fiveP) + reset
		if d := computeDelta(fiveReset); d != "" {
			s += " " + ansiTanDim + "(" + d + ")" + reset
		}
		l3r = append(l3r, s)
	}
	sevenP, sevenReset, sevenOK := 0, "", false
	if in.RateLimits.SevenDay.UsedPercentage > 0 {
		sevenP, sevenReset, sevenOK = int(in.RateLimits.SevenDay.UsedPercentage+0.5), in.RateLimits.SevenDay.ResetsAt, true
	} else if u.sevenDayOK {
		sevenP, sevenReset, sevenOK = u.sevenDay, u.sevenDayReset, true
	}
	if sevenOK {
		s := usageColor(sevenP) + fmt.Sprintf("7d %d%%", sevenP) + reset
		if d := computeDelta(sevenReset); d != "" {
			s += " " + ansiTanDim + "(" + d + ")" + reset
		}
		l3r = append(l3r, s)
	}
	if r := seg(l3r...); r != "" {
		l3 += mid + r
	}
	emit(l3)

	// --- Line 4: rotating workflow tip (loading-screen style) ---
	if t := tipOfMoment(time.Now()); t != "" {
		label := ansiLabelTip + "Tip  " + reset + sep
		body := t
		if avail := width - visibleWidth(label); avail > 1 && len([]rune(body)) > avail {
			body = string([]rune(body)[:avail-1]) + "…"
		}
		fmt.Fprintln(w, label+ansiTip+body+reset)
	}

	return nil
}
