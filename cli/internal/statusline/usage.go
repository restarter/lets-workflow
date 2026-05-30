package statusline

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// usage holds parsed cache state. *OK fields distinguish 0% from missing.
type usage struct {
	fiveHour      int
	fiveHourOK    bool
	sevenDay      int
	sevenDayOK    bool
	fiveHourReset string
	sevenDayReset string
	mtime         time.Time
}

// fresh reports whether cache mtime is within ttl of now.
// Empty mtime (file missing) => not fresh.
func (u usage) fresh(ttl time.Duration) bool {
	if u.mtime.IsZero() {
		return false
	}
	return time.Since(u.mtime) < ttl
}

// readUsageCache reads the 4-line bash-compatible cache format:
//  1. 5h percentage (integer)
//  2. 7d percentage (integer)
//  3. 5h resets_at (ISO 8601)
//  4. 7d resets_at (ISO 8601)
//
// Missing or malformed file returns zero-value usage{}.
func readUsageCache(path string) usage {
	var u usage
	f, err := os.Open(path)
	if err != nil {
		return u
	}
	defer func() { _ = f.Close() }()

	if info, err := f.Stat(); err == nil {
		u.mtime = info.ModTime()
	}

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, 4)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) == 4 {
			break
		}
	}
	if len(lines) >= 1 {
		if v, err := strconv.Atoi(lines[0]); err == nil {
			u.fiveHour, u.fiveHourOK = v, true
		}
	}
	if len(lines) >= 2 {
		if v, err := strconv.Atoi(lines[1]); err == nil {
			u.sevenDay, u.sevenDayOK = v, true
		}
	}
	if len(lines) >= 3 {
		u.fiveHourReset = validateISO(lines[2])
	}
	if len(lines) >= 4 {
		u.sevenDayReset = validateISO(lines[3])
	}
	return u
}

// validateISO returns the input only if it looks like ISO 8601 starting with
// YYYY-MM-DD. Anything else returns empty (matches bash case-statement guard).
func validateISO(s string) string {
	if len(s) < 10 {
		return ""
	}
	if s[4] != '-' || s[7] != '-' {
		return ""
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9} {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
	}
	return s
}

// writeUsageCache writes the 4-line cache format atomically (tmp + rename).
//
// Uses os.CreateTemp for the staging file (unique name per call) so that
// concurrent statusline renders racing to refresh the cache cannot interleave
// writes into a shared `.tmp` file - each gets its own staging file, last
// rename wins. Cache file is mode 0o600 (operational metadata, no other
// user on a shared host needs to read it).
func writeUsageCache(path string, u usage) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "usage-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}
	// CreateTemp on POSIX defaults to 0o600; explicit Chmod for portability.
	if err := os.Chmod(tmp, 0o600); err != nil {
		cleanup()
		return err
	}

	w := bufio.NewWriter(f)
	if u.fiveHourOK {
		_, _ = w.WriteString(strconv.Itoa(u.fiveHour) + "\n")
	} else {
		_, _ = w.WriteString("\n")
	}
	if u.sevenDayOK {
		_, _ = w.WriteString(strconv.Itoa(u.sevenDay) + "\n")
	} else {
		_, _ = w.WriteString("\n")
	}
	_, _ = w.WriteString(u.fiveHourReset + "\n")
	_, _ = w.WriteString(u.sevenDayReset + "\n")
	if err := w.Flush(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// parseISO normalizes an ISO-8601 timestamp — strips fractional seconds, a
// ±HH:MM offset, and a trailing Z — then parses it as UTC. ok=false on
// blank/invalid input. Shared by computeDelta (future) and relAgo (past) so the
// fragile normalization lives in one place (mirrors the old bash sed sequence).
func parseISO(iso string) (time.Time, bool) {
	if iso == "" {
		return time.Time{}, false
	}
	s := iso
	if i := strings.IndexByte(s, '.'); i != -1 {
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		s = s[:i] + s[j:]
	}
	s = strings.TrimSuffix(s, "Z")
	if len(s) >= 6 && (s[len(s)-6] == '+' || s[len(s)-6] == '-') && s[len(s)-3] == ':' {
		s = s[:len(s)-6]
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// computeDelta returns human-readable time-until-reset (e.g. "2d 3h", "45m").
// Empty for invalid timestamps, "now" for past/elapsed.
func computeDelta(iso string) string {
	t, ok := parseISO(iso)
	if !ok {
		return ""
	}
	diff := time.Until(t)
	if diff <= 0 {
		return "now"
	}
	days := int(diff.Hours()) / 24
	hours := int(diff.Hours()) % 24
	minutes := int(diff.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
