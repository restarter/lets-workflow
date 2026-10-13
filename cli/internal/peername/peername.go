// Package peername is the ONE grammar for a peer session name (a Claude Code
// `/rename` name used as a peer address, an orchestrator name, an `orc:` binding).
// ccregistry.ValidName and the task-state `orc:` validation delegate here.
//
// Leaf package: standard library only.
package peername

import (
	"path/filepath"
	"regexp"
	"strings"
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

// RepoSlug is the <repo> part of every role name: the main checkout's basename,
// lowercased, each rune outside [a-z0-9-] mapped to '-', at most MaxRunes runes.
// "" when nothing usable remains.
func RepoSlug(mainRoot string) string {
	base := strings.ToLower(filepath.Base(filepath.Clean(mainRoot)))
	var b strings.Builder
	n := 0
	for _, r := range base {
		if n == MaxRunes {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
		n++
	}
	s := strings.Trim(b.String(), "-")
	if s == "" || s == "." {
		return ""
	}
	return s
}

// Orchestrator is the orchestrator name of repo: <repo>-orc, or <repo>-orc-<scope>.
func Orchestrator(repo, scope string) string {
	if scope == "" {
		return repo + "-orc"
	}
	return repo + "-orc-" + scope
}

// ParseOrchestrator reports whether name is an orchestrator name of repo and returns
// its scope ("" for the plain <repo>-orc). Case-insensitive on the repo part only.
func ParseOrchestrator(repo, name string) (scope string, ok bool) {
	if repo == "" || !Valid(name) {
		return "", false
	}
	prefix := repo + "-orc"
	if len(name) < len(prefix) || !strings.EqualFold(name[:len(prefix)], prefix) {
		return "", false
	}
	rest := name[len(prefix):]
	switch {
	case rest == "":
		return "", true
	case strings.HasPrefix(rest, "-") && len(rest) > 1 && !strings.Contains(rest, " "):
		return rest[1:], true
	}
	return "", false
}

// Worker is the run-worker name of a task: <repo>-<task-id>.
func Worker(repo, taskID string) string { return repo + "-" + taskID }

// Lead is a standing team's lead session name, <repo>-<callsign>-lead; the caller
// checks Valid.
func Lead(repo, callsign string) string { return repo + "-" + callsign + "-lead" }
