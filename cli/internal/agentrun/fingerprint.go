//go:build unix

package agentrun

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// gitOut is the git seam (tests replace it).
var gitOut = func(dir string, args ...string) ([]byte, error) {
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
}

// Fingerprint identifies the checkout's content state: status, the tracked diff
// against HEAD, and every untracked file's name, size and mtime. It changes on a
// second edit to an already modified file, which `git status` alone cannot show.
// Ignored paths (.lets/) are outside it, so LETS's own outputs never count.
func Fingerprint(dir string) string {
	h := sha256.New()
	for _, args := range [][]string{{"status", "--porcelain=v1", "-z"}, {"diff", "HEAD", "--binary"}} {
		out, _ := gitOut(dir, args...)
		h.Write(out)
		h.Write([]byte{0})
	}
	untracked, _ := gitOut(dir, "ls-files", "--others", "--exclude-standard", "-z")
	for _, name := range bytes.Split(untracked, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		if fi, err := os.Lstat(filepath.Join(dir, string(name))); err == nil {
			fmt.Fprintf(h, "%s\x00%d\x00%d\x00", name, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
