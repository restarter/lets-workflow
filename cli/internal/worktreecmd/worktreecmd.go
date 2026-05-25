//go:build unix

// Package worktreecmd implements the `lets worktree` subcommand:
// create/remove/list/info operations on git worktrees with LETS-managed
// symlinks for .lets/ and .beads/.env, atomic operations with rollback,
// and a JSON envelope output for scripted callers.
//
// Mirrors the structure of cli/internal/initcmd/ and cli/internal/updatecmd/.
//
// Build constraint: unix-only because syscall.Flock is used for .gitignore
// concurrency control. Windows support is a future task (see lets-rqep4 backlog).
package worktreecmd

// SchemaVersion is the version of the JSON envelope emitted by all
// subcommands in this package. Bump on field removal or semantic change
// of an existing field; additions are minor.
const SchemaVersion = 1
