// Package statusline implements the LETS-branded Claude Code statusline.
//
// Reads JSON context from Claude Code via stdin, writes a 2-line formatted
// statusline to stdout with branch, model, context window, and usage stats.
//
// Mirrors plugins/lets/scripts/lets/statusline.sh behavior 1:1 plus current
// CLI version in the LETS Workflow header line.
package statusline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Input mirrors the JSON Claude Code pipes to the statusline command.
// Only fields we use are decoded.
type Input struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Cwd           string `json:"cwd"`
	ContextWindow struct {
		UsedPercentage    float64 `json:"used_percentage"`
		ContextWindowSize int     `json:"context_window_size"`
		CurrentUsage      struct {
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
}

// Render decodes the input JSON, fetches usage cache (or spawns background
// refresh if stale), and writes the 2-line formatted statusline to w.
func Render(stdin io.Reader, w io.Writer) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var in Input
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}

	dir := in.Workspace.CurrentDir
	if dir == "" {
		dir = in.Cwd
	}
	projectRoot := detectProjectRoot(dir)
	if projectRoot == "" {
		projectRoot = dir
	}
	cacheDir := filepath.Join(projectRoot, ".lets", "cache")

	branch := detectBranch(dir)
	folder := filepath.Base(dir)

	u := readUsageCache(filepath.Join(cacheDir, "usage"))

	if !u.fresh(cacheTTL) {
		spawnBackgroundFetch(cacheDir)
	}

	return renderLines(w, in, branch, folder, u)
}

// RunFetchOnly is the entry point used by the background subprocess.
// It fetches usage and writes cache, then returns. No stdin/stdout I/O.
func RunFetchOnly(cacheDir string) error {
	return fetchAndCacheUsage(cacheDir)
}

// detectProjectRoot wraps `git -C <dir> rev-parse --show-toplevel`.
// Returns empty string on failure (caller falls back to dir).
func detectProjectRoot(dir string) string {
	if dir == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// detectBranch returns the symbolic branch name or short SHA if detached.
// Empty string if not a git repo.
func detectBranch(dir string) string {
	if dir == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "--short", "HEAD")
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	cmd = exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--short", "HEAD")
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// spawnBackgroundFetch starts a detached subprocess that fetches the usage
// API and writes to cacheDir/usage. Mirrors bash:
//
//	_fetch_usage > /dev/null 2>&1 &
//
// Implementation: re-exec self with --fetch-usage-only, in a new process
// group (so the child survives the parent shell's SIGHUP on exit).
// detachProcessGroup is build-tag split: spawn_unix.go vs spawn_windows.go.
func spawnBackgroundFetch(cacheDir string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "statusline", "--fetch-usage-only", "--cache-dir", cacheDir)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	detachProcessGroup(cmd)
	_ = cmd.Start()
	// Do NOT call cmd.Wait() - that would block the parent. Brief zombie is
	// reclaimed by the kernel once the short-lived parent exits.
}
