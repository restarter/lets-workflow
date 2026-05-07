package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadUsageCache_FullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage")
	body := "42\n67\n2026-05-08T10:30:00Z\n2026-05-14T10:30:00Z\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	u := readUsageCache(path)
	if u.fiveHour != 42 || !u.fiveHourOK {
		t.Errorf("fiveHour: got %d (ok=%v), want 42", u.fiveHour, u.fiveHourOK)
	}
	if u.sevenDay != 67 || !u.sevenDayOK {
		t.Errorf("sevenDay: got %d (ok=%v), want 67", u.sevenDay, u.sevenDayOK)
	}
	if u.fiveHourReset != "2026-05-08T10:30:00Z" {
		t.Errorf("fiveHourReset: got %q", u.fiveHourReset)
	}
	if u.sevenDayReset != "2026-05-14T10:30:00Z" {
		t.Errorf("sevenDayReset: got %q", u.sevenDayReset)
	}
}

func TestReadUsageCache_Missing(t *testing.T) {
	u := readUsageCache(filepath.Join(t.TempDir(), "does-not-exist"))
	if u.fiveHourOK || u.sevenDayOK {
		t.Errorf("expected zero usage for missing file, got %+v", u)
	}
	if !u.mtime.IsZero() {
		t.Errorf("expected zero mtime, got %v", u.mtime)
	}
}

func TestReadUsageCache_MalformedTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage")
	body := "10\n20\nnot-an-iso\nalso-bad\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	u := readUsageCache(path)
	if u.fiveHourReset != "" || u.sevenDayReset != "" {
		t.Errorf("expected empty timestamps, got %q / %q", u.fiveHourReset, u.sevenDayReset)
	}
	if u.fiveHour != 10 || u.sevenDay != 20 {
		t.Errorf("percentages should still parse: got %d/%d", u.fiveHour, u.sevenDay)
	}
}

func TestWriteReadUsageCache_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage")
	in := usage{
		fiveHour: 50, fiveHourOK: true,
		sevenDay: 25, sevenDayOK: true,
		fiveHourReset: "2026-05-08T10:30:00Z",
		sevenDayReset: "2026-05-14T10:30:00Z",
	}
	if err := writeUsageCache(path, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := readUsageCache(path)
	if out.fiveHour != in.fiveHour || out.sevenDay != in.sevenDay {
		t.Errorf("percentage roundtrip: got %d/%d, want %d/%d",
			out.fiveHour, out.sevenDay, in.fiveHour, in.sevenDay)
	}
	if out.fiveHourReset != in.fiveHourReset || out.sevenDayReset != in.sevenDayReset {
		t.Errorf("timestamp roundtrip mismatch")
	}
}

func TestUsage_Fresh(t *testing.T) {
	tests := []struct {
		name string
		mt   time.Time
		ttl  time.Duration
		want bool
	}{
		{"zero time -> stale", time.Time{}, time.Minute, false},
		{"recent -> fresh", time.Now().Add(-30 * time.Second), time.Minute, true},
		{"old -> stale", time.Now().Add(-5 * time.Minute), time.Minute, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := usage{mtime: tt.mt}
			if got := u.fresh(tt.ttl); got != tt.want {
				t.Errorf("fresh: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeDelta(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name          string
		input         string
		wantEmpty     bool
		wantSubstring string
	}{
		{"empty", "", true, ""},
		{"invalid", "garbage", true, ""},
		{"past -> now", now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z"), false, "now"},
		{"days ahead", now.Add(48 * time.Hour).Format("2006-01-02T15:04:05Z"), false, "d"},
		{"hours ahead", now.Add(3 * time.Hour).Format("2006-01-02T15:04:05Z"), false, "h"},
		{"with fractional sec", now.Add(2 * time.Hour).Format("2006-01-02T15:04:05.000Z"), false, "h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDelta(tt.input)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSubstring) {
				t.Errorf("expected substring %q, got %q", tt.wantSubstring, got)
			}
		})
	}
}

func TestValidateISO(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"too-short", ""},
		{"abcd-ef-ghT12:00:00", ""},
		{"2026-05-08T10:30:00Z", "2026-05-08T10:30:00Z"},
		{"2026-05-08", "2026-05-08"},
	}
	for _, tt := range tests {
		if got := validateISO(tt.in); got != tt.want {
			t.Errorf("validateISO(%q): got %q, want %q", tt.in, got, tt.want)
		}
	}
}
