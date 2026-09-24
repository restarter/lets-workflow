// Package rulescache keeps the machine-global ~/.claude/rules/lets-rules.md as
// a derived cache of the RUNNING plugin's rules (lets-tg008). The SessionStart
// hook calls Sync on every start with the CLAUDE_PLUGIN_ROOT of the version the
// session actually loaded; `lets init --user` calls it to bootstrap. It is the
// only writer of that file.
//
// Contract:
//   - key = sha256 of what was written (never the frontmatter version: one
//     version string has been seen with two contents); every Sync hashes both
//     files - there is no stat shortcut, because size+mtime cannot prove equality;
//   - the whole read-compare-backup-write runs under one per-home lock, and
//     re-reads everything inside it;
//   - forward only: an older plugin never replaces a newer cache;
//   - only an installed plugin writes; uncertain means do not write;
//   - a copy that is not provably LETS content is COPIED to .bak[-N] first; the
//     active file is only ever replaced atomically, never moved away;
//   - every failure is a Result, never a panic and never a blocked start.
//
// Scope: user scope only. Project .claude/rules copies and the tracker adapter
// keep the drift machinery (package drift).
package rulescache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

// Outcome is what Sync did.
type Outcome string

const (
	OutcomeNoop      Outcome = "noop"       // nothing to do (or only the key refreshed)
	OutcomeCreated   Outcome = "created"    // the file did not exist and was written
	OutcomeWritten   Outcome = "written"    // an existing copy was replaced
	OutcomeKeptNewer Outcome = "kept-newer" // refused a downgrade - not an error
	OutcomeSkipped   Outcome = "skipped"    // a change was withheld (untrusted root, lock busy, no home)
	OutcomeFailed    Outcome = "failed"     // I/O error; the active file is as it was
)

// Options are the inputs for Sync.
type Options struct {
	PluginRoot string // the running session's plugin root (dir holding rules/lets-rules.md)
	HomeDir    string
	InProject  bool // the session runs inside an initialized LETS project (.lets/.env exists)
	ScopeUser  bool // that project's merged LETS_RULES_SCOPE is "user"
}

// Result reports one Sync.
type Result struct {
	Outcome Outcome
	Reason  string // why, for kept-newer / skipped / failed
	From    string // version of the copy before (may be "")
	To      string // version now installed
	Backup  string // basename of the backup a non-LETS copy was saved to
}

// Key is the persisted cache key.
type Key struct {
	Hash       string `json:"sha256"`
	Version    string `json:"version"`
	SourcePath string `json:"source_path"`
}

var hexHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// writeFile replaces the active file; tests swap it to simulate a failed replace.
var writeFile = fsutil.AtomicWriteBytes

// DstPath is the cached file.
func DstPath(home string) string { return filepath.Join(home, ".claude", "rules", "lets-rules.md") }

// KeyPath is the cache key file.
func KeyPath(home string) string { return filepath.Join(home, ".lets", "cache", "rules-cache.json") }

// LockPath serializes Sync across concurrent session starts.
func LockPath(home string) string { return filepath.Join(home, ".lets", "cache", "rules-cache.lock") }

// Sum is the hex sha256 used as the cache key.
func Sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ReadKey returns the persisted key; an absent, unparseable or malformed key
// (anything but a 64-char hex hash) is (Key{}, false).
func ReadKey(home string) (Key, bool) {
	data, err := os.ReadFile(KeyPath(home))
	if err != nil {
		return Key{}, false
	}
	var k Key
	if json.Unmarshal(data, &k) != nil || !hexHash.MatchString(k.Hash) {
		return Key{}, false
	}
	return k, true
}

// Sync brings the global rules to the running plugin's content, within the
// contract in the package doc.
func Sync(o Options) Result {
	if o.HomeDir == "" || !filepath.IsAbs(o.HomeDir) || filepath.Clean(o.HomeDir) == "/" {
		return Result{Outcome: OutcomeSkipped, Reason: "home directory unresolved"}
	}
	// Cheap exit before any lock or directory: user scope not in use here.
	if _, err := os.Stat(DstPath(o.HomeDir)); errors.Is(err, os.ErrNotExist) && (!o.InProject || !o.ScopeUser) {
		return Result{Outcome: OutcomeNoop}
	}
	if err := os.MkdirAll(filepath.Dir(LockPath(o.HomeDir)), 0o755); err != nil {
		return failed(err)
	}
	lf, err := os.OpenFile(LockPath(o.HomeDir), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return failed(err)
	}
	defer func() { _ = lf.Close() }()
	if err := fsutil.TryLockFile(lf, time.Now().Add(2*time.Second)); err != nil {
		return Result{Outcome: OutcomeSkipped, Reason: fmt.Sprintf("another session is syncing them right now (%v)", err)}
	}
	defer func() { _ = fsutil.UnlockFile(lf) }()
	return syncLocked(o)
}

func syncLocked(o Options) Result {
	src := filepath.Join(o.PluginRoot, "rules", "lets-rules.md")
	dst := DstPath(o.HomeDir)
	key, haveKey := ReadKey(o.HomeDir)

	dstData, err := os.ReadFile(dst)
	exists := err == nil
	if !exists && !errors.Is(err, os.ErrNotExist) {
		return failed(err)
	}
	if !exists && (!o.InProject || !o.ScopeUser) {
		return Result{Outcome: OutcomeNoop} // re-checked under the lock: never create here
	}
	srcData, err := os.ReadFile(src)
	if err != nil {
		return failed(fmt.Errorf("plugin rules unreadable: %w", err))
	}
	srcHash := Sum(srcData)
	srcVer := frontmatter.ReadVersion(src)

	if exists && Sum(dstData) == srcHash {
		// Already equal. Only an installed plugin may record the key: an equal
		// --plugin-dir checkout stays silent and leaves the metadata untouched.
		if (!haveKey || key.Hash != srcHash || key.Version != srcVer || key.SourcePath != src) && untrusted(o.PluginRoot, o.HomeDir, srcVer) == "" {
			writeKey(o.HomeDir, Key{Hash: srcHash, Version: srcVer, SourcePath: src})
		}
		return Result{Outcome: OutcomeNoop}
	}

	if reason := untrusted(o.PluginRoot, o.HomeDir, srcVer); reason != "" {
		return Result{Outcome: OutcomeSkipped, Reason: reason}
	}

	res := Result{Outcome: OutcomeCreated, To: srcVer}
	pristine := false
	if exists {
		dstHash := Sum(dstData)
		res.Outcome = OutcomeWritten
		res.From = frontmatter.ReadVersion(dst)
		if haveKey && dstHash == key.Hash {
			res.From, pristine = key.Version, true
		} else {
			pristine = isPristine(o.HomeDir, dstHash, res.From)
		}
		if validVer(res.From) && semver.Compare("v"+srcVer, "v"+res.From) < 0 {
			return Result{Outcome: OutcomeKeptNewer, From: res.From, To: res.From,
				Reason: fmt.Sprintf("this session runs the older plugin v%s", srcVer)}
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return failed(err)
	}
	if exists && !pristine {
		bak, err := backup(dst, dstData)
		if err != nil {
			return failed(err)
		}
		res.Backup = filepath.Base(bak)
	}
	if err := writeFile(dst, srcData, 0o644); err != nil {
		r := failed(err)
		r.Backup = res.Backup
		return r
	}
	writeKey(o.HomeDir, Key{Hash: srcHash, Version: srcVer, SourcePath: src})
	return res
}

// Notice is the one-line `## LETS Notice` text, "" when nothing worth saying happened.
func (r Result) Notice() string {
	const f = "`~/.claude/rules/lets-rules.md`"
	switch r.Outcome {
	case OutcomeCreated:
		return fmt.Sprintf("Global workflow rules installed at %s (v%s, from the running plugin).", f, r.To)
	case OutcomeWritten:
		msg := fmt.Sprintf("Global workflow rules refreshed to the running plugin v%s (%s).", r.To, f)
		if r.From != "" && r.From != r.To {
			msg = fmt.Sprintf("Global workflow rules refreshed v%s -> v%s (%s).", r.From, r.To, f)
		}
		if r.Backup != "" {
			msg += fmt.Sprintf(" The previous copy matched no LETS release and was saved to `~/.claude/rules/%s` - keep your own rules in a separate .md file.", r.Backup)
		}
		return msg
	case OutcomeKeptNewer:
		return fmt.Sprintf("Global workflow rules kept at v%s - %s. Not an error; the newer copy stays.", r.To, r.Reason)
	case OutcomeSkipped:
		return fmt.Sprintf("Global workflow rules not synced: %s.", r.Reason)
	case OutcomeFailed:
		msg := fmt.Sprintf("Global workflow rules sync FAILED (%s) - %s left unchanged.", r.Reason, f)
		if r.Backup != "" {
			msg += fmt.Sprintf(" A copy was saved to `~/.claude/rules/%s`.", r.Backup)
		}
		return msg
	}
	return ""
}

func failed(err error) Result { return Result{Outcome: OutcomeFailed, Reason: err.Error()} }

func validVer(v string) bool { return v != "" && semver.IsValid("v"+v) }

// untrusted returns "" only when root is provably an installed LETS plugin:
// <home>/.claude/plugins/cache/<marketplace>/lets/<v>[-suffix] whose plugin.json
// says name "lets", version <v>, and <v> equals the rules frontmatter version.
func untrusted(root, home, srcVer string) string {
	if !validVer(srcVer) {
		return "the plugin rules carry no valid version"
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Sprintf("plugin root %s unresolved", root)
	}
	cache, err := filepath.EvalSymlinks(filepath.Join(home, ".claude", "plugins", "cache"))
	if err != nil {
		return "no installed-plugin cache at ~/.claude/plugins/cache"
	}
	rel, err := filepath.Rel(cache, real)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if err != nil || len(parts) != 3 || parts[0] == ".." || parts[1] != "lets" {
		return fmt.Sprintf("plugin root %s is not an installed plugin (expected ~/.claude/plugins/cache/<marketplace>/lets/<version>)", root)
	}
	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	data, err := os.ReadFile(filepath.Join(real, ".claude-plugin", "plugin.json"))
	if err != nil || json.Unmarshal(data, &m) != nil || m.Name != "lets" || m.Version != srcVer ||
		(parts[2] != m.Version && !strings.HasPrefix(parts[2], m.Version+"-")) {
		return fmt.Sprintf("plugin root %s: manifest does not match its install dir and rules", root)
	}
	return ""
}

// isPristine reports whether hash is byte-identical to the rules of an
// installed LETS release of version ver (any marketplace).
func isPristine(home, hash, ver string) bool {
	if !validVer(ver) {
		return false
	}
	pattern := filepath.Join(home, ".claude", "plugins", "cache", "*", "lets", ver+"*", "rules", "lets-rules.md")
	matches, _ := filepath.Glob(pattern)
	for _, p := range matches {
		if data, err := os.ReadFile(p); err == nil && Sum(data) == hash {
			return true
		}
	}
	return false
}

// backup COPIES data to the first free dst.bak, dst.bak-2, ... (O_EXCL, so a
// name is never reused); the active file stays in place until the atomic replace.
func backup(dst string, data []byte) (string, error) {
	for n := 1; n <= 100; n++ {
		cand := dst + ".bak"
		if n > 1 {
			cand = fmt.Sprintf("%s.bak-%d", dst, n)
		}
		f, err := os.OpenFile(cand, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr != nil || cerr != nil {
			_ = os.Remove(cand)
			return "", errors.Join(werr, cerr)
		}
		return cand, nil
	}
	return "", fmt.Errorf("no free backup name next to %s", dst)
}

// writeKey records the key best-effort: a lost write only costs a re-hash.
func writeKey(home string, k Key) {
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return
	}
	_ = fsutil.AtomicWriteBytes(KeyPath(home), append(data, '\n'), 0o644)
}
