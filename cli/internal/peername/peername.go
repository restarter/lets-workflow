// Package peername is the ONE grammar for a peer session name (a Claude Code
// `/rename` name used as a peer address, an orchestrator name, an `orc:` binding).
// ccregistry.ValidName and the task-state `orc:` validation delegate here.
//
// Leaf package: standard library only.
package peername

import (
	"regexp"
	"unicode/utf8"
)

// MaxRunes bounds a name; longer names are refused, never truncated.
const MaxRunes = 64

var nameRe = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._-]*( [\p{L}\p{N}][\p{L}\p{N}._-]*)*$`)

// Valid reports whether name is words of letters, digits, `.`, `_` and `-` (each
// starting with a letter or digit) separated by single spaces, at most MaxRunes
// runes. It refuses anything a shell, a header or a flag parser could misread:
// quotes, `$`, backticks, newlines, a leading `-`, double spaces.
func Valid(name string) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) <= MaxRunes && nameRe.MatchString(name)
}
