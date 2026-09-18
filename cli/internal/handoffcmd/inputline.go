//go:build unix

package handoffcmd

import (
	"regexp"
	"strings"
)

// Input-line states a check before typing can establish.
const (
	lineReady    = "ready"     // idle agent, visibly empty input line
	lineBusy     = "busy"      // working, or a dialog is up: typing would queue or answer it
	lineNotClear = "not_clear" // text is already typed: the pointer would be appended to it
	lineUnknown  = "unknown"   // no recognizer for this agent, or no frame: not a refusal
)

// codexPlaceholders are what Codex shows in an empty composer
// (codex-rs/tui/src/chatwidget.rs PLACEHOLDER / SIDE_PLACEHOLDER).
var codexPlaceholders = []string{"Ask Codex to do anything", "Ask a follow-up question"}

// agentDialogs are prompts a typed Enter would answer, in any agent's tab: Claude's
// permission / trust / queue dialogs (peers' promptReady list), Antigravity's
// first-run workspace trust ("Do you trust the contents of this project?" over
// "> Yes, I trust this folder") and Codex's update prompt ("Press enter to
// continue") - lets-w5tm5 T0. A dialog is busy for every agent, known or not: the
// pointer plus Enter would answer it, and Antigravity's default answer is "trust".
var agentDialogs = []string{"Do you want to proceed?", "Do you want to make this edit", "trust the files", "Do you trust", "queued messages", "Yes, I trust this folder", "Press enter to continue"}

// claudeSpinner is Claude Code's working line (2.1.277, lets-w5tm5 T0): a glyph, a
// verb ending in "…", then " (" - "✶ Calculating… (13s · thinking)"; a finished
// turn reads "✻ Cooked for 7s". Older builds also print "esc to interrupt".
var claudeSpinner = regexp.MustCompile(`^\S\s+\S+… \(`)

// claudePlaceholder is the suggestion a fresh Claude composer shows (`Try "..."`).
func claudePlaceholder(rest string) bool {
	return strings.HasPrefix(rest, `Try "`) && strings.HasSuffix(rest, `"`)
}

func codexPlaceholder(rest string) bool {
	for _, p := range codexPlaceholders {
		if rest == p {
			return true
		}
	}
	return false
}

// inputLine classifies one agent's rendered frame. A dialog is busy for any agent.
// Beyond that, recognizers exist only for agents whose empty, typed and working
// frames were captured (codex, claude: lets-w5tm5 T0, fixtures in
// testdata/frames). Any other agent is unknown - Send still types into it and says
// so: Antigravity's prompt echoes its active mode (`/plan `), so "empty" cannot be
// read off its screen. A Claude draft never reaches the screen at all: Orca's draft
// field is that evidence (gate). Nothing here clears a line: there is no interrupt.
func inputLine(agent string, lines []string) string {
	if showsDialog(lines) {
		return lineBusy
	}
	switch agent {
	case "codex":
		return promptLine(lines, "›", codexPlaceholder, nil)
	case "claude":
		return promptLine(lines, "❯", claudePlaceholder, claudeSpinner)
	}
	return lineUnknown
}

// showsDialog reports whether any line carries a prompt from agentDialogs.
func showsDialog(lines []string) bool {
	for _, l := range lines {
		for _, d := range agentDialogs {
			if strings.Contains(l, d) {
				return true
			}
		}
	}
	return false
}

// promptLine finds the lowest line that starts with the agent's prompt glyph (the
// input line sits at the bottom; history above can repeat the glyph). Empty or a
// placeholder is ready; anything else was typed. "esc to interrupt" or a working
// line anywhere is busy.
func promptLine(lines []string, glyph string, placeholder func(string) bool, working *regexp.Regexp) string {
	for _, l := range lines {
		if strings.Contains(l, "esc to interrupt") || (working != nil && working.MatchString(l)) {
			return lineBusy
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		rest, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), glyph)
		if !ok {
			continue
		}
		rest = stripAnimation(rest)
		if rest == "" || placeholder(rest) {
			return lineReady
		}
		return lineNotClear
	}
	return lineUnknown
}

// stripAnimation drops Braille-pattern cells (U+2800-U+28FF), which Codex animates
// across its input line, and collapses the remaining whitespace (a no-break space
// included).
func stripAnimation(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 0x2800 && r <= 0x28FF {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
