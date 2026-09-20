package peerscmd

// Exit codes of `lets peers`. A degraded source is exit 0 (ok=true, degraded[]).
const (
	ExitOK        = 0
	ExitGeneric   = 1
	ExitUsage     = 2
	ExitNotInRepo = 10
	// ExitNotDelivered: `tell` ran and the envelope is authoritative, but delivered
	// is false - the way worktreecmd types its own refusals (ExitDirtyWorktree = 14,
	// ExitUnpushedCommits = 21), so `lets peers tell && echo sent` cannot lie.
	ExitNotDelivered = 11
)
