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
	"time"

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

// settingsStage is a test seam: a no-op the tests replace to act between the helper's
// steps ("after-exclude", "before-reread", "before-write").
var settingsStage = func(string) {}

// settingsLockName is the per-worktree lock of mergeSettingsLocal, in the worktree's
// own git dir: never inside the work tree (git status would show it) and never the
// common dir (each worktree has its own settings file).
const settingsLockName = "lets-settings-local.lock"

// errSettingsGitDir marks a lock that was not taken because the worktree's git dir
// could not be resolved.
var errSettingsGitDir = errors.New("git dir unresolved")

// lockSettingsLocal serializes mergeSettingsLocal's read -> merge -> write per
// worktree (lets-qysjv): two LETS writers interleaving would rename the older document
// over the newer one and drop the other's entries. The deadline is ctx's, else three
// seconds. The returned func releases the lock.
func lockSettingsLocal(ctx context.Context, wtRoot string) (func(), error) {
	out, err := exec.CommandContext(ctx, "git", "-C", wtRoot, "rev-parse", "--absolute-git-dir").Output()
	gitDir := strings.TrimSpace(string(out))
	if err == nil && gitDir == "" {
		err = errors.New("git printed no git dir")
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSettingsGitDir, err)
	}
	f, err := os.OpenFile(filepath.Join(gitDir, settingsLockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(3 * time.Second)
	}
	if err := fsutil.TryLockFile(f, deadline); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

// EnsureLetsAdditionalDir declares the main checkout's .lets/ as an additional
// working directory of the worktree at wtRoot (lets-urmfa). A worktree reaches .lets/
// through a symlink whose target lies outside the session's working directory, so
// every write there - the team file, snapshots, caches, reports - prompts, even in
// auto mode. The fix is the symlink-resolved <mainRoot>/.lets in
// permissions.additionalDirectories of wtRoot's .claude/settings.local.json, through
// mergeSettingsLocal: an entry already present - verbatim, relative to wtRoot, or
// through a symlink - is not written again. Never fails its caller: the outcome is
// one Step, plus the kind of a warning ("" for ok / skip).
func EnsureLetsAdditionalDir(ctx context.Context, wtRoot, mainRoot string) (Step, string) {
	want := filepath.Join(mainRoot, ".lets")
	if real, err := filepath.EvalSymlinks(want); err == nil {
		want = real
	}
	return mergeSettingsLocal(ctx, wtRoot, mainRoot, settingsMerge{
		key:    "additionalDirectories",
		want:   []string{want},
		same:   func(have, want string) bool { return sameDir(have, want, wtRoot) },
		label:  want,
		impact: fmt.Sprintf("writes to %s will prompt; /add-dir %s in the session is the workaround", want, want),
		retry:  "the next session start retries", // the SessionStart self-heal runs it again
	})
}

// TeamAllowRules are the permission rules a standing team's lead needs so auto mode
// never classifies its lifecycle commands (lets-qysjv, ported by lets-gbfpd). Narrow
// on purpose: switch and the task-state writer only, never remove / sweep; `lets
// members` is a deprecated stub and gets no rule.
func TeamAllowRules() []string {
	return []string{"Bash(lets worktree switch:*)", "Bash(lets worktree task-state:*)"}
}

// EnsureTeamAllowRules adds TeamAllowRules to a team worktree's personal
// .claude/settings.local.json permissions.allow, with EnsureLetsAdditionalDir's safety
// rules, and only while git ignores the file: rules in a file git would publish are
// repository-supplied and wait for workspace trust. A rule already there is not added
// again; the ok step names what was added and how to revoke it.
func EnsureTeamAllowRules(ctx context.Context, wtRoot, mainRoot string) (Step, string) {
	return mergeSettingsLocal(ctx, wtRoot, mainRoot, settingsMerge{
		key:            "allow",
		want:           TeamAllowRules(),
		label:          strings.Join(TeamAllowRules(), ", "),
		impact:         "lets worktree switch and lets worktree task-state will prompt, and auto mode may refuse them; approve them when asked, or add the rules with /permissions",
		requireIgnored: true,
		retry:          "re-run team-init or add the rules with /permissions", // team-init only, no backfill
		added: func(rules []string) string {
			return "added `" + strings.Join(rules, "`, `") + "` to " + settingsLocalRel + " (remove them to revoke)"
		},
	})
}

// settingsMerge is one permissions array a merge-write adds to (lets-qysjv).
type settingsMerge struct {
	key            string                        // "additionalDirectories" | "allow"
	want           []string                      // entries to add, in order
	same           func(have, want string) bool  // nil = exact string match
	label          string                        // how messages name what is added
	impact         string                        // what a warning costs the user, and the workaround
	requireIgnored bool                          // write nothing unless git ignores the file
	added          func(entries []string) string // the ok message for the appended entries; nil = the generic one
	retry          string                        // what retries a settings_local_changed write (who runs it again)
}

// mergeSettingsLocal adds m.want to permissions.<m.key> of wtRoot's
// .claude/settings.local.json - the one merge core every settings writer shares.
//
// Merge-write: every other key and entry is kept (numbers losslessly), each wanted
// entry is added once, and an entry already present (m.same) is not written again.
// Never through a symlink (the store_link_unsafe rule), never a file git tracks,
// never a file that is not the expected JSON shape, and never over a file that
// changed during the merge. The file is kept out of git (check-ignore, else the
// shared info/exclude of mainRoot), also when every entry is already there; with
// m.requireIgnored nothing is written while git still does not ignore it. The whole
// read -> merge -> write holds lockSettingsLocal; the re-read guard stays, since
// Claude Code does not take that lock. Never fails its caller: the outcome is one
// Step, plus the kind of a warning ("" for ok / skip).
func mergeSettingsLocal(ctx context.Context, wtRoot, mainRoot string, m settingsMerge) (Step, string) {
	path := filepath.Join(wtRoot, settingsLocalRel)
	field := "permissions." + m.key
	warn := func(kind, detail string) (Step, string) {
		return Step{Status: StepWarn, Message: fmt.Sprintf(
			"%s: %s left unchanged (%s) - %s", kind, settingsLocalRel, detail, m.impact)}, kind
	}
	same := m.same
	if same == nil {
		same = func(have, want string) bool { return have == want }
	}

	for _, p := range []string{filepath.Join(wtRoot, ".claude"), path} {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return warn(SettingsLocalSymlink, p+" is a symlink; LETS never writes through one")
		}
	}

	// An unresolvable git dir writes nothing, but is reported where the first write
	// would happen: the git probes before it fail on the same cause and name it.
	var gitDirErr error
	unlock, err := lockSettingsLocal(ctx, wtRoot)
	switch {
	case errors.Is(err, errSettingsGitDir):
		gitDirErr = err
	case errors.Is(err, fsutil.ErrLockBusy):
		return warn(SettingsLocalChanged, "another LETS process is updating it; "+m.retry)
	case err != nil:
		return warn(SettingsLocalChanged, "could not take its lock ("+err.Error()+"); "+m.retry)
	default:
		defer unlock()
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
	var entries []any
	if raw, ok := perms[m.key]; ok {
		if entries, ok = raw.([]any); !ok {
			return warn(SettingsLocalShape, field+" is not an array")
		}
	}
	have := make([]string, 0, len(entries))
	for _, d := range entries {
		s, ok := d.(string)
		if !ok {
			return warn(SettingsLocalShape, field+" holds a non-string entry")
		}
		have = append(have, s)
	}
	var missing []string
	for _, w := range m.want {
		found := false
		for _, h := range have {
			if same(h, w) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, w)
		}
	}
	present := len(missing) == 0

	if present {
		ignored, err := settingsIgnored(ctx, wtRoot)
		if err != nil {
			return warn(SettingsLocalGitFailed, "could not tell whether git ignores it: "+err.Error())
		}
		if ignored {
			return Step{Status: StepSkip, Message: settingsLocalRel + " already lists " + m.label + " in " + field}, ""
		}
	}
	tracked, err := settingsTracked(ctx, wtRoot)
	if err != nil {
		return warn(SettingsLocalGitFailed, "could not tell whether git tracks it: "+err.Error())
	}
	if tracked {
		if present {
			return Step{Status: StepSkip, Message: settingsLocalRel + " already lists " + m.label + "; git tracks the file"}, ""
		}
		return warn(SettingsLocalTracked, "git tracks it; add "+m.label+" to "+field+" yourself")
	}

	// Keep the personal file out of git: a machine-wide ignore or a .gitignore entry
	// already may; otherwise the shared info/exclude (lets-x5ucf). A .gitignore
	// negation beats info/exclude, so the result is checked, never assumed.
	ignoreNote, ignoreKind := "", ""
	ignored, err := settingsIgnored(ctx, wtRoot)
	if err != nil {
		return warn(SettingsLocalGitFailed, "could not tell whether git ignores it: "+err.Error())
	}
	if gitDirErr != nil {
		return warn(SettingsLocalGitFailed, "could not resolve its git dir for the lock: "+gitDirErr.Error())
	}
	if !ignored {
		if err := ensureWorktreeExcludes(ctx, mainRoot, []string{settingsLocalRel}); err != nil {
			if m.requireIgnored {
				return warn(SettingsLocalExcludeFailed, fmt.Sprintf("could not add it to info/exclude (%v), and LETS writes it only while git ignores it", err))
			}
			ignoreNote = fmt.Sprintf("; %s: could not add it to info/exclude (%v) - it shows as untracked", SettingsLocalExcludeFailed, err)
			ignoreKind = SettingsLocalExcludeFailed
		} else {
			settingsStage("after-exclude")
			ignoredNow, err := settingsIgnored(ctx, wtRoot)
			if err != nil {
				return warn(SettingsLocalGitFailed, "could not tell whether git ignores it after info/exclude: "+err.Error())
			}
			if !ignoredNow {
				if m.requireIgnored {
					return warn(SettingsLocalNotIgnored, "still not ignored after info/exclude (a .gitignore negation?), and LETS writes it only while git ignores it")
				}
				ignoreNote = fmt.Sprintf("; %s: still not ignored after info/exclude (a .gitignore negation?) - it shows as untracked", SettingsLocalNotIgnored)
				ignoreKind = SettingsLocalNotIgnored
			}
		}
	}
	status, kind := StepOK, ""
	if ignoreKind != "" {
		status, kind = StepWarn, ignoreKind
	}
	if present {
		msg := settingsLocalRel + " already lists " + m.label
		if ignoreKind == "" {
			msg += "; added it to info/exclude"
		}
		return Step{Status: status, Message: msg + ignoreNote}, kind
	}

	for _, w := range missing {
		entries = append(entries, w)
	}
	perms[m.key] = entries
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
	settingsStage("before-reread")
	now, err := os.ReadFile(path)
	switch {
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return warn(SettingsLocalChanged, "could not re-read it before writing ("+err.Error()+"); "+m.retry)
	case (err == nil) != (original != nil) || !bytes.Equal(now, original):
		return warn(SettingsLocalChanged, "it changed while LETS merged it; "+m.retry)
	}
	settingsStage("before-write")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return warn(SettingsLocalWriteFailed, err.Error())
	}
	if err := fsutil.AtomicWriteBytes(path, buf.Bytes(), 0o644); err != nil {
		return warn(SettingsLocalWriteFailed, err.Error())
	}
	// Name only what was appended: an entry the user already had was not added.
	if m.added != nil {
		return Step{Status: status, Message: m.added(missing) + ignoreNote}, kind
	}
	return Step{Status: status, Message: fmt.Sprintf("%s: added %s to %s%s", settingsLocalRel, strings.Join(missing, ", "), field, ignoreNote)}, kind
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

// settingsIgnored reports whether git ignores the settings file in wtRoot: exit 0 is
// ignored, exit 1 is not; any other failure is an error - never read as "not ignored".
func settingsIgnored(ctx context.Context, wtRoot string) (bool, error) {
	err := exec.CommandContext(ctx, "git", "-C", wtRoot, "check-ignore", "-q", settingsLocalRel).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
