package peerscmd

// Exit codes of `lets peers`. A degraded source is exit 0 (ok=true, degraded[]).
const (
	ExitOK        = 0
	ExitGeneric   = 1
	ExitUsage     = 2
	ExitNotInRepo = 10
	// 11 was ExitNotDelivered (v0.10.0 `tell`); the stub never types, so it is retired.
)
