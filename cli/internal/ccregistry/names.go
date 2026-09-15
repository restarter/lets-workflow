// Package ccregistry reads Claude Code's local session registry
// (<claude config dir>/sessions/<pid>.json) and validates the names and session
// ids it carries. The registry format is internal to Claude Code: every entry is
// read under an explicit peerProtocol and anything else degrades by name.
//
// Leaf package: standard library and other leaves only.
package ccregistry

import (
	"regexp"

	"github.com/restarter/lets-workflow/cli/internal/peername"
)

// ValidName reports whether name is a usable peer name. It delegates to
// peername.Valid, the ONE name grammar; markdown passes raw values to Go and
// never validates a name itself.
func ValidName(name string) bool { return peername.Valid(name) }

var sessionRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidSession reports whether sid is a lowercase UUID (a Claude Code session id).
func ValidSession(sid string) bool { return sessionRe.MatchString(sid) }
