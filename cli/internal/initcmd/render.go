// Package initcmd implements the `lets init` subcommand.
package initcmd

import (
	"bytes"
	"fmt"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

// Prefs holds the user-selected initialization preferences. The `/lets:init`
// slash command captures these via AskUserQuestion and passes as flags
// to the `lets init` subcommand.
type Prefs struct {
	Language    string // e.g. "English", "Ukrainian"
	MergeBranch string // e.g. "main", "develop"
	PRFlow      string // "local" | "github" | "bitbucket"
	Tracker     string // adapter name: "beads" (default) | "none"
	Launcher    string // "terminal" (default) | "cmux"
	RulesScope  string // "project" (own .claude/rules copy) | "user" (rely on ~/.claude/rules) | "" = preserve-or-default
	SkipBeads   bool
	// TrackerExplicit records whether --tracker was passed on the command line
	// (cobra Changed). Transient signal, NOT a config key (excluded from
	// AsValues, like SkipBeads): Step 8b warns when an explicit --tracker is
	// overridden by an existing .env value on re-init.
	TrackerExplicit bool
}

// AsValues returns the canonical Key.Name → Prefs field mapping.
//
// Single source of truth for Prefs↔Key wiring — both renderEnv (write fresh)
// and RegenerateEnv (canonical writer) consume this. Adding a new LETS_* key
// requires adding ONE entry here (alongside the Keys entry in letsconfig and
// the Prefs field above) — no other map needs editing.
func (p Prefs) AsValues() map[string]string {
	return map[string]string{
		"LETS_LANGUAGE":     p.Language,
		"LETS_MERGE_BRANCH": p.MergeBranch,
		"LETS_PR_FLOW":      p.PRFlow,
		"LETS_TRACKER":      p.Tracker,
		"LETS_LAUNCHER":     p.Launcher,
		"LETS_RULES_SCOPE":  p.RulesScope,
	}
}

// renderEnv produces the .lets/.env file body using letsconfig.Header and
// per-key comments from letsconfig.Keys. Single source of truth — this,
// renderEnvExample, and RegenerateEnv all consume letsconfig.Keys.
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
// Writes header, then LETS_ENV_VERSION metadata block, then for each Key:
// blank line, comment line, key=value.
//
// LETS_ENV_VERSION is always derived from version.Version so both renderEnv
// (Prefs) and renderEnvExample (Defaults) get the same canonical marker for
// free without each caller having to inject it.
func renderTemplate(header string, values map[string]string) []byte {
	return renderTemplateKeys(header, letsconfig.Keys, values)
}

// renderTemplateKeys is the key-list-parametric core shared by the project
// renderers (all Keys) and renderUserEnv (the UserKeys subset).
func renderTemplateKeys(header string, keys []letsconfig.Key, values map[string]string) []byte {
	var buf bytes.Buffer
	buf.WriteString(header)
	// Version marker - first key, with its own comment block.
	// Single \n after the value matches the existing key-block style; the loop
	// below emits its own blank-line separator before each key.
	buf.WriteString("\n# Managed by lets - do not edit\n")
	fmt.Fprintf(&buf, "%s=%s\n", letsconfig.VersionKeyName, version.Version)
	for _, k := range keys {
		buf.WriteByte('\n')
		fmt.Fprintf(&buf, "# %s\n", k.Comment)
		fmt.Fprintf(&buf, "%s=%s\n", k.Name, values[k.Name])
	}
	return buf.Bytes()
}

// renderUserEnv produces the ~/.lets/.env body (user-level subset only).
func renderUserEnv(values map[string]string) []byte {
	return renderTemplateKeys(letsconfig.UserHeader, letsconfig.UserKeys(), values)
}
