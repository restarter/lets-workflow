package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DetectInsideWorktreeAt returns (insideWorktree, mainRepoRoot) for the
// given path; passing path=="" inspects the current working directory.
// mainRepoRoot is empty when not inside any git repo. This is the single
// canonical worktree detector used across the CLI — both cwd-based callers
// (`lets init`, `lets update`) and path-anchored callers (`lets worktree
// create/info`) route through it.
//
// Mechanism: `git rev-parse --git-dir` points at the worktree's
// `<main>/.git/worktrees/<name>` when inside a worktree, while
// `--git-common-dir` always points at the main repo's `.git`. They differ
// iff we're inside a worktree. Normalize both paths via filepath.Abs before
// comparing — git returns absolute when run from repo root and relative
// (e.g. "../../.git") when run from a subdirectory, so without normalization
// a subfolder of the main repo would be false-positively classified as a
// worktree (Phase 4b smoke-test regression preserved here). Substring-match
// on "/worktrees/" was tried and rejected (path could legitimately contain
// that segment).
func DetectInsideWorktreeAt(path string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resolve := func(arg string) (string, bool) {
		args := []string{"rev-parse", arg}
		if path != "" {
			args = []string{"-C", path, "rev-parse", arg}
		}
		out, err := exec.CommandContext(ctx, "git", args...).Output()
		if err != nil {
			return "", false
		}
		raw := strings.TrimSpace(string(out))
		base := path
		if base == "" {
			base, _ = os.Getwd()
		}
		if !filepath.IsAbs(raw) && base != "" {
			raw = filepath.Join(base, raw)
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", false
		}
		return abs, true
	}

	gitDir, ok := resolve("--git-dir")
	if !ok {
		return false, ""
	}
	commonDir, ok := resolve("--git-common-dir")
	if !ok {
		return false, ""
	}
	// commonDir resolves to <main>/.git — parent is the main repo root.
	mainRoot := filepath.Dir(commonDir)
	return gitDir != commonDir, mainRoot
}

// LinkedWorktreeMain resolves the main checkout of a linked worktree rooted at root
// WITHOUT forking git (render and hook paths): <root>/.git must be a file whose
// `gitdir:` line points at <common>/worktrees/<name>, and that directory's
// `commondir` file names the common dir, whose parent is the main checkout. A `.git`
// directory (a main checkout), a missing `commondir` (a submodule's `.git` file), or
// a common dir not named `.git` all return ok=false.
func LinkedWorktreeMain(root string) (string, bool) {
	fi, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil || !fi.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(root, ".git"))
	if err != nil {
		return "", false
	}
	var gitdir string
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:"); ok {
			gitdir = strings.TrimSpace(v)
			break
		}
	}
	if gitdir == "" {
		return "", false
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	common, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return "", false
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitdir, commonDir)
	}
	commonDir = filepath.Clean(commonDir)
	if filepath.Base(commonDir) != ".git" {
		return "", false
	}
	return filepath.Dir(commonDir), true
}
