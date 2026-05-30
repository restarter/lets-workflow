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
	lastISO := ""
	if notes > 0 {
		lastISO = validateISO(it.Comments[notes-1].CreatedAt)
	}
	return taskID + "|" + sanitizeField(it.Title) + "|" + strconv.Itoa(notes) + "|" + lastISO, nil
}

// sanitizeField strips the cache's structural characters (the "|" delimiter and
// newlines) from a free-text field so a title can't corrupt the single-line,
// pipe-delimited format.
func sanitizeField(s string) string {
	r := strings.NewReplacer("|", "/", "\n", " ", "\r", " ")
	return strings.TrimSpace(r.Replace(s))
}

// fetchAndCacheTaskStatus runs `bd show <id> --json` and writes the derived
// 4-field line to cacheDir/task-status. Used by the detached
// `--fetch-task-only` subprocess; errors are silent (no UI consumes them).
func fetchAndCacheTaskStatus(cacheDir, taskID string) error {
	if taskID == "" {
		return errors.New("no task id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "bd", "show", taskID, "--json").Output()
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
