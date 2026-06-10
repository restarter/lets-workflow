package initcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

// StepStatus is a typed enum for step result categories.
// Underlying type is string so the value is also a human-readable label.
type StepStatus string

const (
	StepOK      StepStatus = "ok"
	StepSkip    StepStatus = "skip"
	StepWarn    StepStatus = "warn"
	StepErr     StepStatus = "err"
	StepMigrate StepStatus = "migrate"
)

// Step describes one apply step result for caller rendering.
type Step struct {
	Status  StepStatus `json:"status"`
	Message string     `json:"message"`
}

// Run executes init in linear order. Idempotent. Refuses if projectRoot is
// $HOME or filesystem root.
//
// Returns Result with Steps slice for the caller to render. Returns error only
// for hard failures (refused preconditions, write errors).
//
// Partial-completion contract: when Run returns an error mid-flight, it ALSO
// returns a Result with Steps populated for work completed so far. Callers
// should render returned Steps even on error so the user sees what was already
// done. Soft warnings (bd not found, foreign statusline) become Steps with
// status StepWarn / StepMigrate, not errors.
func Run(ctx context.Context, prefs Prefs, projectRoot, pluginRoot string) (Result, error) {
	result := NewResult(projectRoot, pluginRoot)

	if err := guardProjectRoot(projectRoot); err != nil {
		return result, err
	}

	// 0. git context (informational - cobra wrapper validated repo presence
	// before calling Run; we just surface the branch for symmetry with the
	// beads step at the end).
	if branch := gitutil.Branch(projectRoot, 2*time.Second); branch != "" {
		result.Add(Step{Status: StepOK, Message: fmt.Sprintf("git (branch: %s)", branch)})
	} else if !gitutil.HasCommits(projectRoot, 2*time.Second) {
		result.Add(Step{Status: StepOK, Message: "git (no commits yet)"})
	} else {
		result.Add(Step{Status: StepOK, Message: "git (detached HEAD)"})
	}

	// 1. .lets/ structure
	dirs := []string{
		filepath.Join(projectRoot, ".lets"),
		filepath.Join(projectRoot, ".lets", "sessions"),
		filepath.Join(projectRoot, ".lets", "reviews"),
		filepath.Join(projectRoot, ".lets", "plans"),
		filepath.Join(projectRoot, ".lets", "execution"),
		filepath.Join(projectRoot, ".lets", "cache"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return result, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	if err := os.Chmod(filepath.Join(projectRoot, ".lets", "cache"), 0o700); err != nil {
		return result, err
	}
	result.Add(Step{Status: StepOK, Message: ".lets/ structure (5 dirs)"})

	// 2. .gitignore — only LETS-owned paths. `bd init` writes its own .beads/
	// entries (and its own auto-generated CLAUDE.md / AGENTS.md hook block);
	// we don't speak for it.
	if err := EnsureGitignore(projectRoot, []string{".lets/", ".worktrees/"}); err != nil {
		return result, err
	}
	result.Add(Step{Status: StepOK, Message: ".gitignore entries"})

	// 3. statusline.sh migration
	if msg, err := MigrateStatuslineSh(projectRoot); err != nil {
		return result, err
	} else if msg != "" {
		result.Add(Step{Status: StepMigrate, Message: msg})
	}

	// 4. yaml→env migration (if applicable)
	if msg, did, err := MigrateYamlToEnv(projectRoot); err != nil {
		return result, err
	} else if did {
		result.Add(Step{Status: StepMigrate, Message: msg})
	} else if msg != "" {
		// Soft warning - yaml present but unreadable (permissions etc).
		result.Add(Step{Status: StepWarn, Message: msg})
	}

	// 5. .env (version-aware regenerate; RegenerateEnv decides skip vs write)
	envPath := filepath.Join(projectRoot, ".lets", ".env")
	envExists := false
	if _, err := os.Stat(envPath); err == nil {
		envExists = true
	}
	// Fresh creation requires all prefs flags - the only path that can't infer
	// values from existing .env or yaml migration.
	if !envExists {
		if prefs.Language == "" || prefs.MergeBranch == "" || prefs.PRFlow == "" {
			return result, fmt.Errorf("creating new .env requires --language, --merge-branch, --pr-flow")
		}
	}
	action, err := RegenerateEnv(envPath, prefs)
	if err != nil {
		return result, err
	}
	result.EnvAction = action
	switch action.Kind {
	case EnvCreated:
		result.Add(Step{Status: StepOK, Message: fmt.Sprintf(".lets/.env created (%s, %s, %s, %s)", version.Format(action.NewVersion), prefs.Language, prefs.MergeBranch, prefs.PRFlow)})
	case EnvSkip:
		result.Add(Step{Status: StepSkip, Message: fmt.Sprintf(".lets/.env (%s, in sync)", version.Format(action.PrevVersion))})
	case EnvRegenerated:
		msg := fmt.Sprintf(".lets/.env regenerated (%s -> %s", version.Format(action.PrevVersion), version.Format(action.NewVersion))
		if len(action.ChangedKeys) > 0 {
			msg += fmt.Sprintf(", %d keys changed", len(action.ChangedKeys))
		}
		msg += ")"
		result.Add(Step{Status: StepOK, Message: msg})
	}

	// 6. .env.example always refreshes from canonical letsconfig defaults
	// (no plugin template file — Go is the single source of truth).
	examplePath := filepath.Join(projectRoot, ".lets", ".env.example")
	if err := AtomicWriteBytes(examplePath, renderEnvExample(), 0o644); err != nil {
		return result, err
	}
	result.Add(Step{Status: StepOK, Message: ".lets/.env.example (refreshed)"})

	// 7. settings.json (value-match: detect canonical command; no provenance marker)
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		result.Add(Step{Status: StepWarn, Message: fmt.Sprintf("settings.json malformed - skipped: %v", err)})
	} else {
		state := detectStatusLineField(settings)
		switch state {
		case StatusLineForeign:
			result.Add(Step{Status: StepSkip, Message: ".claude/settings.json statusLine is user-customized - left alone"})
		case StatusLineLetsDirect:
			result.Add(Step{Status: StepSkip, Message: ".claude/settings.json (statusLine canonical)"})
		default: // Absent or LetsBashWrapper
			if err := SetStatusLine(settingsPath); err != nil {
				return result, err
			}
			result.Add(Step{Status: StepOK, Message: ".claude/settings.json statusLine -> 'lets statusline'"})
		}
	}

	// 8. .claude/rules/lets-rules.md (drift-aware)
	rulesSrc := filepath.Join(pluginRoot, "rules", "lets-rules.md")
	rulesDst := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	rulesData, readErr := os.ReadFile(rulesSrc)
	if readErr != nil {
		// Plugin rules unreadable. Populate Drift.State explicitly so JSON consumers
		// can disambiguate "drift check ran, equal" from "drift check failed".
		result.Drift = DriftReport{Detected: false, State: drift.StatePluginUnreadable}
		result.Add(Step{Status: StepWarn, Message: fmt.Sprintf("plugin rules missing: %s", rulesSrc)})
	} else {
		dr := drift.Check(rulesSrc, rulesDst)
		result.Drift = DriftReport{
			Detected:         dr.Detected(),
			State:            dr.State,
			InstalledVersion: dr.InstalledVersion,
			PluginVersion:    dr.PluginVersion,
			Message:          drift.Message(dr),
		}
		if dr.Detected() && prefs.SkipRules {
			// Global rules at ~/.claude/rules cover this project (Claude Code
			// loads user-level rules everywhere; project copy only overrides).
			// Drift report above stays truthful about the project copy's state;
			// this step says why nothing was written.
			result.Add(Step{Status: StepSkip, Message: ".claude/rules/lets-rules.md (skipped - global rules cover this project; re-run without --skip-rules to install for the team)"})
		} else if dr.Detected() {
			if err := os.MkdirAll(filepath.Dir(rulesDst), 0o755); err != nil {
				return result, err
			}
			if err := AtomicWriteBytes(rulesDst, rulesData, 0o644); err != nil {
				return result, err
			}
			// Recompute drift against newly-written file. Symmetric to the
			// pre-write check; surfaces any post-write inconsistency (atomic-rename
			// oddity, frontmatter corruption) as a non-equal state instead of
			// silently lying that all is well.
			drPost := drift.Check(rulesSrc, rulesDst)
			result.Drift = DriftReport{
				Detected:         drPost.Detected(),
				State:            drPost.State,
				InstalledVersion: drPost.InstalledVersion,
				PluginVersion:    drPost.PluginVersion,
				Message:          drift.Message(drPost),
			}
			switch dr.State {
			case drift.StateMissing:
				result.Add(Step{Status: StepOK, Message: fmt.Sprintf(".claude/rules/lets-rules.md installed (v%s)", dr.PluginVersion)})
			case drift.StateUnknown:
				result.Add(Step{Status: StepOK, Message: fmt.Sprintf(".claude/rules/lets-rules.md refreshed (was: unparseable, now v%s)", dr.PluginVersion)})
			case drift.StateOutdated, drift.StateAhead:
				result.Add(Step{Status: StepOK, Message: fmt.Sprintf(".claude/rules/lets-rules.md updated (v%s -> v%s)", dr.InstalledVersion, dr.PluginVersion)})
			}
		} else {
			result.Add(Step{Status: StepSkip, Message: fmt.Sprintf(".claude/rules/lets-rules.md (v%s up to date)", dr.InstalledVersion)})
		}
	}

	// 9. beads
	if !prefs.SkipBeads {
		bdSteps := runBeadsInit(ctx, projectRoot)
		for _, s := range bdSteps {
			result.Add(s)
		}
	} else {
		result.Add(Step{Status: StepSkip, Message: "beads (--skip-beads)"})
	}

	return result, nil
}

// guardProjectRoot refuses dangerous root paths.
func guardProjectRoot(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if abs == "/" || abs == "" {
		return fmt.Errorf("refusing to init in filesystem root: %s", abs)
	}
	if home, err := os.UserHomeDir(); err == nil && abs == home {
		return fmt.Errorf("refusing to init in $HOME: %s", abs)
	}
	return nil
}

// runBeadsInit invokes `bd init` (if bd on PATH) with 60s timeout.
//
// Detection of "already initialized" goes through `bd status` rather than
// filesystem-layout sniffing — bd's internal layout has changed before
// (.beads/dolt -> .beads/embeddeddolt/<dbname>/) and value-by-asking-bd is
// the same pattern we use for git via `git rev-parse`.
func runBeadsInit(ctx context.Context, projectRoot string) []Step {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		return []Step{{Status: StepWarn, Message: "bd (beads) not on PATH - install beads plugin"}}
	}

	// Authoritative source-of-truth: ask bd if workspace exists.
	// bd status exits 0 in initialized workspace, non-zero otherwise.
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	statusCmd := exec.CommandContext(statusCtx, bdPath, "status")
	statusCmd.Dir = projectRoot
	if err := statusCmd.Run(); err == nil {
		return []Step{{Status: StepSkip, Message: "beads (already initialized)"}}
	}

	// Not initialized -> run bd init
	timeoutCtx, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel2()
	cmd := exec.CommandContext(timeoutCtx, bdPath, "init")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return []Step{{Status: StepWarn, Message: fmt.Sprintf("bd init failed: %v\n%s", err, strings.TrimSpace(string(out)))}}
	}

	// bd config set hash_length 5 - best effort
	configCmd := exec.CommandContext(timeoutCtx, bdPath, "config", "set", "hash_length", "5")
	configCmd.Dir = projectRoot
	_ = configCmd.Run()

	steps := []Step{{Status: StepOK, Message: "beads initialized"}}
	return steps
}

// PrintSteps writes step results to w using simple text format.
func PrintSteps(w io.Writer, steps []Step) {
	for _, s := range steps {
		fmt.Fprintf(w, "%-10s %s\n", "["+string(s.Status)+"]", s.Message)
	}
}
