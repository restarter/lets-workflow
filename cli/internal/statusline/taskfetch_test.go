package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskStatusLine(t *testing.T) {
	tests := []struct {
		name    string
		taskID  string
		json    string
		want    string
		wantErr bool
	}{
		{
			name:   "full issue with comments",
			taskID: "lets-ds6bc",
			json:   `[{"id":"lets-ds6bc","title":"StatusLine 2.0","comments":[{"created_at":"2026-05-30T10:00:00Z"},{"created_at":"2026-05-30T18:50:07Z"}]}]`,
			want:   "lets-ds6bc|StatusLine 2.0|2|2026-05-30T18:50:07Z",
		},
		{
			name:   "no comments",
			taskID: "lets-aaaaa",
			json:   `[{"id":"lets-aaaaa","title":"Fresh task","comments":[]}]`,
			want:   "lets-aaaaa|Fresh task|0|",
		},
		{
			name:   "null comments field",
			taskID: "lets-aaaaa",
			json:   `[{"id":"lets-aaaaa","title":"No comments key"}]`,
			want:   "lets-aaaaa|No comments key|0|",
		},
		{
			name:   "title with pipe and newline is sanitized",
			taskID: "lets-bbbbb",
			json:   `[{"id":"lets-bbbbb","title":"a|b\nc","comments":[]}]`,
			want:   "lets-bbbbb|a/b c|0|",
		},
		{
			name:   "invalid last-comment iso dropped",
			taskID: "lets-ccccc",
			json:   `[{"id":"lets-ccccc","title":"T","comments":[{"created_at":"not-a-date"}]}]`,
			want:   "lets-ccccc|T|1|",
		},
		{name: "empty array", taskID: "lets-ddddd", json: `[]`, wantErr: true},
		{name: "id mismatch", taskID: "lets-ddddd", json: `[{"id":"lets-other","title":"X"}]`, wantErr: true},
		{name: "malformed json", taskID: "lets-ddddd", json: `not json`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := taskStatusLine(tt.taskID, []byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeField(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain title", "plain title"},
		{"a|b", "a/b"},
		{"line1\nline2", "line1 line2"},
		{"carriage\rreturn", "carriage return"},
		{"  trim me  ", "trim me"},
		{"", ""},
		// Terminal-escape injection: ESC + C0 controls + DEL all fold to space.
		{"evil\x1b[2Jclear", "evil [2Jclear"},
		{"osc\x1b]0;pwn\x07end", "osc ]0;pwn end"},
		{"tab\there", "tab here"},
		{"del\x7fbyte", "del byte"},
	}
	for _, tt := range tests {
		if got := sanitizeField(tt.in); got != tt.want {
			t.Errorf("sanitizeField(%q)=%q, want %q", tt.in, got, tt.want)
		}
		// Hard invariant: no C0 control byte or DEL survives.
		for _, r := range sanitizeField(tt.in) {
			if r < 0x20 || r == 0x7f {
				t.Errorf("sanitizeField(%q) leaked control byte %U", tt.in, r)
			}
		}
	}
}

func TestWriteTaskStatusPlaceholder(t *testing.T) {
	dir := t.TempDir()
	if got := cachedTaskID(dir); got != "" {
		t.Errorf("empty cache: cachedTaskID=%q, want empty", got)
	}
	if err := writeTaskStatusPlaceholder(dir, "lets-ds6bc"); err != nil {
		t.Fatalf("placeholder: %v", err)
	}
	if got := cachedTaskID(dir); got != "lets-ds6bc" {
		t.Errorf("cachedTaskID=%q, want lets-ds6bc", got)
	}
	// Placeholder is fresh for its id (debounce) and renders as id-only.
	if !taskStatusFresh(dir, "lets-ds6bc", taskStatusTTL) {
		t.Error("placeholder should read fresh for its own id")
	}
	title, notes, comment, ok := readTaskStatus(dir, "lets-ds6bc")
	if !ok || title != "" || notes != 0 || comment != "" {
		t.Errorf("placeholder should be id-only: ok=%v title=%q notes=%d comment=%q", ok, title, notes, comment)
	}
}

func TestTaskStatusFresh(t *testing.T) {
	id := "lets-ds6bc"
	write := func(dir, content string, age time.Duration) {
		p := filepath.Join(dir, "task-status")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		when := time.Now().Add(-age)
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	t.Run("missing file", func(t *testing.T) {
		if taskStatusFresh(t.TempDir(), id, taskStatusTTL) {
			t.Error("missing file should be stale")
		}
	})
	t.Run("fresh same id", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "lets-ds6bc|Title|3|x", 1*time.Second)
		if !taskStatusFresh(dir, id, taskStatusTTL) {
			t.Error("recent same-id file should be fresh")
		}
	})
	t.Run("stale by age", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "lets-ds6bc|Title|3|x", taskStatusTTL+time.Minute)
		if taskStatusFresh(dir, id, taskStatusTTL) {
			t.Error("old file should be stale")
		}
	})
	t.Run("different id", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "lets-other|Title|3|x", 1*time.Second)
		if taskStatusFresh(dir, id, taskStatusTTL) {
			t.Error("mismatched-id file should be stale")
		}
	})
}

func TestWriteTaskStatusCache_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	line := "lets-ds6bc|StatusLine 2.0|7|2026-05-30T18:50:07Z"
	if err := writeTaskStatusCache(filepath.Join(dir, "task-status"), line); err != nil {
		t.Fatalf("write: %v", err)
	}
	title, notes, comment, ok := readTaskStatus(dir, "lets-ds6bc")
	if !ok || title != "StatusLine 2.0" || notes != 7 || comment != "2026-05-30T18:50:07Z" {
		t.Errorf("round-trip mismatch: ok=%v title=%q notes=%d comment=%q", ok, title, notes, comment)
	}
	// mode 0o600
	info, err := os.Stat(filepath.Join(dir, "task-status"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode=%o, want 600", info.Mode().Perm())
	}
}

func TestFetchAndCacheTaskStatus_RejectsBadID(t *testing.T) {
	// Guard fires before any exec, so no bd needed. Leading "-" or empty must
	// be rejected (argv flag-smuggling defense).
	for _, id := range []string{"", "-rf", "--json"} {
		if err := fetchAndCacheTaskStatus(t.TempDir(), id); err == nil {
			t.Errorf("fetchAndCacheTaskStatus(%q) = nil, want error", id)
		}
	}
}

// TestStripControl: the shared escape-injection barrier neutralizes C0, ESC,
// DEL and C1 control bytes while keeping printable text (incl. the now-inert
// "[2J" that loses its ESC introducer).
func TestStripControl(t *testing.T) {
	in := "ok\x1b[2J\x07\x7f end"
	got := stripControl(in)
	for _, r := range got {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Errorf("stripControl leaked control %U in %q", r, got)
		}
	}
	if !strings.Contains(got, "ok") || !strings.Contains(got, "end") || !strings.Contains(got, "[2J") {
		t.Errorf("stripControl dropped printable text: %q", got)
	}
}

// TestTaskStatusLine_LatestComment: the last-comment timestamp is the MAX by
// parsed time, not the last array element (bd output ordering isn't a contract).
func TestTaskStatusLine_LatestComment(t *testing.T) {
	js := []byte(`[{"id":"lets-ds6bc","title":"T","comments":[` +
		`{"created_at":"2026-06-01T10:00:00Z"},` +
		`{"created_at":"2026-06-03T10:00:00Z"},` + // latest, NOT last in the array
		`{"created_at":"2026-06-02T10:00:00Z"}]}]`)
	line, err := taskStatusLine("lets-ds6bc", js)
	if err != nil {
		t.Fatalf("taskStatusLine: %v", err)
	}
	if !strings.HasSuffix(line, "|2026-06-03T10:00:00Z") {
		t.Errorf("want latest comment 2026-06-03 by max(created_at), got %q", line)
	}
}
