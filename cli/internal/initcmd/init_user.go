package initcmd

import (
	"fmt"
	"path/filepath"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/rulescache"
)

// UserOptions carries the inputs for RunUser. Struct (not positionals) so the
// two same-typed path params can't be transposed silently.
type UserOptions struct {
	Language   string // empty = preserve existing ~/.lets/.env value, else default
	Launcher   string // same sentinel semantics
	HomeDir    string // resolved by the cobra wrapper via os.UserHomeDir(); never ""
	PluginRoot string // validated by DetectPluginRoot in the cobra wrapper
}

// RunUser executes the user-scope install: global rules at
// ~/.claude/rules/lets-rules.md + user-level defaults at ~/.lets/.env.
// Idempotent. Deliberately a SUBSET of project Run: no git, no .gitignore,
// no migrations, no settings.json statusline, no beads, no .env.example.
//
// The global rules copy is a cache of the running plugin (lets-tg008), written
// only through rulescache: a copy that is not LETS content is saved to
// .bak[-N] before it is replaced, and a newer cache is never downgraded by an
// older plugin. Anything withheld (kept newer, untrusted plugin root, lock busy,
// I/O failure) is a named warning, never silent.
//
// Result envelope: same initcmd.Result, SchemaVersion unchanged.
// Result.ProjectRoot carries HomeDir (the "scope root" for a user-scope run -
// documented in cli/README.md; the /lets:init markdown branches on --user and
// reads it accordingly).
func RunUser(o UserOptions) (Result, error) {
	result := NewResult(o.HomeDir, o.PluginRoot)

	if err := guardHomeDir(o.HomeDir); err != nil {
		return result, err
	}

	// 1. Global rules: ~/.claude/rules/lets-rules.md is a cache of the running
	// plugin, written only through rulescache (lets-tg008). --user is an
	// explicit bootstrap, so it may create the file.
	res := rulescache.Sync(rulescache.Options{PluginRoot: o.PluginRoot, HomeDir: o.HomeDir, InProject: true, ScopeUser: true})
	switch res.Outcome {
	case rulescache.OutcomeCreated, rulescache.OutcomeWritten:
		result.Drift = DriftReport{State: drift.StateEqual}
		result.Add(Step{Status: StepOK, Message: res.Notice()})
	case rulescache.OutcomeNoop:
		result.Drift = DriftReport{State: drift.StateEqual}
		result.Add(Step{Status: StepSkip, Message: "~/.claude/rules/lets-rules.md (matches the running plugin)"})
	default: // kept-newer, skipped, failed: named, never silent
		result.Drift = DriftReport{Detected: true, State: drift.StateUnknown, Message: res.Notice()}
		result.Add(Step{Status: StepWarn, Message: res.Notice()})
	}

	// 2. User-level defaults: ~/.lets/.env (LETS_LANGUAGE + LETS_LAUNCHER)
	envPath := filepath.Join(o.HomeDir, ".lets", ".env")
	action, err := RegenerateUserEnv(envPath, map[string]string{
		"LETS_LANGUAGE": o.Language,
		"LETS_LAUNCHER": o.Launcher,
	})
	if err != nil {
		return result, err
	}
	result.EnvAction = action
	switch action.Kind {
	case EnvCreated:
		result.Add(Step{Status: StepOK, Message: "~/.lets/.env created (user-level defaults: language, launcher)"})
	case EnvSkip:
		result.Add(Step{Status: StepSkip, Message: "~/.lets/.env (in sync)"})
	case EnvRegenerated:
		result.Add(Step{Status: StepOK, Message: fmt.Sprintf("~/.lets/.env regenerated (%d key(s) changed)", len(action.ChangedKeys))})
	}

	return result, nil
}

// guardHomeDir refuses degenerate home paths. A relative $HOME (e.g. "." in a
// broken container) is refused outright - silently Abs-ing it would land
// .claude/ inside an arbitrary cwd, and a relative home is always a
// misconfiguration for a machine-global install. Root-refusal is best-effort
// on Windows (drive roots like C:\ are not special-cased).
func guardHomeDir(home string) error {
	if home == "" {
		return fmt.Errorf("cannot resolve home directory for --user install")
	}
	if !filepath.IsAbs(home) {
		return fmt.Errorf("refusing user-scope install: home directory is not absolute: %s", home)
	}
	if filepath.Clean(home) == "/" {
		return fmt.Errorf("refusing user-scope install with home = filesystem root")
	}
	return nil
}
