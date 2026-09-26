//go:build unix

package orcacmd

// Exit codes for `lets orca`. Orca unavailability is NOT an error (graceful
// fallback, exit 0). Only bad usage and an invalid --repo are hard errors.
const (
	ExitOK          = 0
	ExitGeneric     = 1
	ExitUsage       = 2
	ExitRepoInvalid = 10 // --repo is not a directory or not a main checkout

	// team-create / team-remove break the never-hard-fail convention on purpose: they
	// fire only after a create or remove was attempted, or when a safety net refuses.
	// Orca being unavailable stays exit 0 (state not_attempted).
	ExitCreateAmbiguous      = 11 // team-create: after the create, Orca and git disagree
	ExitCreateFailed         = 12 // team-create: neither Orca nor git shows the worktree
	ExitTeamWorktreeMismatch = 13 // team-create: the created worktree's name, branch or HEAD is not the expected one
	ExitAgentTerminalPresent = 14 // team-create: an agent already runs in the new worktree
	ExitDirtyWorktree        = 15 // team-remove: uncommitted or untracked changes
	ExitUnpushedCommits      = 16 // team-remove: HEAD is not proven on a remote
	ExitArchiveHookFailed    = 17 // team-remove: Orca's archive hook failed; the worktree stays
	ExitRemoveAmbiguous      = 18 // team-remove: after the remove, Orca and git disagree
	ExitNotListed            = 19 // team-remove: Orca lists no worktree of that name (a Go-created one)
)
