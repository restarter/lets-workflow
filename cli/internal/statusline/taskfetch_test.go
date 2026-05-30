package statusline

import (
	"os"
	"path/filepath"
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
	}
	for _, tt := range tests {
		if got := sanitizeField(tt.in); got != tt.want {
			t.Errorf("sanitizeField(%q)=%q, want %q", tt.in, got, tt.want)
		}
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
