//go:build unix

package agentrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write(t, filepath.Join(dir, ".gitignore"), ".lets/\n")
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprint(t *testing.T) {
	dir := gitRepo(t)
	clean := Fingerprint(dir)
	if clean != Fingerprint(dir) {
		t.Fatal("fingerprint is not stable")
	}
	write(t, filepath.Join(dir, "a.txt"), "two\n")
	modified := Fingerprint(dir)
	if modified == clean {
		t.Error("a modified file must change the fingerprint")
	}
	write(t, filepath.Join(dir, "a.txt"), "three\n")
	if Fingerprint(dir) == modified {
		t.Error("a second edit to an already modified file must change it (the v1 gap)")
	}
	again := Fingerprint(dir)
	write(t, filepath.Join(dir, "new.txt"), "x")
	if Fingerprint(dir) == again {
		t.Error("a new untracked file must change it")
	}
	withNew := Fingerprint(dir)
	write(t, filepath.Join(dir, ".lets", "handoffs", "b-report.md"), "report")
	if Fingerprint(dir) != withNew {
		t.Error("a file under the ignored .lets/ must not change it")
	}
}
