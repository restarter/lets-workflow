// Package taskstate is the ONE owner of the per-branch task-state file
// `.lets/sessions/.task-<branch-slug>` (fields `task:` / `start:` / `session:` /
// `origin:` / `orc:`). Every Go writer goes through MergeWrite and every markdown
// writer through `lets worktree task-state`, so a writer only ever changes the keys
// it owns and keeps every other line - including lines a newer LETS added.
//
// Leaf package: standard library plus the fsutil, peername and taskid leaves.
package taskstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/peername"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
)

// Keys is the canonical order of the known keys; new keys are appended in it.
var Keys = []string{"task", "start", "session", "origin", "orc"}

var (
	// ErrInvalidValue: a value in WriteOpts.Set failed validation; nothing was written.
	ErrInvalidValue = errors.New("invalid task-state value")
	// ErrEmptySlug: detached HEAD (no branch) has no task-state file.
	ErrEmptySlug = errors.New("empty branch slug")
	// ErrLockBusy: the deadline passed while another writer held the lock.
	ErrLockBusy = fsutil.ErrLockBusy
)

var (
	shaRe     = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	sessionRe = regexp.MustCompile(`^[0-9a-f]{7,64} [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Slug maps a branch to the file key. An empty (detached) branch has no file.
func Slug(branch string) (string, bool) {
	if branch == "" {
		return "", false
	}
	return strings.ReplaceAll(branch, "/", "-"), true
}

// State is the parsed file. Other holds unknown lines verbatim.
type State struct {
	Task, Start, Session, Origin, Orc string
	Other                             []string
}

func (s *State) set(key, value string) {
	switch key {
	case "task":
		s.Task = value
	case "start":
		s.Start = value
	case "session":
		s.Session = value
	case "origin":
		s.Origin = value
	case "orc":
		s.Orc = value
	}
}

// Path returns the task-state file path for slug under letsDir (<root>/.lets).
func Path(letsDir, slug string) string {
	return filepath.Join(letsDir, "sessions", ".task-"+slug)
}

func lockPath(letsDir, slug string) string {
	return filepath.Join(letsDir, "locks", "task-"+slug+".lock")
}

// splitLine returns the known key and value of a `key: value` line.
func splitLine(line string) (key, value string, known bool) {
	for _, k := range Keys {
		if v, ok := strings.CutPrefix(line, k+": "); ok {
			return k, strings.TrimSpace(v), true
		}
	}
	return "", "", false
}

func parse(content string) State {
	var s State
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if k, v, ok := splitLine(line); ok {
			if !seen[k] {
				s.set(k, v)
				seen[k] = true
			}
			continue
		}
		s.Other = append(s.Other, line)
	}
	return s
}

// Read parses the file for slug. A missing file is os.ErrNotExist.
func Read(letsDir, slug string) (State, error) {
	if slug == "" {
		return State{}, ErrEmptySlug
	}
	data, err := os.ReadFile(Path(letsDir, slug))
	if err != nil {
		return State{}, err
	}
	return parse(string(data)), nil
}

// WriteOpts configures MergeWrite.
type WriteOpts struct {
	Set      map[string]string // only these keys change; an empty value deletes the key
	Create   bool              // false: a missing file is left missing (refresh-if-exists)
	Deadline time.Time         // zero: block; else TryLockFile until the deadline
	// Derive runs under the lock with the state MergeWrite just read (exists=false for
	// a missing file) and returns more keys to change, merged over Set. A writer whose
	// update depends on the current task - a conflict check, a start: kept for the same
	// task - decides here, on what it will overwrite, never on an earlier Read. A
	// non-nil error writes nothing and is returned as is.
	Derive func(cur State, exists bool) (map[string]string, error)
}

// ErrFileAbsent: the file does not exist and WriteOpts.Create was false.
var ErrFileAbsent = errors.New("task-state file absent")

// Validate checks one key's value (an empty value is a delete and always valid).
func Validate(key, value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\n\r") || strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("%w: %s holds a control character", ErrInvalidValue, key)
	}
	ok := false
	switch key {
	case "task":
		ok = taskid.Valid(value)
	case "start":
		ok = shaRe.MatchString(value)
	case "session":
		ok = sessionRe.MatchString(value)
	case "origin":
		ok = value == "branch" || value == "dir"
	case "orc":
		ok = peername.Valid(value)
	default:
		return fmt.Errorf("%w: unknown key %q", ErrInvalidValue, key)
	}
	if !ok {
		return fmt.Errorf("%w: %s %q", ErrInvalidValue, key, value)
	}
	return nil
}

// lock opens and locks the slug's lock file. The returned func releases it.
func lock(letsDir, slug string, deadline time.Time) (func(), error) {
	if err := os.MkdirAll(filepath.Join(letsDir, "locks"), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath(letsDir, slug), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if deadline.IsZero() {
		err = fsutil.LockFile(f)
	} else {
		err = fsutil.TryLockFile(f, deadline)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

// acquire is the lock MergeWrite takes (a seam: a test writes the file while a writer
// waits for it).
var acquire = lock

// MergeWrite takes .lets/locks/task-<slug>.lock, re-reads, runs Derive, validates
// every value to set, applies it, keeps every other line (in its original order), and
// writes atomically (tmp + rename in the same directory, 0600). It returns the new state.
func MergeWrite(letsDir, slug string, o WriteOpts) (State, error) {
	if slug == "" {
		return State{}, ErrEmptySlug
	}
	for k, v := range o.Set {
		if err := Validate(k, v); err != nil {
			return State{}, err
		}
	}
	path := Path(letsDir, slug)
	// Refresh-if-exists must not leave anything behind: without this check the lock
	// below creates .lets/locks in every repo a SessionStart refresh runs in, LETS or not.
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) && !o.Create {
		return State{}, ErrFileAbsent
	}
	unlock, err := acquire(letsDir, slug, o.Deadline)
	if err != nil {
		return State{}, err
	}
	defer unlock()

	data, err := os.ReadFile(path)
	exists := err == nil
	switch {
	case errors.Is(err, os.ErrNotExist) && !o.Create:
		return State{}, ErrFileAbsent
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return State{}, err
	}

	set := o.Set
	if o.Derive != nil {
		more, err := o.Derive(parse(string(data)), exists)
		if err != nil {
			return State{}, err
		}
		set = map[string]string{}
		for k, v := range o.Set {
			set[k] = v
		}
		for k, v := range more {
			if err := Validate(k, v); err != nil {
				return State{}, err
			}
			set[k] = v
		}
	}

	var out []string
	applied := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		k, _, known := splitLine(line)
		v, inSet := set[k]
		switch {
		case !known || !inSet:
			out = append(out, line)
		case applied[k] || v == "":
			// a duplicate of a key being set, or a delete: drop it
		default:
			out = append(out, k+": "+v)
		}
		if known && inSet {
			applied[k] = true
		}
	}
	for _, k := range Keys {
		if v, ok := set[k]; ok && v != "" && !applied[k] {
			out = append(out, k+": "+v)
		}
	}
	content := strings.Join(out, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return State{}, err
	}
	if err := atomicWrite(path, content); err != nil {
		return State{}, err
	}
	return parse(content), nil
}

// Remove deletes the file and any stranded atomic-write temp siblings under the lock.
func Remove(letsDir, slug string, deadline time.Time) error {
	if slug == "" {
		return ErrEmptySlug
	}
	unlock, err := lock(letsDir, slug, deadline)
	if err != nil {
		return err
	}
	defer unlock()
	return removeLocked(letsDir, slug)
}

// ErrChanged: RemoveIfTask found a different task than the caller recorded.
var ErrChanged = errors.New("task-state file changed since it was read")

// RemoveIfTask deletes the file only if, under its lock, it still names task (""
// = still names no task). A file already gone is not an error. The comparison is on
// task: alone on purpose - a session: refresh for the same task (the SessionStart
// hook) does not change what the caller's marker covers.
func RemoveIfTask(letsDir, slug, task string, deadline time.Time) error {
	if slug == "" {
		return ErrEmptySlug
	}
	unlock, err := lock(letsDir, slug, deadline)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := os.ReadFile(Path(letsDir, slug))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if parse(string(data)).Task != task {
		return ErrChanged
	}
	return removeLocked(letsDir, slug)
}

// removeLocked deletes the file and this slug's stranded temps; the caller holds the lock.
func removeLocked(letsDir, slug string) error {
	path := Path(letsDir, slug)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	prefix := tempPath(path) + "."
	if temps, _ := filepath.Glob(prefix + "*"); temps != nil {
		for _, m := range temps {
			// Only atomicWrite's `.tasktmp-<slug>.<digits>`: the prefix of a slug that
			// extends this one (`lets-abc` vs `lets-abc.1234`) leaves a non-digit remainder.
			// A legacy `.task-<slug>.<digits>.tmp` of an older binary is never deleted -
			// it is also the state file of a legal branch (`feature.12345.tmp`).
			if tempSuffixRe.MatchString(strings.TrimPrefix(m, prefix)) {
				_ = os.Remove(m)
			}
		}
	}
	return nil
}

var tempSuffixRe = regexp.MustCompile(`^[0-9]+$`)

// tempPath is the temp-file stem of a task-state path: `.tasktmp-<slug>` beside it.
// Temps live outside the `.task-` namespace, so no state file can be mistaken for one.
func tempPath(path string) string {
	return filepath.Join(filepath.Dir(path), ".tasktmp-"+strings.TrimPrefix(filepath.Base(path), ".task-"))
}

// Slugs lists the slugs of the task-state files under letsDir: every `.task-*` entry.
// Temps live under `.tasktmp-*` and locks under .lets/locks/, so nothing is filtered
// by guessing. A missing sessions directory is an empty list; an unreadable one is an error.
func Slugs(letsDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(letsDir, "sessions"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		slug, ok := strings.CutPrefix(e.Name(), ".task-")
		if !ok || slug == "" || e.IsDir() {
			continue
		}
		out = append(out, slug)
	}
	return out, nil
}

// atomicWrite writes content via a same-dir temp file + rename, so it survives the
// .lets symlink in worktrees (a cross-device rename would fail).
func atomicWrite(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(tempPath(path))+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}
