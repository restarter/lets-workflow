//go:build !unix

package cli

import "github.com/restarter/lets-workflow/cli/internal/taskstate"

// selfHealFn is a no-op off unix: `lets worktree adopt` needs flock and POSIX
// symlinks (the worktree.go / worktree_stub.go split).
var selfHealFn = func(root, rulesPath string) string { return "" }

// peersHealFn is a no-op off unix: peer roles need flock (peerscmd is unix-only).
var peersHealFn = func(root, sid string) {}

// sessionGuardFn returns the zero guard off unix (no session registry reader, no
// flock): the unguarded session-boundary refresh.
var sessionGuardFn = func(root, sid string) (taskstate.SessionGuard, func()) { return taskstate.SessionGuard{}, nil }
