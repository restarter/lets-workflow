package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameDir(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	if !SameDir(link, a) {
		t.Error("a symlink must equal its target")
	}
	if SameDir(a, b) {
		t.Error("distinct dirs must differ")
	}
}
