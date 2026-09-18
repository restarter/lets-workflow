//go:build unix

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrcaCard_NotEnabledWithoutLauncher(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	_ = os.MkdirAll(filepath.Join(dir, ".lets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".lets", ".env"), []byte("LETS_LAUNCHER=terminal\n"), 0o644)
	t.Setenv("ORCA_WORKTREE_ID", "r::"+dir)
	t.Chdir(dir)
	root := NewRootCmd()
	root.SetArgs([]string{"orca", "card", "--phase", "start", "--comment", "x", "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"reason": "orca_not_enabled"`) {
		t.Errorf("LETS_LAUNCHER=terminal must not reach Orca:\n%s", out.String())
	}
}
