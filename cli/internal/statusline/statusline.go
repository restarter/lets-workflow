// Package statusline implements the LETS-branded Claude Code statusline.
//
// Reads JSON context from Claude Code via stdin, writes a 2-line formatted
// statusline to stdout with branch, model, context window, and usage stats.
//
// Mirrors plugins/lets/scripts/lets/statusline.sh behavior 1:1 plus current
// CLI version in the LETS Workflow header line.
package statusline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// Input mirrors the JSON Claude Code pipes to the statusline command.
// PROTOTYPE (lets-ds6bc): expanded to decode the FULL documented payload so the
// max "rich" level can show everything. Compact (renderLines) still only reads
// model/workspace/cwd/context_window — additive, so it is unaffected.
type Input struct {
	Cwd            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	SessionName    string `json:"session_name"`
	TranscriptPath string `json:"transcript_path"`
	Version        string `json:"version"`
	Model          struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
		ProjectDir string `json:"project_dir"`
		// NOTE: workspace.git_worktree is intentionally NOT decoded — Claude
		// Code sends it as a STRING (the worktree path) in worktrees, not a
		// bool, which would fail json.Unmarshal and blank the whole bar. We
		// detect worktrees via worktree.name instead (see inWorktree).
		Repo struct {
			Host  string `json:"host"`
			Owner string `json:"owner"`
			Name  string `json:"name"`
		} `json:"repo"`
	} `json:"workspace"`
	OutputStyle struct {
		Name string `json:"name"`
	} `json:"output_style"`
	Cost struct {
		TotalCostUSD      float64 `json:"total_cost_usd"`
		TotalDurationMs   int64   `json:"total_duration_ms"`
		TotalLinesAdded   int     `json:"total_lines_added"`
		TotalLinesRemoved int     `json:"total_lines_removed"`
	} `json:"cost"`
	ContextWindow struct {
		TotalInputTokens    int     `json:"total_input_tokens"`
		TotalOutputTokens   int     `json:"total_output_tokens"`
		UsedPercentage      float64 `json:"used_percentage"`
		RemainingPercentage float64 `json:"remaining_percentage"`
		ContextWindowSize   int     `json:"context_window_size"`
		CurrentUsage        struct {
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	Exceeds200k bool `json:"exceeds_200k_tokens"`
	Effort      struct {
		Level string `json:"level"`
	} `json:"effort"`
	Thinking struct {
		Enabled bool `json:"enabled"`
	} `json:"thinking"`
	FastMode   bool `json:"fast_mode"`
	RateLimits struct {
		FiveHour struct {
			UsedPercentage float64 `json:"used_percentage"`
			// resets_at is flexISO, not string: Claude Code's statusline payload
			// sends it as a NUMBER (Unix epoch), while the Anthropic usage API
			// sends an ISO string. A plain string field fails json.Unmarshal on
			// the number and blanks the whole bar (same class as git_worktree).
			ResetsAt flexISO `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       flexISO `json:"resets_at"`
		} `json:"seven_day"`
	} `json:"rate_limits"`
	Vim struct {
		Mode string `json:"mode"`
	} `json:"vim"`
	Agent struct {
		Name string `json:"name"`
	} `json:"agent"`
	PR struct {
		Number      int    `json:"number"`
		URL         string `json:"url"`
		ReviewState string `json:"review_state"`
	} `json:"pr"`
	Worktree struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		Branch         string `json:"branch"`
		OriginalBranch string `json:"original_branch"`
	} `json:"worktree"`
}

// Render decodes the input JSON, fetches usage cache (or spawns background
// refresh if stale), and writes the statusline to w. The rich multi-line box is
// the DEFAULT; compact=true selects the legacy 2-line output. showTip toggles
// the rich bottom tip line (env LETS_STATUSLINE_TIP=off/0/false also disables);
// showDir toggles the Full-tier location pill (env LETS_STATUSLINE_DIR=off too).
//
// Resilient to empty/invalid stdin: Claude Code occasionally invokes the
// statusline command with no input (e.g. during /reload-plugins or initial
// render before the IPC pipe is wired). Empty input → render with zero-value
// Input (defaults to cwd-based detection). A blank statusline error is more
// disruptive than missing context.
func Render(stdin io.Reader, w io.Writer, light, compact, showTip, showDir bool) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var in Input
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &in); err != nil {
			// Don't fail rendering — surface a minimal line and continue.
			// stderr is not visible in Claude Code's bottom bar, so silent
			// fallback beats a blank/error line.
			fmt.Fprintf(w, "🌱 LETS Workflow [JSON parse error: %v]\n", err)
			return nil
		}
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
	// folder = the project/worktree root basename (git top-level), NOT the cwd
	// basename — otherwise the location pill shows whatever subdir you cd into
	// (e.g. "cli") instead of a stable project/worktree name.
	folder := filepath.Base(dir)
	if projectRoot != "" {
		folder = filepath.Base(projectRoot)
	}

	u := readUsageCache(filepath.Join(cacheDir, "usage"))

	if !u.fresh(cacheTTL) {
		spawnBackgroundFetch(cacheDir)
	}

	// Legacy 2-line output, opt-in via --compact (kept as a fallback for
	// terminals where the rich box's emoji/box-drawing misbehaves).
	if compact {
		return renderLines(w, in, branch, folder, u)
	}

	// Rich box is the default. env LETS_STATUSLINE_TIP=off/0/false hides the tip;
	// LETS_STATUSLINE_DIR=off/0/false hides the Full-tier location pill.
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("LETS_STATUSLINE_TIP"))); v == "off" || v == "0" || v == "false" || v == "no" {
		showTip = false
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("LETS_STATUSLINE_DIR"))); v == "off" || v == "0" || v == "false" || v == "no" {
		showDir = false
	}
	// Background-refresh the task-status cache off the hot path (same detached-
	// subprocess pattern as usage). Only the rich Line 2 consumes it. The id is
	// free (branch name); the bd call happens in the detached child, never inline.
	if id := taskIDFromBranch(branch); id != "" && !taskStatusFresh(cacheDir, id, taskStatusTTL) {
		// On a task SWITCH (cached id differs) the cache holds no data for this
		// id, so without a debounce every render in the fetch window re-spawns
		// bd. Write an id-only placeholder first: it renders immediately and
		// reads "fresh", collapsing the burst to one fetch. On a same-id TTL
		// refresh we skip the placeholder to keep showing the stale-but-real
		// title while bd refreshes.
		if cachedTaskID(cacheDir) != id {
			_ = writeTaskStatusPlaceholder(cacheDir, id)
		}
		spawnBackgroundTaskFetch(cacheDir, id)
	}
	return renderRich(w, in, branch, folder, u, detectWidth(), cacheDir, light, showTip, showDir)
}

// RunFetchOnly is the entry point used by the background subprocess.
// It fetches usage and writes cache, then returns. No stdin/stdout I/O.
func RunFetchOnly(cacheDir string) error {
	return fetchAndCacheUsage(cacheDir)
}

// RunFetchTaskOnly is the entry point for the detached task-status refresh
// (`lets statusline --fetch-task-only`). Queries bd for the task and writes the
// task-status cache. No stdin/stdout I/O.
func RunFetchTaskOnly(cacheDir, taskID string) error {
	return fetchAndCacheTaskStatus(cacheDir, taskID)
}

// detectProjectRoot wraps `git -C <dir> rev-parse --show-toplevel`.
// Returns empty string on failure (caller falls back to dir).
//
// 1-second timeout because statusline renders frequently and any git lag
// would visibly stall the bottom bar.
func detectProjectRoot(dir string) string {
	if dir == "" {
		return ""
	}
	return gitutil.ProjectRoot(dir, 1*time.Second)
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

// spawnBackgroundTaskFetch starts a detached subprocess that queries bd for the
// active task and refreshes cacheDir/task-status. Same detach mechanics as
// spawnBackgroundFetch; the bd call stays off the render hot path. bd is
// resolved from the inherited PATH — if it's absent the child fails silently
// and Line 2 degrades to id-only, which is the documented graceful path.
func spawnBackgroundTaskFetch(cacheDir, taskID string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "statusline", "--fetch-task-only", "--cache-dir", cacheDir, "--task-id", taskID)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	detachProcessGroup(cmd)
	_ = cmd.Start()
}
