package initcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// StatuslineState classifies what's at .lets/statusline.sh.
type StatuslineState int

const (
	StatuslineAbsent      StatuslineState = iota // file doesn't exist
	StatuslineCurrentShim                        // matches embeddedStatuslineShim byte-for-byte
	StatuslineLegacyBash                         // > 5KB, contains legacy markers (_fetch_usage, compute_delta)
	StatuslineForeign                            // present but unrecognized - leave alone
)

// StatusLineFieldState classifies .claude/settings.json statusLine.command shape.
type StatusLineFieldState int

const (
	StatusLineAbsent          StatusLineFieldState = iota // statusLine field missing/empty
	StatusLineLetsBashWrapper                             // legacy: bash -c wrapper around $(git rev-parse).../.lets/statusline.sh
	StatusLineLetsDirect                                  // value == "lets statusline" (current canonical)
	StatusLineForeign                                     // user-customized
)

// DetectProjectRoot returns git toplevel or empty string. 2s timeout because
// `lets init` runs interactively and a hung git would block visibly.
func DetectProjectRoot() string {
	return gitutil.ProjectRoot("", 2*time.Second)
}

// DetectInsideWorktreeWithRoot returns (insideWorktree, mainRepoRoot) for the
// current working directory. mainRepoRoot is empty when not inside any git
// repo. Used by both DetectInsideWorktree (returns the bool only) and by
// callers that also need the main repo path (e.g. `lets worktree info`).
//
// Mechanism: `git rev-parse --git-dir` points at the worktree's
// `<main>/.git/worktrees/<name>` when inside a worktree, while
// `--git-common-dir` always points at the main repo's `.git`. They differ
// iff we're inside a worktree. Normalize both paths via filepath.Abs before
// comparing — git returns absolute when run from repo root and relative
// (e.g. "../../.git") when run from a subdirectory, so without normalization
// a subfolder of the main repo would be false-positively classified as a
// worktree (Phase 4b smoke-test regression preserved here).
func DetectInsideWorktreeWithRoot() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resolve := func(arg string) (string, bool) {
		out, err := exec.CommandContext(ctx, "git", "rev-parse", arg).Output()
		if err != nil {
			return "", false
		}
		abs, err := filepath.Abs(strings.TrimSpace(string(out)))
		if err != nil {
			return "", false
		}
		return abs, true
	}

	gitDir, ok := resolve("--git-dir")
	if !ok {
		return false, ""
	}
	commonDir, ok := resolve("--git-common-dir")
	if !ok {
		return false, ""
	}
	// commonDir resolves to <main>/.git — parent is the main repo root.
	mainRoot := filepath.Dir(commonDir)
	return gitDir != commonDir, mainRoot
}

// DetectInsideWorktree delegates to DetectInsideWorktreeWithRoot for the
// boolean answer (DRY).
func DetectInsideWorktree() bool {
	in, _ := DetectInsideWorktreeWithRoot()
	return in
}

// DetectPluginRoot resolves plugin install dir.
// Order: --plugin-root flag (passed in via param) > $CLAUDE_PLUGIN_ROOT env > error.
// No walk-up - too risky (plugin.json hijack vector).
//
// Sanity check: the resolved path MUST contain `.claude-plugin/plugin.json`.
// This blocks malicious "lets init --plugin-root=./vendor/evil" invocations
// that could otherwise overwrite the project's .claude/rules/ with arbitrary
// content (rules feed the orchestrator's project-instructions channel).
func DetectPluginRoot(flagValue string) (string, error) {
	cand := flagValue
	if cand == "" {
		cand = os.Getenv("CLAUDE_PLUGIN_ROOT")
	}
	if cand == "" {
		return "", errors.New("plugin root not found: pass --plugin-root or set CLAUDE_PLUGIN_ROOT")
	}
	abs, err := filepath.Abs(cand)
	if err != nil {
		return "", fmt.Errorf("invalid --plugin-root: %w", err)
	}
	marker := filepath.Join(abs, ".claude-plugin", "plugin.json")
	if _, err := os.Stat(marker); err != nil {
		return "", fmt.Errorf("not a LETS plugin install (missing .claude-plugin/plugin.json under %s)", abs)
	}
	return abs, nil
}

// detectStatuslineSh classifies .lets/statusline.sh.
func detectStatuslineSh(path string) StatuslineState {
	data, err := os.ReadFile(path)
	if err != nil {
		return StatuslineAbsent
	}
	if bytes.Equal(data, embeddedStatuslineShim) {
		return StatuslineCurrentShim
	}
	// Legacy bash heuristic: large (>5KB) AND contains key legacy markers.
	if len(data) > 5000 &&
		bytes.Contains(data, []byte("_fetch_usage")) &&
		bytes.Contains(data, []byte("compute_delta")) {
		return StatuslineLegacyBash
	}
	return StatuslineForeign
}

// detectStatusLineField classifies the statusLine command in settings.json by
// the value alone. For a single-string field, value-match against canonical
// commands answers every classification question (own / legacy / foreign).
// Earlier versions also wrote a `_letsManaged.statusLine` provenance marker;
// it added no decision power and is no longer written. Existing installs may
// still carry the orphan key — Claude Code ignores unknown keys, harmless.
func detectStatusLineField(settings map[string]any) StatusLineFieldState {
	sl, _ := settings["statusLine"].(map[string]any)
	if sl == nil {
		return StatusLineAbsent
	}
	cmd, _ := sl["command"].(string)
	if cmd == "" {
		return StatusLineAbsent
	}
	if cmd == "lets statusline" {
		return StatusLineLetsDirect
	}
	if strings.Contains(cmd, ".lets/statusline.sh") {
		return StatusLineLetsBashWrapper
	}
	return StatusLineForeign
}

// readSettingsJSON reads .claude/settings.json into a map. Returns nil map on
// missing file. Returns error only on malformed JSON (caller should refuse to
// mutate).
func readSettingsJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", path, err)
	}
	return m, nil
}
