//go:build unix

package worktreecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

// settingsLocalRel is a checkout's personal Claude Code settings file.
const settingsLocalRel = ".claude/settings.local.json"

// The kinds EnsureLetsAdditionalDir names in a warning. The self-heal keys its
// Notice policy on them (SettingsLocalTracked and SettingsLocalNotIgnored are not
// Notices - the project's own choice).
const (
	SettingsLocalSymlink       = "settings_local_symlink"
	SettingsLocalUnreadable    = "settings_local_unreadable"
	SettingsLocalShape         = "settings_local_shape"
	SettingsLocalGitFailed     = "settings_local_git_failed"
	SettingsLocalTracked       = "settings_local_tracked"
	SettingsLocalChanged       = "settings_local_changed"
	SettingsLocalWriteFailed   = "settings_local_write_failed"
	SettingsLocalNotIgnored    = "settings_local_not_ignored"
	SettingsLocalExcludeFailed = "settings_local_exclude_failed"
)

// EnsureLetsAdditionalDir declares the main checkout's .lets/ as an additional
// working directory of the worktree at wtRoot (lets-urmfa). A worktree reaches .lets/
// through a symlink whose target lies outside the session's working directory, so
// every write there - the team file, snapshots, caches, reports - prompts, even in
// auto mode. The fix is the symlink-resolved <mainRoot>/.lets in
// permissions.additionalDirectories of wtRoot's .claude/settings.local.json.
//
// Merge-write: every other key and entry is kept (numbers losslessly), the path is
// added once, and an entry already present - verbatim, relative to wtRoot, or through
// a symlink - is not written again. Never through a symlink (the store_link_unsafe
// rule), never a file git tracks, never a file that is not the expected JSON shape,
// and never over a file that changed during the merge. The file is kept out of git
// (check-ignore, else the shared info/exclude), also when the entry is already there.
// Never fails its caller: the outcome is one Step, plus the kind of a warning ("" for
// ok / skip).
func EnsureLetsAdditionalDir(ctx context.Context, wtRoot, mainRoot string) (Step, string) {
	want := filepath.Join(mainRoot, ".lets")
	if real, err := filepath.EvalSymlinks(want); err == nil {
		want = real
	}
	path := filepath.Join(wtRoot, settingsLocalRel)
	warn := func(kind, detail string) (Step, string) {
		return Step{Status: StepWarn, Message: fmt.Sprintf(
			"%s: %s left unchanged (%s) - writes to %s will prompt; /add-dir %s in the session is the workaround",
			kind, settingsLocalRel, detail, want, want)}, kind
	}

	for _, p := range []string{filepath.Join(wtRoot, ".claude"), path} {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return warn(SettingsLocalSymlink, p+" is a symlink; LETS never writes through one")
		}
	}

	original, settings, err := readSettingsObject(path)
	if err != nil {
		return warn(SettingsLocalUnreadable, err.Error())
	}
	perms := map[string]any{}
	if raw, ok := settings["permissions"]; ok {
		if perms, ok = raw.(map[string]any); !ok {
			return warn(SettingsLocalShape, "permissions is not an object")
		}
	}
	var dirs []any
	if raw, ok := perms["additionalDirectories"]; ok {
		if dirs, ok = raw.([]any); !ok {
			return warn(SettingsLocalShape, "permissions.additionalDirectories is not an array")
		}
	}
	present := ""
	for _, d := range dirs {
		s, ok := d.(string)
		if !ok {
			return warn(SettingsLocalShape, "permissions.additionalDirectories holds a non-string entry")
		}
		if present == "" && sameDir(s, want, wtRoot) {
			present = s
		}
	}

	if present != "" && settingsIgnored(ctx, wtRoot) {
		return Step{Status: StepSkip, Message: settingsLocalRel + " already lists " + want + " in permissions.additionalDirectories"}, ""
	}
	tracked, err := settingsTracked(ctx, wtRoot)
	if err != nil {
		return warn(SettingsLocalGitFailed, "could not tell whether git tracks it: "+err.Error())
	}
	if tracked {
		if present != "" {
			return Step{Status: StepSkip, Message: settingsLocalRel + " already lists " + want + "; git tracks the file"}, ""
		}
		return warn(SettingsLocalTracked, "git tracks it; add "+want+" to permissions.additionalDirectories yourself")
	}

	// Keep the personal file out of git: a machine-wide ignore or a .gitignore entry
	// already may; otherwise the shared info/exclude (lets-x5ucf). A .gitignore
	// negation beats info/exclude, so the result is checked, never assumed.
	ignoreNote, ignoreKind := "", ""
	if !settingsIgnored(ctx, wtRoot) {
		if err := ensureWorktreeExcludes(ctx, mainRoot, []string{settingsLocalRel}); err != nil {
			ignoreNote = fmt.Sprintf("; %s: could not add it to info/exclude (%v) - it shows as untracked", SettingsLocalExcludeFailed, err)
			ignoreKind = SettingsLocalExcludeFailed
		} else if !settingsIgnored(ctx, wtRoot) {
			ignoreNote = fmt.Sprintf("; %s: still not ignored after info/exclude (a .gitignore negation?) - it shows as untracked", SettingsLocalNotIgnored)
			ignoreKind = SettingsLocalNotIgnored
		}
	}
	status, kind := StepOK, ""
	if ignoreKind != "" {
		status, kind = StepWarn, ignoreKind
	}
	if present != "" {
		msg := settingsLocalRel + " already lists " + want
		if ignoreKind == "" {
			msg += "; added it to info/exclude"
		}
		return Step{Status: status, Message: msg + ignoreNote}, kind
	}

	perms["additionalDirectories"] = append(dirs, want)
	settings["permissions"] = perms
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep `&&` and `<` in the user's permission rules as written
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return warn(SettingsLocalWriteFailed, err.Error())
	}
	// A lost update is not a torn file: Claude Code may have written the file since the
	// read above. Re-read right before the rename; a change means no write this time.
	if now, err := os.ReadFile(path); (err == nil) != (original != nil) || !bytes.Equal(now, original) {
		return warn(SettingsLocalChanged, "it changed while LETS merged it; the next session start retries")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return warn(SettingsLocalWriteFailed, err.Error())
	}
	if err := fsutil.AtomicWriteBytes(path, buf.Bytes(), 0o644); err != nil {
		return warn(SettingsLocalWriteFailed, err.Error())
	}
	return Step{Status: status, Message: fmt.Sprintf("%s: added %s to permissions.additionalDirectories%s", settingsLocalRel, want, ignoreNote)}, kind
}

// sameDir reports whether the additionalDirectories entry s names want: verbatim, or
// after resolving it (a relative entry against wtRoot, then symlinks).
func sameDir(s, want, wtRoot string) bool {
	if s == want {
		return true
	}
	if !filepath.IsAbs(s) {
		s = filepath.Join(wtRoot, s)
	}
	real, err := filepath.EvalSymlinks(s)
	return err == nil && real == want
}

// readSettingsObject reads a settings file as a JSON object, numbers kept as
// json.Number. A missing file is (nil, empty object); a blank file is an empty object;
// anything that is not exactly one object (malformed, null, an array, trailing data)
// is an error, so the caller never overwrites it. The sibling statuslinecmd.readSettings
// accepts null - this path writes unattended, so it refuses it.
func readSettingsObject(path string) ([]byte, map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, map[string]any{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return data, map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, nil, err
	}
	if m == nil {
		return nil, nil, errors.New("not a JSON object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("trailing data after the JSON object")
	}
	return data, m, nil
}

// settingsTracked reports whether git tracks the settings file in wtRoot. Any git
// failure is an error - never read as "untracked".
func settingsTracked(ctx context.Context, wtRoot string) (bool, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", wtRoot, "ls-files", "--cached", "--", settingsLocalRel).Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// settingsIgnored reports whether git ignores the settings file in wtRoot; any
// failure counts as not ignored (the caller then tries info/exclude and checks again).
func settingsIgnored(ctx context.Context, wtRoot string) bool {
	return exec.CommandContext(ctx, "git", "-C", wtRoot, "check-ignore", "-q", settingsLocalRel).Run() == nil
}
