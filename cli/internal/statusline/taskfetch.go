package statusline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// taskStatusTTL bounds how often the detached refresh re-queries bd for the
// active task's title + comment count. Title/notes change rarely and bd here
// may be a remote (Dolt) backend, so keep this generous — staleness costs only
// a slightly old note count on Line 2, never correctness.
const taskStatusTTL = 90 * time.Second

// taskStatusFresh reports whether cacheDir/task-status is recent enough to skip
// a background refresh AND was written for the current task. A file written for
// a different task counts as stale: the id guard in readTaskStatus rejects it
// for rendering anyway, so we want a refresh to replace it with this task's
// data rather than sit on a mismatched cache until the TTL elapses.
func taskStatusFresh(cacheDir, taskID string, ttl time.Duration) bool {
	path := filepath.Join(cacheDir, "task-status")
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) >= ttl {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	id, _, _ := strings.Cut(strings.TrimSpace(string(b)), "|")
	return id == taskID
}

// cachedTaskID returns the task id recorded in the cache, or "" if absent.
func cachedTaskID(cacheDir string) string {
	b, err := os.ReadFile(filepath.Join(cacheDir, "task-status"))
	if err != nil {
		return ""
	}
	id, _, _ := strings.Cut(strings.TrimSpace(string(b)), "|")
	return id
}

// writeTaskStatusPlaceholder writes an id-only cache line (empty title/notes)
// so a just-switched-to task renders immediately as id-only AND the cache reads
// "fresh" for this id — debouncing the detached bd fetch to a single in-flight
// call instead of re-spawning one per render until bd returns. The real fetch
// overwrites it with the title when it completes.
func writeTaskStatusPlaceholder(cacheDir, taskID string) error {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}
	return writeTaskStatusCache(filepath.Join(cacheDir, "task-status"), taskID+"||")
}

// bdIssue is the subset of `bd show <id> --json` we consume. The command emits
// a single-element array with comments embedded, so one bd call covers title +
// note count + last-comment timestamp.
type bdIssue struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Comments []struct {
		CreatedAt string `json:"created_at"`
	} `json:"comments"`
}

// taskStatusLine turns `bd show <id> --json` output into the 4-field cache line
// id|title|note-count|last-comment-iso. Pure (no I/O) so the parse is unit-test
// -able without spawning bd.
func taskStatusLine(taskID string, bdShowJSON []byte) (string, error) {
	var issues []bdIssue
	if err := json.Unmarshal(bdShowJSON, &issues); err != nil {
		return "", err
	}
	if len(issues) == 0 || issues[0].ID != taskID {
		return "", errors.New("bd returned no matching issue")
	}
	it := issues[0]
	notes := len(it.Comments)
	// Pick the most-recent comment by parsed timestamp, not array position — bd's
	// output ordering is not a guaranteed contract (the Dolt/remote backend can
	// merge multi-writer comments out of order), and string position would then
	// show the wrong "(37m ago)" age. parseISO normalizes timezones so the
	// comparison is chronological, not lexicographic.
	lastISO := ""
	var lastT time.Time
	for _, c := range it.Comments {
		iso := validateISO(c.CreatedAt)
		if iso == "" {
			continue
		}
		if t, ok := parseISO(iso); ok && (lastISO == "" || t.After(lastT)) {
			lastISO, lastT = iso, t
		}
	}
	return taskID + "|" + sanitizeField(it.Title) + "|" + strconv.Itoa(notes) + "|" + lastISO, nil
}

// stripControl neutralizes terminal control bytes in any externally-sourced
// field before it is rendered: every C0 control (< 0x20, including ESC/CR/LF),
// DEL (0x7f), and the C1 control range (0x80-0x9f, which can act as CSI/OSC
// introducers on some terminals) becomes a space. This is the shared
// escape-injection barrier for EVERY untrusted rendered field — the folder /
// worktree / branch name (filesystem- and ref-controlled), the model name, PR
// state, effort, and the bd task title. It must run on the RAW value before our
// own ANSI SGR coloring is wrapped around it, so the renderer's colors survive
// while injected `\x1b[2J` / OSC / cursor sequences do not.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}

// sanitizeField is stripControl plus the cache-specific concerns for the bd task
// title: it folds the "|" delimiter to "/" (so it can't corrupt the
// pipe-delimited cache line) and trims surrounding whitespace.
func sanitizeField(s string) string {
	return strings.TrimSpace(stripControl(strings.ReplaceAll(s, "|", "/")))
}

// fetchAndCacheTaskStatus runs `bd show <id> --json` and writes the derived
// 4-field line to cacheDir/task-status. Used by the detached
// `--fetch-task-only` subprocess; errors are silent (no UI consumes them).
func fetchAndCacheTaskStatus(cacheDir, taskID string) error {
	// Defense in depth: taskID normally comes from taskIDFromBranch (regex,
	// can't start with "-"), but --task-id can be passed arbitrary input. Reject
	// a leading "-" so it can't be smuggled as a bd flag, and pass it after "--"
	// so bd's parser treats it strictly as a positional argument.
	if taskID == "" || strings.HasPrefix(taskID, "-") {
		return errors.New("invalid task id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "bd", "show", "--json", "--", taskID).Output()
	if err != nil {
		return err
	}
	line, err := taskStatusLine(taskID, out)
	if err != nil {
		return err
	}
	// 0o700 mirrors what `lets init` sets for .lets/cache (operational metadata,
	// not world-readable on shared hosts). Re-tighten in case it pre-existed wider.
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(cacheDir, 0o700)
	return writeTaskStatusCache(filepath.Join(cacheDir, "task-status"), line)
}

// writeTaskStatusCache writes the single-line cache atomically (CreateTemp +
// rename, mode 0o600), mirroring writeUsageCache so concurrent renders racing
// to refresh can't interleave a partial write.
func writeTaskStatusCache(path, line string) error {
	f, err := os.CreateTemp(filepath.Dir(path), "task-status-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(e error) error {
		_ = f.Close()
		_ = os.Remove(tmp)
		return e
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fail(err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
