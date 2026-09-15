package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLinkedWorktreeMain(t *testing.T) {
	main := real(t, t.TempDir())
	run(t, main, "init", "-q", "-b", "main")
	run(t, main, "commit", "-q", "--allow-empty", "-m", "init")

	// absolute gitdir: (what `git worktree add` writes)
	wt := filepath.Join(real(t, t.TempDir()), "wt")
	run(t, main, "worktree", "add", "-q", "-b", "x", wt)
	if got, ok := LinkedWorktreeMain(wt); !ok || got != main {
		t.Errorf("absolute gitdir: %q %v, want %q", got, ok, main)
	}

	// relative gitdir:
	rel := filepath.Join(main, "sub-wt")
	run(t, main, "worktree", "add", "-q", "-b", "y", rel)
	gitdir, err := os.ReadFile(filepath.Join(rel, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Clean(string(gitdir[len("gitdir: ") : len(gitdir)-1]))
	relPath, _ := filepath.Rel(rel, abs)
	if err := os.WriteFile(filepath.Join(rel, ".git"), []byte("gitdir: "+relPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := LinkedWorktreeMain(rel); !ok || got != main {
		t.Errorf("relative gitdir: %q %v, want %q", got, ok, main)
	}

	// the main checkout itself is not a linked worktree
	if _, ok := LinkedWorktreeMain(main); ok {
		t.Error("a .git directory must return ok=false")
	}

	// a submodule-style .git file without commondir
	sub := real(t, t.TempDir())
	modules := filepath.Join(sub, "modules", "m")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	sm := filepath.Join(sub, "m")
	if err := os.MkdirAll(sm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sm, ".git"), []byte("gitdir: ../modules/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LinkedWorktreeMain(sm); ok {
		t.Error("a submodule .git file (no commondir) must return ok=false")
	}

	// DetectInsideWorktreeAt agrees
	if in, m := DetectInsideWorktreeAt(wt); !in || real(t, m) != main {
		t.Errorf("DetectInsideWorktreeAt(wt) = %v %q", in, m)
	}
}
