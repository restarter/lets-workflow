//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// linkMode tells linkShared who is linking: create (lets just made this worktree,
// so a real .lets inside it can only be a statusline race) or adopt (a worktree
// someone else made, where a real .lets may hold data - never deleted).
type linkMode int

const (
	modeCreate linkMode = iota
	modeAdopt
)

// StoreLink reports one declared store link (trackeradapter `links:`).
type StoreLink struct {
	Path   string `json:"path"`
	Linked bool   `json:"linked"`
}

// linkOutcome is what linkShared did.
type linkOutcome struct {
	Steps      []Step
	StoreLinks []StoreLink
	LetsLinked bool
	MovedAside string // adopt: where a cache-only real .lets was moved ("" when nothing moved)
}

// maxAside bounds the .lets.pre-adopt[-N] search.
const maxAside = 99

// linkShared links the worktree to the main checkout: `.lets` as a whole-dir
// symlink, then every declared store link. It never follows a committed symlink,
// only ever tightens permissions, and in adopt mode never deletes a directory.
func linkShared(ctx context.Context, mainRoot, wtRoot string, links []trackeradapter.Link, mode linkMode) (linkOutcome, error) {
	_ = ctx
	var out linkOutcome
	add := func(status, msg string) { out.Steps = append(out.Steps, Step{Status: status, Message: msg}) }

	moved, err := linkLets(mainRoot, wtRoot, mode, add)
	out.MovedAside = moved
	if err != nil {
		return out, err
	}
	out.LetsLinked = true

	for _, l := range links {
		linked, err := linkStore(mainRoot, wtRoot, l, add)
		out.StoreLinks = append(out.StoreLinks, StoreLink{Path: l.Path, Linked: linked})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// linkLets makes <wt>/.lets a symlink to <main>/.lets. Idempotent: a symlink that
// already resolves to the main .lets is success. A concurrent adopt that moved or
// linked first is re-evaluated, not reported as a conflict.
func linkLets(mainRoot, wtRoot string, mode linkMode, add func(status, msg string)) (string, error) {
	mainLets := filepath.Join(mainRoot, ".lets")
	wtLets := filepath.Join(wtRoot, ".lets")
	moved := ""
	for attempt := 0; attempt < 3; attempt++ {
		fi, err := os.Lstat(wtLets)
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			if fsutil.SameDir(wtLets, mainLets) {
				add(StepOK, ".lets/ already linked")
				return moved, nil
			}
			_ = os.Remove(wtLets)
			add(StepWarn, ".lets/ pointed elsewhere; relinked")
		case err == nil && fi.IsDir() && mode == modeCreate:
			_ = os.RemoveAll(wtLets)
			add(StepWarn, ".lets/ symlinked (defensive: pre-existing real dir removed; statusline race likely)")
		case err == nil && fi.IsDir() && cacheOnly(wtLets):
			aside, mvErr := moveAside(wtLets)
			if errors.Is(mvErr, fs.ErrNotExist) {
				continue // another adopt moved it first: look again
			}
			if mvErr != nil {
				return moved, &Error{Code: ExitLetsDirConflict, Kind: "lets_dir_conflict",
					Message: "could not move " + wtLets + " aside: " + mvErr.Error()}
			}
			moved = aside
			add(StepWarn, "pre-existing real .lets (cache only) moved to "+aside+" - safe to delete by hand")
		case err == nil && fi.IsDir():
			return moved, &Error{Code: ExitLetsDirConflict, Kind: "lets_dir_conflict",
				Message:     wtLets + " is a real directory with non-cache content",
				Remediation: fmt.Sprintf("inspect it, then: mv %s %s.bak && lets worktree adopt", shellQuote(wtLets), shellQuote(wtLets))}
		case err == nil:
			return moved, &Error{Code: ExitLetsDirConflict, Kind: "lets_dir_conflict",
				Message:     wtLets + " is a file, not the .lets symlink",
				Remediation: fmt.Sprintf("inspect it, then: mv %s %s.bak && lets worktree adopt", shellQuote(wtLets), shellQuote(wtLets))}
		case !errors.Is(err, fs.ErrNotExist):
			return moved, &Error{Code: ExitFilesystem, Kind: "lstat_failed", Message: wtLets, Cause: err}
		}
		if err := CreateSymlink(wtLets, mainLets, mainRoot); err != nil {
			var pe *Error
			if errors.As(err, &pe) && errors.Is(pe.Cause, fs.ErrExist) {
				continue // a concurrent adopt linked it: re-check
			}
			return moved, err
		}
		add(StepOK, ".lets/ symlinked")
		return moved, nil
	}
	return moved, &Error{Code: ExitLetsDirConflict, Kind: "lets_dir_conflict", Message: wtLets + " kept changing while linking"}
}

// cacheOnly reports whether every non-directory entry under dir lives under
// cache/ or locks/ (what a statusline or hook writes into a fresh .lets).
func cacheOnly(dir string) bool {
	only := true
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			only = false
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "cache/") && !strings.HasPrefix(rel, "locks/") {
			only = false
			return filepath.SkipAll
		}
		return nil
	})
	return only
}

// moveAside renames dir to the first free name among .lets.pre-adopt,
// .lets.pre-adopt-2 ... -99. os.Rename only - never a delete.
func moveAside(dir string) (string, error) {
	for n := 1; n <= maxAside; n++ {
		name := dir + ".pre-adopt"
		if n > 1 {
			name = fmt.Sprintf("%s.pre-adopt-%d", dir, n)
		}
		if _, err := os.Lstat(name); errors.Is(err, fs.ErrNotExist) {
			return name, os.Rename(dir, name)
		}
	}
	return "", fmt.Errorf("no free .lets.pre-adopt name up to -%d", maxAside)
}

// linkStore links one declared store file. The main side is never followed through a
// symlink; the worktree side goes through os.OpenRoot so a parent committed as a
// symlink cannot carry the link outside the worktree.
func linkStore(mainRoot, wtRoot string, l trackeradapter.Link, add func(status, msg string)) (bool, error) {
	src := filepath.Join(mainRoot, filepath.FromSlash(l.Path))
	fi, err := os.Lstat(src)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		add(StepSkip, fmt.Sprintf("no %s in main repo", l.Path))
		return false, nil
	case err != nil:
		add(StepWarn, fmt.Sprintf("store_link_unsafe: cannot stat %s: %v", l.Path, err))
		return false, nil
	case fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular():
		add(StepWarn, fmt.Sprintf("store_link_unsafe: %s in the main repo is not a regular file; not linked", l.Path))
		return false, nil
	}
	realMain, err1 := filepath.EvalSymlinks(mainRoot)
	realSrc, err2 := filepath.EvalSymlinks(src)
	if rel, err := filepath.Rel(realMain, realSrc); err1 != nil || err2 != nil || err != nil || strings.HasPrefix(rel, "..") {
		add(StepWarn, fmt.Sprintf("store_link_unsafe: %s resolves outside the main repo; not linked", l.Path))
		return false, nil
	}
	if cur := fi.Mode().Perm(); cur&l.Mode != cur {
		if err := os.Chmod(src, cur&l.Mode); err != nil {
			add(StepWarn, fmt.Sprintf("could not tighten %s to %04o: %v", l.Path, cur&l.Mode, err))
		}
	}

	root, err := os.OpenRoot(wtRoot)
	if err != nil {
		return false, &Error{Code: ExitStoreLinkFailed, Kind: "store_link_failed", Message: wtRoot, Cause: err}
	}
	defer func() { _ = root.Close() }()

	rel := filepath.FromSlash(l.Path)
	// Parents: create missing ones 0700; leave pre-existing ones (and their modes) alone.
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	cur := ""
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		pfi, err := root.Lstat(cur)
		switch {
		case err == nil && pfi.Mode()&os.ModeSymlink != 0:
			add(StepWarn, fmt.Sprintf("store_link_unsafe: %s is a symlink in the worktree; %s not linked", cur, l.Path))
			return false, nil
		case err == nil && !pfi.IsDir():
			return false, &Error{Code: ExitStoreLinkFailed, Kind: "store_link_conflict",
				Message: fmt.Sprintf("%s exists in the worktree and is not a directory", cur)}
		case errors.Is(err, fs.ErrNotExist):
			if err := root.Mkdir(cur, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return false, &Error{Code: ExitStoreLinkFailed, Kind: "store_link_failed", Message: cur, Cause: err}
			}
			_ = root.Chmod(cur, 0o700) // defeat the umask on a directory lets just created
		case err != nil:
			add(StepWarn, fmt.Sprintf("store_link_unsafe: %s: %v; %s not linked", cur, err, l.Path))
			return false, nil
		}
	}

	target := src
	if r, err := filepath.Rel(mainRoot, filepath.Join(wtRoot, rel)); err == nil && !strings.HasPrefix(r, "..") {
		if rt, err := filepath.Rel(filepath.Dir(filepath.Join(wtRoot, rel)), src); err == nil {
			target = rt
		}
	}
	if lfi, err := root.Lstat(rel); err == nil {
		if lfi.Mode()&os.ModeSymlink != 0 && sameFile(filepath.Join(wtRoot, rel), src) {
			add(StepOK, l.Path+" already linked")
			return true, nil
		}
		return false, &Error{Code: ExitStoreLinkFailed, Kind: "store_link_conflict",
			Message:     fmt.Sprintf("%s already exists in the worktree and is not a link to the main repo's file", l.Path),
			Remediation: fmt.Sprintf("move it aside, then re-run: mv %s %s.bak", shellQuote(filepath.Join(wtRoot, rel)), shellQuote(filepath.Join(wtRoot, rel)))}
	}
	if err := root.Symlink(target, rel); err != nil {
		if errors.Is(err, fs.ErrExist) && sameFile(filepath.Join(wtRoot, rel), src) {
			add(StepOK, l.Path+" already linked")
			return true, nil
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.EXDEV) {
			add(StepWarn, fmt.Sprintf("store_link_unsafe: %s escapes the worktree; not linked", l.Path))
			return false, nil
		}
		return false, &Error{Code: ExitStoreLinkFailed, Kind: "store_link_failed", Message: l.Path, Cause: err}
	}
	add(StepOK, fmt.Sprintf("%s symlinked (main file %04o)", l.Path, fi.Mode().Perm()&l.Mode))
	return true, nil
}

// sameFile reports whether a and b resolve to the same file.
func sameFile(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// shellQuote single-quotes a path for a printed remediation command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
