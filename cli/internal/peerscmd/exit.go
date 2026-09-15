package peerscmd

// Exit codes of `lets peers`. A degraded source is exit 0 (ok=true, degraded[]).
const (
	ExitOK        = 0
	ExitGeneric   = 1
	ExitUsage     = 2
	ExitNotInRepo = 10
)
