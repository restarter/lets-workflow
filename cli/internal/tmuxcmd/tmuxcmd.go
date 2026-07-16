//go:build unix

// Package tmuxcmd implements `lets tmux` - a thin, optional launcher wrapper
// around the tmux CLI. Opens a LETS worktree (or any directory) in a tmux
// window/session, typically running `claude '/lets:start <id>'`.
//
// tmux is cross-platform on unix (Linux + macOS) and strictly optional: when
// tmux is not on PATH, Open never fails - it returns OK=true with
// Launch.Launched=false plus a FallbackCommand the caller renders as the manual
// new-terminal instruction. Windows is served by the !unix stub in cli/.
//
// Mirrors the JSON-envelope + typed-exit-code structure of cmuxcmd/worktreecmd.
// Keep in sync with cli/internal/cmuxcmd - the two launchers are deliberate
// siblings, not a shared abstraction (see the plan's Non-Goals).
package tmuxcmd

// SchemaVersion is the version of the JSON envelope emitted by tmux
// subcommands. Per-package; guarded by TestResult_SchemaContract.
const SchemaVersion = 1
