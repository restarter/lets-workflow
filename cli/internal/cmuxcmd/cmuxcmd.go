//go:build unix

// Package cmuxcmd implements `lets cmux` - a thin, optional launcher wrapper
// around the cmux terminal CLI (manaflow-ai/cmux). Opens a LETS worktree (or
// any directory) in a cmux workspace, typically running `claude '/lets:start <id>'`.
//
// cmux is macOS-only and strictly optional: when cmux is not on PATH (or the
// host is not macOS), Open never fails - it returns OK=true with
// Launch.Launched=false plus a FallbackCommand the caller renders as the
// manual new-terminal instruction.
//
// Mirrors the JSON-envelope + typed-exit-code structure of worktreecmd.
package cmuxcmd

// SchemaVersion is the version of the JSON envelope emitted by cmux
// subcommands. Per-package; guarded by TestResult_SchemaContract.
const SchemaVersion = 1
