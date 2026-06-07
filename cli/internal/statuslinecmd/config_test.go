package statuslinecmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// readRawSettings reads settings.local.json back into a map for assertions.
func readRawSettings(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(settingsPath(root))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.local.json is not valid JSON: %v\n%s", err, data)
	}
	return m
}

func statusLineCommand(t *testing.T, m map[string]any) string {
	t.Helper()
	sl, _ := m["statusLine"].(map[string]any)
	if sl == nil {
		t.Fatalf("statusLine key missing: %v", m)
	}
	cmd, _ := sl["command"].(string)
	return cmd
}

func TestApply_CreatesSettingsWithFlags(t *testing.T) {
	root := t.TempDir()
	res, err := Apply(root, Flags{Light: true, NoTip: true}, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.OK || !res.Changed {
		t.Fatalf("expected ok+changed, got %+v", res)
	}
	if got := statusLineCommand(t, readRawSettings(t, root)); got != "lets statusline --light --no-tip" {
		t.Errorf("command = %q, want %q", got, "lets statusline --light --no-tip")
	}
}

func TestApply_PreservesOtherKeys(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed a local settings file with a sibling key that must survive the merge.
	seed := `{"permissions":{"allow":["Bash(ls:*)"]},"statusLine":{"type":"command","command":"lets statusline"}}`
	if err := os.WriteFile(settingsPath(root), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, Flags{Compact: true}, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	m := readRawSettings(t, root)
	if _, ok := m["permissions"]; !ok {
		t.Errorf("permissions key was dropped: %v", m)
	}
	if got := statusLineCommand(t, m); got != "lets statusline --compact" {
		t.Errorf("command = %q, want %q", got, "lets statusline --compact")
	}
}

func TestShow_RoundTrip(t *testing.T) {
	root := t.TempDir()
	want := Flags{Light: true, NoDir: true, NoTask: true}
	if _, err := Apply(root, want, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	res, err := Show(root)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if res.Appearance == nil || *res.Appearance != *want.appearance() {
		t.Errorf("Show appearance = %+v, want %+v", res.Appearance, want.appearance())
	}
	if res.Changed {
		t.Errorf("Show must not report Changed")
	}
}

func TestShow_DefaultsWhenAbsent(t *testing.T) {
	root := t.TempDir()
	res, err := Show(root)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !res.OK || res.Command != "lets statusline" {
		t.Errorf("absent settings should report defaults, got %+v", res)
	}
}

func TestApply_ForeignRefusedThenForced(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"statusLine":{"type":"command","command":"my-custom-statusline --x"}}`
	if err := os.WriteFile(settingsPath(root), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	// Without --force: refused, foreign command left intact.
	res, err := Apply(root, Flags{Light: true}, false)
	var e *Error
	if !errors.As(err, &e) || e.Kind != "foreign_statusline" {
		t.Fatalf("expected foreign_statusline error, got %v", err)
	}
	if res.OK {
		t.Errorf("foreign refusal should be ok=false")
	}
	if got := statusLineCommand(t, readRawSettings(t, root)); got != "my-custom-statusline --x" {
		t.Errorf("foreign command must be left intact, got %q", got)
	}
	// With --force: overwritten.
	if _, err := Apply(root, Flags{Light: true}, true); err != nil {
		t.Fatalf("Apply --force: %v", err)
	}
	if got := statusLineCommand(t, readRawSettings(t, root)); got != "lets statusline --light" {
		t.Errorf("--force should overwrite, got %q", got)
	}
}

func TestApply_MalformedRefused(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath(root), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(root, Flags{Light: true}, false)
	var e *Error
	if !errors.As(err, &e) || e.Kind != "malformed_settings" {
		t.Fatalf("expected malformed_settings error, got %v", err)
	}
}

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in     string
		want   Flags
		wantOK bool
	}{
		{"lets statusline", Flags{}, true},
		{"lets statusline --light --no-tip", Flags{Light: true, NoTip: true}, true},
		{"lets statusline --compact --no-dir --no-task", Flags{Compact: true, NoDir: true, NoTask: true}, true},
		{"my-custom-bar", Flags{}, false},
		{"lets statusline --unknown", Flags{}, false},
		{"", Flags{}, false},
	}
	for _, c := range cases {
		got, ok := parseCommand(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseCommand(%q) = (%+v, %v), want (%+v, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
