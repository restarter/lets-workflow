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

	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
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
	Status  StepStatus
	Message string
}

// RunOptions reserves space for future runtime tuning (verbosity, dry-run).
// Currently empty - all behavior is driven by Prefs and arguments.
type RunOptions struct{}

// Run executes init in linear order. Idempotent. Refuses if projectRoot is
// $HOME or filesystem root.
//
// Returns slice of Steps for the caller to render. Returns error only for
// hard failures (refused preconditions, write errors).
//
// Partial-completion contract: when Run returns an error mid-flight, it ALSO
// returns the Steps slice covering work completed so far. Callers should
// render returned steps even on error so the user sees what was already
// done. Soft warnings (bd not found, foreign statusline) become Steps with
// status StepWarn / StepMigrate, not errors.
func Run(ctx context.Context, prefs Prefs, _ RunOptions, projectRoot, pluginRoot string) ([]Step, error) {
	if err := guardProjectRoot(projectRoot); err != nil {
		return nil, err
	}

	var steps []Step

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
			return steps, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	if err := os.Chmod(filepath.Join(projectRoot, ".lets", "cache"), 0o700); err != nil {
		return steps, err
	}
	steps = append(steps, Step{Status: StepOK, Message: ".lets/ structure (5 dirs)"})

	// 2. .gitignore
	if err := EnsureGitignore(projectRoot, []string{".lets/", ".beads/", ".worktrees/"}); err != nil {
		return steps, err
	}
	steps = append(steps, Step{Status: StepOK, Message: ".gitignore entries"})

	// 3. statusline.sh migration
	if msg, err := MigrateStatuslineSh(projectRoot); err != nil {
		return steps, err
	} else if msg != "" {
		steps = append(steps, Step{Status: StepMigrate, Message: msg})
	}

	// 4. yaml→env migration (if applicable)
	if msg, did, err := MigrateYamlToEnv(projectRoot); err != nil {
		return steps, err
	} else if did {
		steps = append(steps, Step{Status: StepMigrate, Message: msg})
	}

	// 5. .env (if absent)
	envPath := filepath.Join(projectRoot, ".lets", ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envBytes := renderEnv(prefs)
		if err := atomicWriteBytes(envPath, envBytes, 0o644); err != nil {
			return steps, err
		}
		steps = append(steps, Step{Status: StepOK, Message: fmt.Sprintf(".lets/.env (%s, %s, %s)", prefs.Language, prefs.MergeBranch, prefs.PRFlow)})
	} else {
		steps = append(steps, Step{Status: StepSkip, Message: ".lets/.env (exists)"})
	}

	// 6. .env.example always refreshes from plugin template
	templatePath := filepath.Join(pluginRoot, "hooks", "config-template.env")
	if data, err := os.ReadFile(templatePath); err == nil {
		examplePath := filepath.Join(projectRoot, ".lets", ".env.example")
		if err := atomicWriteBytes(examplePath, data, 0o644); err != nil {
			return steps, err
		}
		steps = append(steps, Step{Status: StepOK, Message: ".lets/.env.example (refreshed)"})
	} else {
		steps = append(steps, Step{Status: StepWarn, Message: fmt.Sprintf("plugin template missing: %s", templatePath)})
	}

	// 7. settings.json (provenance-aware)
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		steps = append(steps, Step{Status: StepWarn, Message: fmt.Sprintf("settings.json malformed - skipped: %v", err)})
	} else {
		state := detectStatusLineField(settings)
		switch state {
		case StatusLineForeign:
			steps = append(steps, Step{Status: StepSkip, Message: ".claude/settings.json statusLine is user-customized - left alone"})
		case StatusLineLetsManaged:
			steps = append(steps, Step{Status: StepSkip, Message: ".claude/settings.json (statusLine managed)"})
		case StatusLineLetsDirect:
			if err := SetStatusLineManaged(settingsPath); err != nil {
				return steps, err
			}
			steps = append(steps, Step{Status: StepOK, Message: ".claude/settings.json _letsManaged marker added"})
		default: // Absent or LetsBashWrapper
			if err := SetStatusLineManaged(settingsPath); err != nil {
				return steps, err
			}
			steps = append(steps, Step{Status: StepOK, Message: ".claude/settings.json statusLine -> 'lets statusline'"})
		}
	}

	// 8. .claude/rules/lets-rules.md (copy from plugin, version-aware)
	rulesSrc := filepath.Join(pluginRoot, "rules", "lets-rules.md")
	rulesDst := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	rulesData, err := os.ReadFile(rulesSrc)
	if err != nil {
		steps = append(steps, Step{Status: StepWarn, Message: fmt.Sprintf("plugin rules missing: %s", rulesSrc)})
	} else {
		pluginVer := frontmatter.ReadVersion(rulesSrc)
		installedVer := frontmatter.ReadVersion(rulesDst)
		needsUpdate := installedVer == "" || installedVer != pluginVer
		if needsUpdate {
			if err := os.MkdirAll(filepath.Dir(rulesDst), 0o755); err != nil {
				return steps, err
			}
			if err := atomicWriteBytes(rulesDst, rulesData, 0o644); err != nil {
				return steps, err
			}
			if installedVer == "" {
				steps = append(steps, Step{Status: StepOK, Message: fmt.Sprintf(".claude/rules/lets-rules.md installed (v%s)", pluginVer)})
			} else {
				steps = append(steps, Step{Status: StepOK, Message: fmt.Sprintf(".claude/rules/lets-rules.md upgraded (v%s -> v%s)", installedVer, pluginVer)})
			}
		} else {
			steps = append(steps, Step{Status: StepSkip, Message: fmt.Sprintf(".claude/rules/lets-rules.md (v%s up to date)", installedVer)})
		}
	}

	// 9. beads
	if !prefs.SkipBeads {
		bdSteps := runBeadsInit(ctx, projectRoot)
		steps = append(steps, bdSteps...)
	} else {
		steps = append(steps, Step{Status: StepSkip, Message: "beads (--skip-beads)"})
	}

	return steps, nil
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
func runBeadsInit(ctx context.Context, projectRoot string) []Step {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		return []Step{{Status: StepWarn, Message: "bd (beads) not on PATH - install beads plugin"}}
	}

	beadsDir := filepath.Join(projectRoot, ".beads")
	doltDir := filepath.Join(beadsDir, "dolt")
	if entries, err := os.ReadDir(doltDir); err == nil {
		if len(entries) > 0 {
			return []Step{{Status: StepSkip, Message: "beads (already initialized)"}}
		}
		// empty .beads/dolt/ blocks bd init - clean up
		if err := os.RemoveAll(beadsDir); err != nil {
			return []Step{{Status: StepErr, Message: fmt.Sprintf("rm .beads/: %v", err)}}
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
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

	if _, err := os.Stat(filepath.Join(beadsDir, "hooks")); err == nil {
		_ = EnsureGitignore(projectRoot, []string{".beads/hooks/"})
	}
	return steps
}

// PrintSteps writes step results to w using simple text format. Used in
// non-interactive mode. Interactive mode also calls this after spinner exits
// (the completion screen renders the LETS box separately).
func PrintSteps(w io.Writer, steps []Step) {
	for _, s := range steps {
		fmt.Fprintf(w, "%-10s %s\n", "["+string(s.Status)+"]", s.Message)
	}
}
