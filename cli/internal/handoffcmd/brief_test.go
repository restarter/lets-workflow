//go:build unix

package handoffcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// handoffsDir makes <root>/.lets/handoffs under a temp root and returns both.
func handoffsDir(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, ".lets", "handoffs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckBrief(t *testing.T) {
	root := filepath.Join(t.TempDir(), "w")
	dir := handoffsDir(t, root)
	brief := filepath.Join(dir, "2026-09-18-1600-lets-w5tm5-handoff.md")
	if err := os.WriteFile(brief, []byte("# brief\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckBrief(root, brief); err != nil {
		t.Fatalf("valid brief refused: %v", err)
	}
	plans := filepath.Join(root, ".lets", "plans")
	_ = os.MkdirAll(plans, 0o700)
	_ = os.WriteFile(filepath.Join(plans, "p.md"), []byte("x"), 0o600)
	for name, b := range map[string]string{
		"relative":        ".lets/handoffs/2026-09-18-1600-lets-w5tm5-handoff.md",
		"unclean":         dir + "/../handoffs/2026-09-18-1600-lets-w5tm5-handoff.md",
		"other directory": filepath.Join(plans, "p.md"),
		"space in name":   filepath.Join(dir, "a b.md"),
		"dollar in name":  filepath.Join(dir, "a$b.md"),
		"not markdown":    filepath.Join(dir, "a.txt"),
		"missing":         filepath.Join(dir, "missing.md"),
	} {
		if err := CheckBrief(root, b); err == nil {
			t.Errorf("%s: %q accepted", name, b)
		}
	}
	// A checkout path outside the safe alphabet never reaches the pointer line (the
	// v1 gap: `/work/a;touch x;z` passed and the pointer ran a command in a shell).
	for _, bad := range []string{"/work/a;touch x;z", "/work/a&b", "/work/a|b", "/work/a>b", "/work/a(b", "/work/~a", "/work/a*b", "/work/a\x1bb", "/work/a\nb", "/work/a'b", "/work/a$b", "relative/w"} {
		b := filepath.Join(bad, ".lets", "handoffs", "b.md")
		if err := CheckBrief(bad, b); err == nil || !strings.Contains(err.Error(), "checkout path") {
			t.Errorf("root %q: %v", bad, err)
		}
	}
}

func TestPointer_OneLine(t *testing.T) {
	brief := "/w/.lets/handoffs/2026-09-18-1600-lets-w5tm5-handoff.md"
	p := Pointer(brief)
	if strings.ContainsAny(p, "\r\n") || !strings.Contains(p, brief) {
		t.Errorf("pointer %q", p)
	}
	if OutBase(brief) != "/w/.lets/handoffs/2026-09-18-1600-lets-w5tm5-handoff" {
		t.Errorf("out base %q", OutBase(brief))
	}
}
