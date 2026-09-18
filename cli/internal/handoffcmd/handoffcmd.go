//go:build unix

// Package handoffcmd implements `lets handoff` - delivering a hand-off brief to an
// agent and bringing its final report back: `targets` lists the agent terminals of
// this checkout, `send` types a one-line pointer to the brief into one of them,
// `codex` runs the brief through Codex headless, `await` waits for the report of a
// brief sent to a tab (Codex: its rollout; any other agent: the report-file
// contract).
//
// The handoff lane, not the peer lane: a brief goes to a tool, never to a LETS
// session's peer inbox, so this package never imports peerscmd (TestDeniedImports).
// Orca and Codex are optional: an absent binary or app is ok=true with a named
// reason. Only bad usage and an invalid brief are hard errors.
package handoffcmd

// SchemaVersion is the version of the `lets handoff` JSON envelope (per-package,
// guarded by TestResult_SchemaContract).
const SchemaVersion = 1
