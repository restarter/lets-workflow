//go:build unix

package initcmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
)

// TestEnsureGitignore_ConcurrentSafe spawns 50 goroutines that all try to
// add the same entries; flock + integrity check must converge to a single
// set with no interleaved garbage. Run under -race.
func TestEnsureGitignore_ConcurrentSafe(t *testing.T) {
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
