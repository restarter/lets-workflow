package peerscmd

import (
	"regexp"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// Header is the parsed `[lets-peer ...]` line that opens every peer message.
// Names are display only; addressing compares session ids exactly.
type Header struct {
	ID      string // 16 hex chars
	Kind    string // ask | ping | tell | ask-ro
	FromSID string
	ToSID   string
	From    string // "<role>/<live name>"
	To      string // "<live name>"
}

var (
	msgIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)
	// crossWrapper is Claude Code's wrapper around a delivered cross-session message.
	// A SendMessage delivered to another session lands in its transcript as a user
	// text that opens with ONE exact prefix line before the wrapper (lets-rry3c,
	// recorded live 2026-09-25); the prefix is accepted only when the wrapper follows.
	crossWrapper = regexp.MustCompile(`^\s*(?:Another Claude session sent a message:\s*)?<cross-session-message[^>]*>\s*`)
)

// ValidKind reports whether k is a peer message kind.
func ValidKind(k string) bool {
	switch k {
	case "ask", "ping", "tell", "ask-ro":
		return true
	}
	return false
}

// ParseHeader is the ONE header parser: line must START with `[lets-peer `, fields
// are `key=value` / `key="value"` tokens up to the closing `]`, and every field is
// validated. A header quoted later inside text is not a header.
func ParseHeader(line string) (Header, bool) {
	rest, ok := strings.CutPrefix(line, "[lets-peer ")
	if !ok {
		return Header{}, false
	}
	fields := map[string]string{}
	for {
		rest = strings.TrimLeft(rest, " ")
		if strings.HasPrefix(rest, "]") {
			break
		}
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			return Header{}, false
		}
		key := rest[:eq]
		if strings.ContainsAny(key, " ]\"") {
			return Header{}, false
		}
		rest = rest[eq+1:]
		var val string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return Header{}, false
			}
			val, rest = rest[1:1+end], rest[2+end:]
		} else {
			end := strings.IndexAny(rest, " ]")
			if end < 0 {
				return Header{}, false
			}
			val, rest = rest[:end], rest[end:]
		}
		if _, dup := fields[key]; dup {
			return Header{}, false
		}
		fields[key] = val
	}
	h := Header{ID: fields["id"], Kind: fields["kind"], FromSID: fields["from_sid"], ToSID: fields["to_sid"], From: fields["from"], To: fields["to"]}
	if !msgIDRe.MatchString(h.ID) || !ValidKind(h.Kind) || !ccregistry.ValidSession(h.FromSID) || !ccregistry.ValidSession(h.ToSID) {
		return Header{}, false
	}
	return h, true
}

// leadingHeader parses a header at the very start of a message body, after an
// optional Claude Code `<cross-session-message ...>` wrapper - itself optionally
// preceded by Claude Code's delivery prefix "Another Claude session sent a
// message:". Any other text before the header, or the prefix without the wrapper,
// is not a header.
func leadingHeader(text string) (Header, bool) {
	text = crossWrapper.ReplaceAllString(text, "")
	line, _, _ := strings.Cut(strings.TrimLeft(text, " \t\r\n"), "\n")
	return ParseHeader(line)
}
