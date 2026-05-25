//go:build unix

package initcmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
)

// TestEnsureGitignore_IdempotentSameEntries exercises 50 goroutines that all
// request the SAME entries. AtomicWriteBytes (temp+rename) alone produces a
// coherent final file via last-writer-wins, so this is the deduplication
// test — NOT the flock-correctness test. The proper flock-correctness test
// is TestEnsureGitignore_ConcurrentDistinctEntries below.
func TestEnsureGitignore_IdempotentSameEntries(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := initcmd.EnsureGitignore(dir, []string{".lets", ".worktrees/"}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	got := string(b)
	if c := strings.Count(got, ".lets\n"); c != 1 {
		t.Errorf(".lets count=%d, want 1; full:\n%s", c, got)
	}
	if c := strings.Count(got, ".worktrees/\n"); c != 1 {
		t.Errorf(".worktrees/ count=%d, want 1; full:\n%s", c, got)
	}
	// No interleaved garbage: lines must not have leading whitespace.
	for i, line := range strings.Split(got, "\n") {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			t.Errorf("line %d has leading whitespace: %q (full:\n%s)", i, line, got)
		}
	}
}

// TestEnsureGitignore_ConcurrentDistinctEntries is the actual flock-
// correctness test. Each of 50 goroutines requests a DIFFERENT entry; in
// the absence of flock, last-writer-wins drops contributions from every
// reader that observed an older file. With flock + read-after-write
// integrity check, all 50 entries must survive in the final file.
//
// To stress the race fully we add a small artificial pause between read
// and write (via the EnsureGitignore implementation's natural latency
// when the file grows) and rely on Go's scheduler interleaving. Under
// -race the test exposes any data race in the helper itself.
func TestEnsureGitignore_ConcurrentDistinctEntries(t *testing.T) {
	dir := t.TempDir()
	const N = 50
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry := fmt.Sprintf(".lets-goroutine-%02d", i)
			if err := initcmd.EnsureGitignore(dir, []string{entry}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	got := string(b)
	missing := 0
	for i := 0; i < N; i++ {
		want := fmt.Sprintf(".lets-goroutine-%02d\n", i)
		if !strings.Contains(got, want) {
			t.Errorf("entry %q missing — flock failed to serialize writers", strings.TrimSpace(want))
			missing++
		}
	}
	if missing > 0 {
		t.Logf("final .gitignore (%d entries missing):\n%s", missing, got)
	}
}
