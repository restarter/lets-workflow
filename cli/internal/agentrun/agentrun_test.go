//go:build unix

package agentrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeToken is assembled, never one literal: push protection reads test files too.
var fakeToken = "ghp" + "_" + "abcdefghijklmnopqrstuvwxyz0123456789"

func TestLookup(t *testing.T) {
	if p, ok := Lookup("codex"); !ok || p.Name() != "codex" {
		t.Errorf("codex: %v %v", p, ok)
	}
	if _, isCodex := mustLookup(t, "codex").(codex); !isCodex {
		t.Error("codex must be the rollout provider")
	}
	for _, name := range []string{"antigravity", "claude"} {
		p := mustLookup(t, name)
		if _, isFile := p.(reportFile); !isFile || p.Name() != name {
			t.Errorf("%s: %T %q", name, p, p.Name())
		}
	}
	for _, bad := range []string{"", "Codex", "a b", "-x", strings.Repeat("a", 40)} {
		if _, ok := Lookup(bad); ok {
			t.Errorf("%q must not resolve", bad)
		}
	}
}

func mustLookup(t *testing.T, name string) Provider {
	t.Helper()
	p, ok := Lookup(name)
	if !ok {
		t.Fatalf("%s not found", name)
	}
	return p
}

func TestWriteNew_NeverOverwrites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.md")
	if err := writeNew(p, "one"); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(p, "two"); err == nil {
		t.Error("second write must fail")
	}
	b, _ := os.ReadFile(p)
	fi, _ := os.Stat(p)
	if string(b) != "one" || fi.Mode().Perm() != 0o600 {
		t.Errorf("content %q mode %v", b, fi.Mode().Perm())
	}
}

func TestClean_RedactsAndCaps(t *testing.T) {
	got := clean("token " + fakeToken + " at https://u:p@h/x")
	if strings.Contains(got, fakeToken) || strings.Contains(got, "u:p@") {
		t.Errorf("not redacted: %q", got)
	}
	big := clean(strings.Repeat("a", 200<<10))
	if len(big) > ReportCap+64 || !strings.Contains(big, "…[truncated") {
		t.Errorf("not capped: %d bytes", len(big))
	}
}
