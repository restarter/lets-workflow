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

// claudeDialogs are prompts a typed Enter would answer (peers' promptReady list).
var claudeDialogs = []string{"Do you want to proceed?", "Do you want to make this edit", "trust the files", "Do you trust", "queued messages"}

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

// inputLine classifies one agent's rendered frame. Recognizers exist only for
// agents whose empty, typed and working frames were captured (codex, claude:
// lets-w5tm5 T0, fixtures in testdata/frames). Any other agent is unknown - Send
// still types into it and says so: Antigravity's prompt echoes its active mode
// (`/plan `), so "empty" cannot be read off its screen. A Claude draft never
// reaches the screen at all: Orca's draft field is that evidence (gate). Nothing
// here clears a line: there is no interrupt.
func inputLine(agent string, lines []string) string {
	switch agent {
	case "codex":
		return promptLine(lines, "›", codexPlaceholder, nil, nil)
	case "claude":
		return promptLine(lines, "❯", claudePlaceholder, claudeDialogs, claudeSpinner)
	}
	return lineUnknown
}

// promptLine finds the lowest line that starts with the agent's prompt glyph (the
// input line sits at the bottom; history above can repeat the glyph). Empty or a
// placeholder is ready; anything else was typed. "esc to interrupt", a working
// line or a dialog anywhere is busy.
func promptLine(lines []string, glyph string, placeholder func(string) bool, dialogs []string, working *regexp.Regexp) string {
	for _, l := range lines {
		if strings.Contains(l, "esc to interrupt") || (working != nil && working.MatchString(l)) {
			return lineBusy
		}
		for _, d := range dialogs {
			if strings.Contains(l, d) {
				return lineBusy
			}
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
