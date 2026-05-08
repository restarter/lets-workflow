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
	StatusLineLetsManaged                                 // _letsManaged.statusLine == true (provenance marker)
	StatusLineLetsBashWrapper                             // legacy: bash -c wrapper around $(git rev-parse).../.lets/statusline.sh
	StatusLineLetsDirect                                  // value == "lets statusline" (current canonical, but no provenance marker yet)
	StatusLineForeign                                     // user-customized, no LETS markers
)

// DetectProjectRoot returns git toplevel or empty string.
func DetectProjectRoot() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// DetectInsideWorktree returns true if the current dir is inside a git worktree
// (not the main repo). Robust detection: --git-dir points at the worktree's
// `<main>/.git/worktrees/<name>` while --git-common-dir points at the main
// repo's `.git`. They differ iff we're inside a worktree. Substring matching
// on the literal "/worktrees/" path component is fragile (path could legally
// contain that segment elsewhere), so prefer the path-equality check.
func DetectInsideWorktree() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gitDir, err := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return false
	}
	commonDir, err := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(gitDir)) != strings.TrimSpace(string(commonDir))
}

// DetectPluginRoot resolves plugin install dir.
// Order: --plugin-root flag (passed in via param) > $CLAUDE_PLUGIN_ROOT env > error.
// No walk-up - too risky (plugin.json hijack vector). Caller must provide.
func DetectPluginRoot(flagValue string) (string, error) {
	if flagValue != "" {
		abs, err := filepath.Abs(flagValue)
		if err != nil {
			return "", fmt.Errorf("invalid --plugin-root: %w", err)
		}
		return abs, nil
	}
	if env := os.Getenv("CLAUDE_PLUGIN_ROOT"); env != "" {
		return env, nil
	}
	return "", errors.New("plugin root not found: pass --plugin-root or set CLAUDE_PLUGIN_ROOT")
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

// detectStatusLineField classifies the statusLine command in settings.json.
func detectStatusLineField(settings map[string]any) StatusLineFieldState {
	managed, _ := settings["_letsManaged"].(map[string]any)
	if managed != nil {
		if b, _ := managed["statusLine"].(bool); b {
			return StatusLineLetsManaged
		}
	}
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
