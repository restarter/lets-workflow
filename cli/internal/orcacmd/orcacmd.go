//go:build unix

// Package orcacmd implements `lets orca` - a thin, optional wrapper around the Orca
// desktop app's CLI (stablyai/orca): open a task worktree through Orca, write a
// gate notification as the workspace card comment, and report whether Orca is
// usable. Orca is an opt-in addon: nothing here runs unless LETS_LAUNCHER=orca,
// an explicit `lets orca` command, or an explicit --probe-orca asks for it.
//
// Never hard-fails on Orca's side: an absent binary, an app that is not running,
// a verb this Orca build does not have, or unrecognized output all return OK=true
// with a named reason, so callers degrade to cmux / terminal. Only bad usage (and,
// for open, an invalid --repo) is a hard error.
//
// The Orca CLI is versioned with the app, so this package does not pin a
// capability table: it calls the verb and classifies the failure. Markdown that
// composes multi-verb Orca flows reads `orca skills get <name>` at runtime.
//
// Imports only leaves plus gitutil; never initcmd, peerscmd or worktreecmd.
package orcacmd

// SchemaVersion is the version of the JSON envelope emitted by `lets orca`.
// Per-package; guarded by TestResult_SchemaContract.
const SchemaVersion = 1
