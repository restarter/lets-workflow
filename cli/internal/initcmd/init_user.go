package initcmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/restarter/lets-workflow/cli/internal/drift"
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
// ahead-state no-clobber: unlike the project copy (plugin-owned, reset on any
// drift), a global rules file NEWER than the plugin is left untouched - it is
// either a user customization (the only per-project opt-out mechanism Claude
// Code offers, see GH anthropics/claude-code#8395) or a newer release's copy;
// both must survive. unknown (unparseable frontmatter) IS overwritten:
// lets-* files are plugin-owned by convention and an unversioned copy can't
// be drift-tracked.
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

	// 1. Global rules: ~/.claude/rules/lets-rules.md (drift-aware, ahead-safe)
	rulesSrc := filepath.Join(o.PluginRoot, "rules", "lets-rules.md")
	rulesDst := filepath.Join(o.HomeDir, ".claude", "rules", "lets-rules.md")
	rulesData, readErr := os.ReadFile(rulesSrc)
	if readErr != nil {
		result.Drift = DriftReport{Detected: false, State: drift.StatePluginUnreadable}
		result.Add(Step{Status: StepWarn, Message: fmt.Sprintf("plugin rules missing: %s", rulesSrc)})
	} else {
		dr := drift.Check(rulesSrc, rulesDst)
		result.Drift = DriftReport{
			Detected:         dr.Detected(),
			State:            dr.State,
			InstalledVersion: dr.InstalledVersion,
			PluginVersion:    dr.PluginVersion,
			Message:          drift.MessageUser(dr),
		}
		switch {
		case dr.State == drift.StateAhead:
			result.Add(Step{Status: StepWarn, Message: fmt.Sprintf("~/.claude/rules/lets-rules.md AHEAD (v%s > plugin v%s) - left untouched (customized or newer release)", dr.InstalledVersion, dr.PluginVersion)})
		case dr.Detected():
			if err := os.MkdirAll(filepath.Dir(rulesDst), 0o755); err != nil {
				return result, err
			}
			if err := AtomicWriteBytes(rulesDst, rulesData, 0o644); err != nil {
				return result, err
			}
			// Recompute drift against the newly-written file - symmetric to
			// project Run Step 8 (surfaces post-write inconsistency instead of
			// silently lying that all is well).
			drPost := drift.Check(rulesSrc, rulesDst)
			result.Drift = DriftReport{
				Detected:         drPost.Detected(),
				State:            drPost.State,
				InstalledVersion: drPost.InstalledVersion,
				PluginVersion:    drPost.PluginVersion,
				Message:          drift.MessageUser(drPost),
			}
			switch dr.State {
			case drift.StateMissing:
				result.Add(Step{Status: StepOK, Message: fmt.Sprintf("~/.claude/rules/lets-rules.md installed (v%s)", dr.PluginVersion)})
			case drift.StateUnknown:
				result.Add(Step{Status: StepOK, Message: fmt.Sprintf("~/.claude/rules/lets-rules.md refreshed (was: unparseable, now v%s)", dr.PluginVersion)})
			case drift.StateOutdated:
				result.Add(Step{Status: StepOK, Message: fmt.Sprintf("~/.claude/rules/lets-rules.md updated (v%s -> v%s)", dr.InstalledVersion, dr.PluginVersion)})
			}
		default:
			result.Add(Step{Status: StepSkip, Message: fmt.Sprintf("~/.claude/rules/lets-rules.md (v%s up to date)", dr.InstalledVersion)})
		}
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
