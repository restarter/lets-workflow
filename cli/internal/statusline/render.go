package statusline

import (
	"fmt"
	"io"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// ANSI color codes - 24-bit RGB sequences mirroring bash exactly.
const (
	ansiReset      = "\033[0m"
	ansiBoldGold   = "\033[1;38;2;255;215;0m"  // LETS Workflow header
	ansiSepGold    = "\033[38;2;153;122;0m"    // separator (»)
	ansiBranch     = "\033[38;2;232;160;144m"  // branch / folder (peach)
	ansiBoldOrange = "\033[1;38;2;255;175;50m" // model
	ansiTan        = "\033[38;2;190;176;140m"  // context window (tan)
	ansiTanDim     = "\033[2;38;2;190;176;140m"
	ansiGray       = "\033[90m"               // mid-dot separator
	ansiGreen      = "\033[38;2;130;200;130m" // usage <50%
	ansiYellow     = "\033[38;2;255;200;80m"  // usage 50-80%
	ansiRed        = "\033[38;2;255;100;100m" // usage >=80%
)

const (
	separatorMidDot = " \xc2\xb7 "       // " · " UTF-8
	separatorAngle  = " \xc2\xbb "       // " » " UTF-8
	leafEmoji       = "\xf0\x9f\x8c\xb1" // 🌱 UTF-8
)

// renderLines writes the 2-line statusline output.
//
// Line 1: 🌱 LETS Workflow vX.Y.Z » branch-or-folder
// Line 2: model » window N% (Nk/Nk) [· 5h N% (reset)] [· 7d N% (reset)]
func renderLines(w io.Writer, in Input, branch, folder string, u usage) error {
	sep := ansiSepGold + separatorAngle + ansiReset

	// Line 1 — `v` prefix elided for untagged dev builds (renders as
	// "LETS Workflow dev" instead of awkward "LETS Workflow vdev").
	versionDisplay := version.Version
	if !version.IsDev() {
		versionDisplay = "v" + versionDisplay
	}
	fmt.Fprintf(w, "%s %sLETS Workflow %s%s", leafEmoji, ansiBoldGold, versionDisplay, ansiReset)
	fmt.Fprint(w, sep)
	branchOrFolder := branch
	if branchOrFolder == "" {
		branchOrFolder = folder
	}
	fmt.Fprintf(w, "%s%s%s", ansiBranch, branchOrFolder, ansiReset)
	fmt.Fprintln(w)

	// Line 2
	fmt.Fprintf(w, "%s%s%s", ansiBoldOrange, in.Model.DisplayName, ansiReset)
	fmt.Fprint(w, sep)

	if pct := in.ContextWindow.UsedPercentage; pct > 0 {
		fmt.Fprintf(w, "%swindow %d%%%s", ansiTan, int(pct+0.5), ansiReset)
		used := in.ContextWindow.CurrentUsage.CacheReadInputTokens +
			in.ContextWindow.CurrentUsage.CacheCreationInputTokens +
			in.ContextWindow.CurrentUsage.InputTokens +
			in.ContextWindow.CurrentUsage.OutputTokens
		total := in.ContextWindow.ContextWindowSize
		if used > 0 && total > 0 {
			fmt.Fprintf(w, " %s(%dk/%dk)%s", ansiTanDim, used/1000, total/1000, ansiReset)
		}
		if u.fiveHourOK || u.sevenDayOK {
			fmt.Fprintf(w, "%s%s%s", ansiGray, separatorMidDot, ansiReset)
		}
	}

	if u.fiveHourOK {
		fmt.Fprintf(w, "%s5h %d%%%s", usageColor(u.fiveHour), u.fiveHour, ansiReset)
		if delta := computeDelta(u.fiveHourReset); delta != "" {
			fmt.Fprintf(w, " %s(%s)%s", ansiTanDim, delta, ansiReset)
		}
		if u.sevenDayOK {
			fmt.Fprintf(w, "%s%s%s", ansiGray, separatorMidDot, ansiReset)
		}
	}

	if u.sevenDayOK {
		fmt.Fprintf(w, "%s7d %d%%%s", usageColor(u.sevenDay), u.sevenDay, ansiReset)
		if delta := computeDelta(u.sevenDayReset); delta != "" {
			fmt.Fprintf(w, " %s(%s)%s", ansiTanDim, delta, ansiReset)
		}
	}

	return nil
}

// usageColor returns the ANSI prefix for a percentage utilization value.
func usageColor(pct int) string {
	switch {
	case pct >= 80:
		return ansiRed
	case pct >= 50:
		return ansiYellow
	default:
		return ansiGreen
	}
}
