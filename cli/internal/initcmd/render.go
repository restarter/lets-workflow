// Package initcmd implements the `lets init` subcommand.
package initcmd

import (
	"bytes"
	"fmt"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
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

// renderEnv produces the .lets/.env file body using letsconfig.Header and
// per-key comments from letsconfig.Keys. Single source of truth — this,
// renderEnvExample, and UpdateEnvKeys all consume letsconfig.Keys.
//
// MUST match testdata/golden_env_*.txt exactly. Goldens are regenerated with
// `go test ./internal/initcmd -run TestRenderEnv_Golden -update`.
func renderEnv(p Prefs) []byte {
	return renderTemplate(letsconfig.Header, p.AsValues())
}

// renderEnvExample produces the .lets/.env.example body using canonical
// defaults from letsconfig.Keys. Replaces the deleted plugins/lets/hooks/config-template.env
// file. Single source of truth — adding a new key in letsconfig.Keys automatically
// updates the example.
func renderEnvExample() []byte {
	return renderTemplate(letsconfig.ExampleHeader, letsconfig.Defaults())
}

// renderTemplate is the shared template logic for renderEnv + renderEnvExample.
// Writes header, then for each Key: blank line, comment line, key=value.
func renderTemplate(header string, values map[string]string) []byte {
	var buf bytes.Buffer
	buf.WriteString(header)
	for _, k := range letsconfig.Keys {
		buf.WriteByte('\n')
		fmt.Fprintf(&buf, "# %s\n", k.Comment)
		fmt.Fprintf(&buf, "%s=%s\n", k.Name, values[k.Name])
	}
	return buf.Bytes()
}
