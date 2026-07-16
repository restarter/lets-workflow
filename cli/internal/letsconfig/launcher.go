package letsconfig

// ShippedLaunchers is the set of accepted LETS_LAUNCHER values. Every entry
// except "terminal" MUST have a matching `lets <name>` cobra subcommand
// registered in cli/internal/cli/root.go - that invariant is pinned in both
// directions by TestShippedLaunchers_MatchSubcommands. "terminal" is the
// no-subcommand launcher: the caller just prints a `cd … && claude` line.
//
// The tracker platform's shippedTrackers is the sibling of this list; there the
// whitelist is pinned to plugins/lets/rules/tracker-*.md files on disk. A
// launcher has no adapter file (it IS a Go package), so the pin targets the
// registered subcommand instead. Same contract: a value the binary cannot serve
// must never be accepted.
var ShippedLaunchers = []string{"terminal", "cmux", "tmux"}

// ValidLauncher reports whether s is an accepted LETS_LAUNCHER value.
// The empty string is NOT valid here - callers treat "" as "not supplied" and
// fall back to Defaults()["LETS_LAUNCHER"] before validating.
func ValidLauncher(s string) bool {
	for _, l := range ShippedLaunchers {
		if l == s {
			return true
		}
	}
	return false
}
