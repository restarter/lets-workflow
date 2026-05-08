// Package initcmd implements the `lets init` subcommand.
package initcmd

import (
	"bytes"
	"fmt"
)

// Prefs holds the user-selected initialization preferences. The `/lets:init`
// slash command captures these via AskUserQuestion and passes as flags
// to the `lets init` subcommand.
type Prefs struct {
	Language    string // e.g. "English", "Ukrainian"
	MergeBranch string // e.g. "main", "develop"
	PRFlow      string // "local" | "github" | "bitbucket"
	Tracker     string // "beads" (canonical default; reserved for Linear/Jira)
	SkipBeads   bool
	ForceEnv    bool // if true, surgically update existing .env via UpdateEnvKeys
}

// AsValues returns the canonical Key.Name → Prefs field mapping.
//
// Single source of truth for Prefs↔Key wiring — both renderEnv (write fresh)
// and UpdateEnvKeys (surgical update) consume this. Adding a new LETS_* key
// requires adding ONE entry here (alongside the Keys entry in letsconfig and
// the Prefs field above) — no other map needs editing.
func (p Prefs) AsValues() map[string]string {
	return map[string]string{
		"LETS_LANGUAGE":     p.Language,
		"LETS_MERGE_BRANCH": p.MergeBranch,
		"LETS_PR_FLOW":      p.PRFlow,
		"LETS_TRACKER":      p.Tracker,
	}
}

// renderEnv produces the .lets/.env file body.
// Output is byte-for-byte equivalent to bash heredoc in init.sh:124-141.
// MUST match testdata/golden_env_*.txt exactly.
func renderEnv(p Prefs) []byte {
	var buf bytes.Buffer
	buf.WriteString("# LETS plugin config\n")
	buf.WriteString("# NOT FOR SECRETS. Contents are injected verbatim into model context every\n")
	buf.WriteString("# session (subject to whitelist filter in hooks/session-start.sh). Put\n")
	buf.WriteString("# tokens/passwords elsewhere (gh auth, OS keychain, .beads/.env).\n")
	buf.WriteString("\n")
	buf.WriteString("# Response language (English/Ukrainian/Italian/etc)\n")
	fmt.Fprintf(&buf, "LETS_LANGUAGE=%s\n", p.Language)
	buf.WriteString("\n")
	buf.WriteString("# Target branch for merges and PR base\n")
	fmt.Fprintf(&buf, "LETS_MERGE_BRANCH=%s\n", p.MergeBranch)
	buf.WriteString("\n")
	buf.WriteString("# PR flow: github | bitbucket | local\n")
	fmt.Fprintf(&buf, "LETS_PR_FLOW=%s\n", p.PRFlow)
	buf.WriteString("\n")
	buf.WriteString("# Task tracker (currently beads supported)\n")
	buf.WriteString("LETS_TRACKER=beads\n")
	return buf.Bytes()
}
