// Package letsconfig is the single source of truth for canonical LETS_* config
// keys, their human-readable comment blocks, and their canonical defaults.
// Consumers:
//   - cli/internal/initcmd/render.go::renderEnv (initial .env write — uses Prefs values)
//   - cli/internal/initcmd/render.go::renderEnvExample (.env.example write — uses Default values)
//   - cli/internal/initcmd/env.go::RegenerateEnv (canonical writer; preserves user values + foreign keys)
//   - cli/internal/hook/sessionstart/sessionstart.go (whitelist for hook env injection)
//   - Future: /lets:doctor (validation + display)
//
// Adding a new config key (full recipe; CONTRIBUTING.md carries the same steps
// in prose under "### Adding a new config key"). Required edits:
//  1. Append a Key{Name, Comment, Default} entry to Keys below
//  2. Add field to Prefs struct in cli/internal/initcmd/render.go AND add
//     ONE entry to Prefs.AsValues() map (one-line addition right below)
//  3. Regenerate env goldens (go test ./internal/initcmd -run Golden -update)
//     and bump any hardcoded key-count assertions. Do NOT bump the
//     lets-rules.md frontmatter version per change - that happens once per
//     release at ceremony time (see CLAUDE.md "Release Flow").
//
// If the key is exposed via the /lets:init slash command (most are):
//  4. Add a --<key> cobra flag in cli/internal/cli/init.go (raw flag value
//     passed directly into Prefs; empty indicates "user did not pass --<key>")
//  5. Add an AskUserQuestion in plugins/lets/commands/init.md
//
// Auto-derived (no edit needed):
//   - .lets/.env content       (renderEnv → renderTemplate(Header, p.AsValues()))
//   - .lets/.env.example       (renderEnvExample → renderTemplate(ExampleHeader, Defaults()))
//   - SessionStart whitelist   (sessionstart imports Names())
//   - Regenerate wiring        (RegenerateEnv uses p.AsValues(), iterates Keys)
//   - Future /lets:doctor      (validation + display)
package letsconfig

// VersionKeyName is the metadata key recorded as the first line of generated
// .env files. Used by RegenerateEnv to detect when the file's schema or header
// is out of date relative to the running binary.
//
// NOT included in Keys — version is metadata, not user-facing config. The
// SessionStart hook whitelist (Names()) excludes it, so it's never injected
// into the model context.
const VersionKeyName = "LETS_ENV_VERSION"

// Header is the file-level comment block written at the top of fresh .env files.
// Per CLAUDE.md: kept above keys (NOT inline) because the SessionStart hook strips
// full-line comments before injecting env into model context.
const Header = `# LETS plugin config
# NOT FOR SECRETS. Contents are injected verbatim into model context every
# session (subject to whitelist filter in lets hook session-start). Put
# tokens/passwords elsewhere (gh auth, OS keychain, .beads/.env).
`

// ExampleHeader is the file-level comment block for .lets/.env.example. Differs
// from Header by including the "Copy to .lets/.env" instruction (this file is
// reference, not active config).
const ExampleHeader = `# LETS plugin config — REFERENCE ONLY
# Copy to .lets/.env and adjust for your project.
# NOT FOR SECRETS. The active .env is injected verbatim into model context every
# session (subject to whitelist filter in lets hook session-start). Put
# tokens/passwords elsewhere (gh auth, OS keychain, .beads/.env).
`

// UserHeader is the file-level comment block for the user-level ~/.lets/.env
// written by `lets init --user`. Same NOT-FOR-SECRETS contract as Header;
// names the precedence so a user editing the file knows project values win.
const UserHeader = `# LETS user-level config (machine-global defaults)
# Per-project .lets/.env values OVERRIDE these. NOT FOR SECRETS - injected
# into model context in EVERY project you open (subject to whitelist filter
# in lets hook session-start). Put tokens/passwords elsewhere (gh auth, OS
# keychain, .beads/.env).
`

// Key describes a single LETS_* config key.
type Key struct {
	Name    string // e.g. "LETS_LANGUAGE"
	Comment string // single-line, rendered as "# {comment}\n" ABOVE the key=value
	Default string // canonical default for .env.example and Prefs fallback
	// UserLevel marks keys that make sense as machine-global defaults in
	// ~/.lets/.env (written by `lets init --user`). Per-project keys
	// (MERGE_BRANCH, PR_FLOW, TRACKER) stay false: a global value would be
	// wrong in any repo that deviates, and the hook has better fallbacks
	// (git-derived default branch). A user can still hand-add any LETS_* key
	// to ~/.lets/.env - RegenerateUserEnv preserves it as a foreign line and
	// the hook injects it (whitelist applies at emit).
	UserLevel bool
}

// Keys is the canonical, ordered list. Single source of truth for the
// canonical LETS_* config keys. Adding a new key: see package doc above.
var Keys = []Key{
	{
		Name:      "LETS_LANGUAGE",
		Comment:   "Default response language — write the English name, like every value here (English, Ukrainian, Russian, Japanese, ...)",
		Default:   "English",
		UserLevel: true,
	},
	{
		Name:    "LETS_MERGE_BRANCH",
		Comment: "Target branch for merges and PR base",
		Default: "main",
	},
	{
		Name:    "LETS_PR_FLOW",
		Comment: "PR flow: github | bitbucket | local",
		Default: "local",
	},
	{
		Name:    "LETS_TRACKER",
		Comment: "Task tracker (currently 'beads'; schema reserved for Linear/Jira)",
		Default: "beads",
	},
	{
		Name:      "LETS_LAUNCHER",
		Comment:   "Worktree launcher: terminal (print the cd command) | cmux (open in a cmux workspace, macOS only)",
		Default:   "terminal",
		UserLevel: true,
	},
	{
		Name:    "LETS_RULES_SCOPE",
		Comment: "Where this project's workflow rules come from: project (own .claude/rules copy) | user (rely on the ~/.claude/rules global copy)",
		Default: "project",
	},
}

// UserKeys returns the subset of Keys that `lets init --user` manages in
// ~/.lets/.env. Fresh slice each call (same contract as Defaults).
func UserKeys() []Key {
	var out []Key
	for _, k := range Keys {
		if k.UserLevel {
			out = append(out, k)
		}
	}
	return out
}

// Defaults returns a map[Name]Default for fast lookup. Used by renderEnvExample,
// the cobra wrapper's empty-Prefs fallback, and (future) /lets:doctor display.
//
// Returns a FRESH map on every call — safe to mutate by callers. No caching;
// the allocation cost is trivial for the small canonical key set.
func Defaults() map[string]string {
	out := make(map[string]string, len(Keys))
	for _, k := range Keys {
		out[k.Name] = k.Default
	}
	return out
}

// Names returns just the Key names — for callers that don't need comments
// (sessionstart whitelist, envupdate iteration).
func Names() []string {
	out := make([]string, len(Keys))
	for i, k := range Keys {
		out[i] = k.Name
	}
	return out
}
