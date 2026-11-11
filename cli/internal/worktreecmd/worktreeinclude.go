//go:build unix

package worktreecmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// writeExcludes is the exclude writer copyWorktreeInclude calls - a test seam.
var writeExcludes = ensureWorktreeExcludes

// copyWorktreeInclude copies the main checkout's local files into a new worktree
// (lets-41cpf), honouring Claude Code's native .worktreeinclude: a file is copied
// when it matches a .worktreeinclude pattern (gitignore syntax) AND git ignores it
// in the main checkout - tracked content already arrives through git. It COPIES,
// never links, and never overwrites: an identical destination is silent, a
// different one is kept with a warning. Runtime state (.env, node_modules, caches)
// is per-worktree by design - it is copied only when a repo lists it, never by
// default. Never fatal: every problem is a warn step. The copied paths go to the
// shared info/exclude, because a branch's committed .gitignore may lack the rule
// and would show them as untracked.
func copyWorktreeInclude(ctx context.Context, mainRoot, wtRoot string) []Step {
	spec := filepath.Join(mainRoot, ".worktreeinclude")
	if _, err := os.Stat(spec); err != nil {
		return nil // no file: nothing requested
	}
	out, err := exec.CommandContext(ctx, "git", "-C", mainRoot, "ls-files", "--others", "--ignored", "--exclude-from="+spec, "-z").Output()
	if err != nil {
		return []Step{{Status: StepWarn, Message: "worktreeinclude: could not list matching files: " + err.Error()}}
	}
	candidates := splitNUL(out)
	if len(candidates) == 0 {
		return []Step{{Status: StepOK, Message: "worktreeinclude: no matching local files"}}
	}
	ignored := gitIgnored(ctx, mainRoot, candidates) // git check-ignore --stdin -z
	// Every write goes through an os.Root on the worktree: path resolution that would
	// leave it - a symlinked parent, even one swapped in after a check - fails
	// atomically (Go's openat-based containment), so no lexical check is trusted.
	root, err := os.OpenRoot(wtRoot)
	if err != nil {
		return []Step{{Status: StepWarn, Message: "worktreeinclude: cannot open the worktree: " + err.Error()}}
	}
	defer func() { _ = root.Close() }()
	var steps []Step
	var copied, present []string
	for _, rel := range candidates {
		if !ignored[rel] || !cleanRel(rel) {
			continue
		}
		src, dst := filepath.Join(mainRoot, rel), filepath.Join(wtRoot, rel)
		fi, err := os.Lstat(src)
		if err != nil || !fi.Mode().IsRegular() {
			steps = append(steps, Step{Status: StepWarn, Message: "worktreeinclude: skipped " + rel + " (not a regular file)"})
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			if sameContent(src, dst) {
				present = append(present, rel) // already there: still owed its exclude (a re-adopt repairs a failed one)
			} else {
				steps = append(steps, Step{Status: StepWarn, Message: "worktreeinclude: kept the existing " + rel + " (differs from the main checkout's)"})
			}
			continue
		}
		if err := copyRegular(root, src, rel, fi.Mode().Perm()); err != nil {
			if errors.Is(err, fs.ErrExist) && sameContent(src, dst) {
				present = append(present, rel)
			} else if errors.Is(err, fs.ErrExist) {
				steps = append(steps, Step{Status: StepWarn, Message: "worktreeinclude: kept the existing " + rel + " (differs from the main checkout's)"})
			} else {
				steps = append(steps, Step{Status: StepWarn, Message: "worktreeinclude: could not copy " + rel + ": " + err.Error()})
			}
			continue
		}
		copied = append(copied, rel)
	}
	if owed := append(append([]string{}, copied...), present...); len(owed) > 0 {
		patterns := make([]string, 0, len(owed))
		for _, rel := range owed {
			patterns = append(patterns, literalExclude(rel))
		}
		if err := writeExcludes(ctx, mainRoot, patterns); err != nil {
			steps = append(steps, Step{Status: StepWarn, Message: "worktreeinclude: could not update info/exclude: " + err.Error()})
		}
	}
	if len(copied) > 0 {
		steps = append([]Step{{Status: StepOK, Message: fmt.Sprintf("worktreeinclude: copied %d file(s): %s", len(copied), strings.Join(copied, ", "))}}, steps...)
	}
	return steps
}

// splitNUL splits git's -z output; empty entries are dropped.
func splitNUL(b []byte) []string {
	var out []string
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}

// gitIgnored reports which paths git ignores in root (`git check-ignore --stdin -z`).
// Exit 1 means none is ignored; any failure yields an empty map (nothing copied).
func gitIgnored(ctx context.Context, root string, paths []string) map[string]bool {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "--stdin", "-z")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	set := map[string]bool{}
	if err != nil {
		return set
	}
	for _, p := range splitNUL(out) {
		set[p] = true
	}
	return set
}

// cleanRel accepts a relative, already-clean path that stays inside the worktree
// and touches neither .git nor .lets, with no line break in it.
func cleanRel(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) || filepath.Clean(rel) != rel || strings.ContainsAny(rel, "\n\r") {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if p == ".." {
			return false
		}
	}
	return parts[0] != ".git" && parts[0] != ".lets"
}

// sameContent reports whether a and b are readable and byte-identical.
func sameContent(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil || fa.Size() != fb.Size() {
		return false
	}
	da, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(da, db)
}

// literalExclude is a root-anchored, literal ignore entry for rel: "/" + rel with
// every glob character and backslash escaped, and each trailing space escaped (git
// drops unescaped trailing spaces). The leading "/" already neutralizes a leading
// "#" or "!" and pins the entry to that one path.
func literalExclude(rel string) string {
	var b strings.Builder
	b.WriteByte('/')
	for _, r := range rel {
		switch r {
		case '\\', '*', '?', '[', ']':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	s := b.String()
	trimmed := strings.TrimRight(s, " ")
	if n := len(s) - len(trimmed); n > 0 {
		return trimmed + strings.Repeat("\\ ", n)
	}
	return s
}

// copyRegular copies src to rel inside root. Containment and no-replace are both
// the filesystem's: os.Root refuses any resolution that leaves the worktree, and
// O_EXCL never replaces an existing or concurrently created leaf (fs.ErrExist). On
// an error after the open, only the file this call created is removed.
func copyRegular(root *os.Root, src, rel string, perm os.FileMode) (err error) {
	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = root.Remove(rel)
		return err
	}
	if err = out.Close(); err != nil {
		_ = root.Remove(rel)
		return err
	}
	return nil
}
