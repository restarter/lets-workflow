//go:build unix

package handoffcmd

// Exit codes for `lets handoff`. Orca or Codex being unavailable is NOT an error
// (a named reason, exit 0); only bad usage and an invalid brief are.
const (
	ExitOK           = 0
	ExitGeneric      = 1
	ExitUsage        = 2
	ExitBriefInvalid = 10 // --brief is not a brief artifact-path wrote
)
