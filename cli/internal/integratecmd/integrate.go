//go:build unix

// Package integratecmd is `lets integrate`: it lands one chunk of an isolated
// implementer's branch in the caller's tree as a staged patch, never as a merge
// and never through `reset --hard`. The commits since..from are cherry-picked in
// a temporary detached worktree at the caller's HEAD; the resulting diff is
// written to `.lets/cache/integrate-<run>-<chunk>.patch` and applied with
// `git apply --index`, then verified against the picked tree. A conflict leaves
// the caller's tree untouched. The lead commits; this package never does.
//
// The temporary worktree lives at `<git-common-dir>/lets-integrate/<run>-<chunk>`
// and is proved ours before any `worktree remove --force`: an owner marker
// outside it (`<run>-<chunk>.owner`) and a nonce in its own gitdir must match.
// Every git call in or on it runs with hooks off (tempGit).
//
// Leaf package: standard library plus the fsutil leaf.
package integratecmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

// Options is one `lets integrate` call. From is any commit-ish (a branch or a
// sha); Since must be its ancestor. Run and Chunk name the temp worktree and the
// patch file, so they are validated before any path is built. Revert with Patch
// is `--revert --patch`: undo a patch an earlier call applied (Revert).
type Options struct {
	From, Since, Run, Chunk string
	Revert                  bool
	Patch                   string
	LockDeadline            time.Duration // zero = defaultLockDeadline
}

const (
	defaultLockDeadline = 5 * time.Second
	nonceFile           = "lets-integrate-nonce"
	minGitMajor         = 2
	minGitMinor         = 45 // cherry-pick --empty=drop
)

// nameRe is the grammar of --run and --chunk: no `/`, no `.`, so `..` can never
// point the temp path at a registered worktree.
var nameRe = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// Run integrates since..from into the caller's tree at root (its git toplevel).
func Run(ctx context.Context, root string, o Options) (*Result, error) {
	if o.Revert {
		return Revert(ctx, root, o)
	}
	r := newResult("integrate")
	if o.From == "" || o.Since == "" {
		return r, fail(r, &Error{Code: ExitUsage, Kind: "usage", Message: "--from and --since are required"})
	}
	for _, f := range []struct{ flag, v string }{{"--run", o.Run}, {"--chunk", o.Chunk}} {
		if !nameRe.MatchString(f.v) {
			return r, fail(r, &Error{Code: ExitUsage, Kind: "invalid_name", Message: fmt.Sprintf("%s %q must match %s", f.flag, f.v, nameRe)})
		}
	}
	if e := checkGitVersion(ctx, root); e != nil {
		return r, fail(r, e)
	}
	unlock, e := lock(root, o.LockDeadline)
	if e != nil {
		return r, fail(r, e)
	}
	defer unlock()

	base, from, since, e := preconditions(ctx, root, o)
	if e != nil {
		return r, fail(r, e)
	}
	r.step(StepOK, "preconditions: tree clean, HEAD on a branch, "+o.Since+" is an ancestor of "+o.From+", no merge in range")
	commits, err := gitOut(ctx, root, "rev-list", "--reverse", since+".."+from)
	if err != nil {
		return r, fail(r, gitErr("rev-list", err))
	}
	if commits == "" {
		r.OK = true
		r.step(StepSkip, "since..from is empty - nothing to integrate")
		return r, nil
	}

	t, e := newTemp(ctx, root, o.Run, o.Chunk)
	if e != nil {
		return r, fail(r, e)
	}
	if e := t.clearStale(ctx, root, r); e != nil {
		return r, fail(r, e)
	}
	if e := t.create(ctx, root, base); e != nil {
		return r, fail(r, e)
	}
	defer func() {
		if e := t.removeOwned(ctx, root); e != nil {
			r.step(StepWarn, "temp worktree not removed: "+e.Error()+" - remove "+t.path+" by hand")
			return
		}
		r.step(StepOK, "temp worktree removed")
	}()

	if e := t.pick(ctx, since, from, r); e != nil {
		return r, fail(r, e)
	}
	picked, err := tempGit(ctx, t.path, "rev-list", "--reverse", base+"..HEAD")
	if err != nil {
		return r, fail(r, gitErr("rev-list", err))
	}
	if picked == "" {
		r.OK = true
		r.step(StepSkip, "every commit became empty on the caller's HEAD - nothing to apply")
		return r, nil
	}
	r.Picked = strings.Split(picked, "\n")
	head := r.Picked[len(r.Picked)-1]
	r.step(StepOK, fmt.Sprintf("picked %d commit(s)", len(r.Picked)))

	if e := apply(ctx, root, t, o, base, head, r); e != nil {
		return r, fail(r, e)
	}
	r.OK = true
	return r, nil
}

// preconditions checks the caller tree and the range; it returns the caller's
// HEAD and the resolved from / since shas.
func preconditions(ctx context.Context, root string, o Options) (base, from, since string, e *Error) {
	dirty, err := gitOut(ctx, root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", "", "", gitErr("status", err)
	}
	if dirty != "" {
		return "", "", "", &Error{Code: ExitDirtyTree, Kind: "dirty_tree", Message: "the caller tree has changes:\n" + dirty, Remediation: "commit or remove them first"}
	}
	if _, err := gitOut(ctx, root, "symbolic-ref", "-q", "HEAD"); err != nil {
		return "", "", "", &Error{Code: ExitDetachedHead, Kind: "detached_head", Message: "HEAD is not on a branch"}
	}
	if base, err = gitOut(ctx, root, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return "", "", "", gitErr("rev-parse HEAD", err)
	}
	if from, err = gitOut(ctx, root, "rev-parse", "--verify", "--quiet", "--end-of-options", o.From+"^{commit}"); err != nil {
		return "", "", "", &Error{Code: ExitFromMissing, Kind: "from_missing", Message: "--from " + o.From + " names no commit"}
	}
	if since, err = gitOut(ctx, root, "rev-parse", "--verify", "--quiet", "--end-of-options", o.Since+"^{commit}"); err != nil {
		return "", "", "", &Error{Code: ExitSinceNotAncestor, Kind: "since_not_ancestor", Message: "--since " + o.Since + " names no commit"}
	}
	if _, err := gitOut(ctx, root, "merge-base", "--is-ancestor", since, from); err != nil {
		if exitStatus(err) == 1 {
			return "", "", "", &Error{Code: ExitSinceNotAncestor, Kind: "since_not_ancestor", Message: "--since " + o.Since + " is not an ancestor of --from " + o.From}
		}
		return "", "", "", gitErr("merge-base", err)
	}
	merges, err := gitOut(ctx, root, "rev-list", "--merges", since+".."+from)
	if err != nil {
		return "", "", "", gitErr("rev-list --merges", err)
	}
	if merges != "" {
		return "", "", "", &Error{Code: ExitMergeInRange, Kind: "merge_in_range", Message: "since..from holds merge commit(s): " + strings.ReplaceAll(merges, "\n", ", ")}
	}
	return base, from, since, nil
}

// apply writes the picked diff as a patch, applies it to the caller's index and
// worktree, and verifies both equal the picked tree; a mismatch is reverse-applied.
func apply(ctx context.Context, root string, t *temp, o Options, base, head string, r *Result) *Error {
	files, err := tempGit(ctx, t.path, "diff", "--name-only", "--no-renames", base, head)
	if err != nil {
		return gitErr("diff --name-only", err)
	}
	if files != "" {
		r.Files = strings.Split(files, "\n")
	}
	patch, err := tempGitRaw(ctx, t.path, "-c", "diff.noprefix=false", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--binary",
		"--src-prefix=a/", "--dst-prefix=b/", base, head)
	if err != nil {
		return gitErr("diff --binary", err)
	}
	cache := filepath.Join(root, ".lets", "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return ioErr(err)
	}
	patchPath := filepath.Join(cache, "integrate-"+o.Run+"-"+o.Chunk+".patch")
	if err := fsutil.AtomicWriteBytes(patchPath, patch, 0o600); err != nil {
		return ioErr(err)
	}
	r.PatchPath = patchPath
	if _, err := gitOut(ctx, root, "apply", "--index", "--binary", "--whitespace=nowarn", patchPath); err != nil {
		return gitErr("apply --index", err)
	}
	r.step(StepOK, "patch applied to the index and worktree: "+patchPath)
	_, cachedErr := gitOut(ctx, root, "diff", "--cached", "--quiet", head)
	_, wtErr := gitOut(ctx, root, "diff", "--quiet")
	if cachedErr == nil && wtErr == nil {
		r.step(StepOK, "index and worktree equal the picked tree "+head)
		return nil
	}
	e := &Error{Code: ExitVerifyMismatch, Kind: "verify_mismatch", Message: "the applied index or worktree differ from the picked tree " + head + "; the patch was reverse-applied"}
	if err := reverseApply(ctx, root, patchPath); err != nil {
		e.Message = "the applied index or worktree differ from the picked tree " + head + ", and the reverse-apply failed: " + err.Error()
		e.Remediation = "the patch's files are still staged: " + strings.Join(r.Files, ", ") + "; inspect `git status`; the patch is " + patchPath
	}
	return e
}

// reverseApply undoes a patch this call applied, index and worktree together.
// It refuses (and changes nothing) when the worktree no longer matches the
// index - the caller then gets the paths and fixes them by hand.
func reverseApply(ctx context.Context, root, patchPath string) error {
	_, err := gitOut(ctx, root, "apply", "-R", "--index", "--binary", "--whitespace=nowarn", patchPath)
	return err
}

// Revert undoes a rejected chunk's patch in the caller's tree, index and
// worktree together, in three steps that each stop before touching anything:
// the patch's paths must have index == worktree (59), a staged change against
// HEAD (58 revert_nothing_staged - a committed chunk is never undone here), and a
// reverse that applies cleanly (58); after it the paths must be back to HEAD (57).
// The patch file stays.
func Revert(ctx context.Context, root string, o Options) (*Result, error) {
	r := newResult("revert")
	if o.Patch == "" || o.From != "" || o.Since != "" {
		return r, fail(r, &Error{Code: ExitUsage, Kind: "usage", Message: "--revert takes --patch <path> and no --from / --since"})
	}
	patch := o.Patch
	if !filepath.IsAbs(patch) {
		patch = filepath.Join(root, patch)
	}
	if fi, err := os.Stat(patch); err != nil || !fi.Mode().IsRegular() {
		return r, fail(r, &Error{Code: ExitUsage, Kind: "patch_missing", Message: "--patch " + o.Patch + " is not a readable file", Remediation: "pass the patch_path an integrate call returned"})
	}
	r.PatchPath = patch
	unlock, e := lock(root, o.LockDeadline)
	if e != nil {
		return r, fail(r, e)
	}
	defer unlock()

	paths, err := patchPaths(ctx, root, patch)
	if err != nil {
		return r, fail(r, gitErr("apply --numstat", err))
	}
	if len(paths) == 0 {
		r.OK = true
		r.step(StepSkip, "the patch touches no path - nothing to revert")
		return r, nil
	}
	r.Files = paths
	scoped := func(args ...string) []string {
		return append(append([]string{"--literal-pathspecs"}, args...), append([]string{"--"}, paths...)...)
	}

	if _, err := gitOut(ctx, root, scoped("diff", "--quiet")...); err != nil {
		if exitStatus(err) == 1 {
			return r, fail(r, &Error{Code: ExitRevertIndexDiffers, Kind: "revert_index_differs", Message: "the worktree differs from the index on the patch's paths: " + strings.Join(paths, ", ") + "; nothing was touched", Remediation: "stage or drop those edits, then revert"})
		}
		return r, fail(r, gitErr("diff --quiet", err))
	}
	r.step(StepOK, "index == worktree on the patch's paths")
	if _, err := gitOut(ctx, root, scoped("diff", "--cached", "--quiet", "HEAD")...); err == nil {
		return r, fail(r, &Error{Code: ExitRevertConflictingEdit, Kind: "revert_nothing_staged", Message: "the patch's paths have no staged change against HEAD: " + strings.Join(paths, ", ") + " - the chunk is committed or was never applied; nothing was touched", Remediation: "to undo a committed chunk, git revert that commit"})
	} else if exitStatus(err) != 1 {
		return r, fail(r, gitErr("diff --cached --quiet", err))
	}
	if _, err := gitOut(ctx, root, "apply", "-R", "--check", "--index", "--binary", "--whitespace=nowarn", patch); err != nil {
		return r, fail(r, &Error{Code: ExitRevertConflictingEdit, Kind: "revert_conflicting_edit", Message: "the patch no longer reverses cleanly on " + strings.Join(paths, ", ") + " (" + err.Error() + "); nothing was touched", Remediation: "an edit after the integrate overlaps the patch - resolve it by hand"})
	}
	if _, err := gitOut(ctx, root, "apply", "-R", "--index", "--binary", "--whitespace=nowarn", patch); err != nil {
		return r, fail(r, gitErr("apply -R --index", err))
	}
	r.step(StepOK, "patch reverse-applied to the index and worktree")
	_, cachedErr := gitOut(ctx, root, scoped("diff", "--cached", "--quiet", "HEAD")...)
	_, wtErr := gitOut(ctx, root, scoped("diff", "--quiet", "HEAD")...)
	if cachedErr != nil || wtErr != nil {
		return r, fail(r, &Error{Code: ExitVerifyMismatch, Kind: "revert_verify_mismatch", Message: "the reverse WAS applied to the index and worktree; these paths still differ from HEAD (other staged edits?): " + strings.Join(paths, ", "), Remediation: "inspect `git status`; the patch is " + patch})
	}
	r.step(StepOK, "the patch's paths are back to HEAD; the patch file stays at "+patch)
	r.OK = true
	return r, nil
}

// patchPaths lists every path a patch touches, both sides of a rename: `git
// apply --numstat -z` names only a rename's destination, so the forward and the
// reverse listing are merged. A record with an empty path field (a git that
// prints the rename as two paths after it) takes the two paths that follow.
func patchPaths(ctx context.Context, root, patch string) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	for _, dir := range [][]string{nil, {"-R"}} {
		out, err := gitRaw(ctx, root, append(append([]string{"apply"}, dir...), "--numstat", "-z", patch)...)
		if err != nil {
			return nil, err
		}
		fields := strings.Split(string(out), "\x00")
		for i := 0; i < len(fields); i++ {
			parts := strings.SplitN(fields[i], "\t", 3)
			if len(parts) != 3 {
				continue
			}
			names := []string{parts[2]}
			if parts[2] == "" && i+2 < len(fields) {
				names = []string{fields[i+1], fields[i+2]}
				i += 2
			}
			for _, n := range names {
				if n != "" && !seen[n] {
					seen[n] = true
					paths = append(paths, n)
				}
			}
		}
	}
	return paths, nil
}

// temp is the temporary worktree of one run / chunk and its owner marker.
type temp struct {
	run, chunk, commonDir, path, marker string
}

// marker is the owner record written outside the temp worktree.
type marker struct {
	Run     string `json:"run"`
	Chunk   string `json:"chunk"`
	Pid     int    `json:"pid"`
	Created string `json:"created"`
	Path    string `json:"path"`
	Nonce   string `json:"nonce"`
}

func newTemp(ctx context.Context, root, run, chunk string) (*temp, *Error) {
	common, err := gitOut(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, gitErr("rev-parse --git-common-dir", err)
	}
	if real, err := filepath.EvalSymlinks(common); err == nil {
		common = real
	}
	p := filepath.Join(common, "lets-integrate", run+"-"+chunk)
	return &temp{run: run, chunk: chunk, commonDir: common, path: p, marker: p + ".owner"}, nil
}

func foreign(path, why string) *Error {
	return &Error{Code: ExitGitError, Kind: "temp_foreign", Message: path + " is occupied by something this run cannot prove it owns (" + why + "); nothing was touched", Remediation: "inspect and remove it by hand"}
}

// clearStale removes a temp worktree an earlier call left behind - only after
// proving it ours. A marker with no occupant is dropped; any occupant without
// that proof is refused untouched.
func (t *temp) clearStale(ctx context.Context, root string, r *Result) *Error {
	listed, e := t.listed(ctx, root)
	if e != nil {
		return e
	}
	_, statErr := os.Lstat(t.path)
	exists := statErr == nil
	if !listed && !exists {
		if err := os.Remove(t.marker); err == nil {
			r.step(StepOK, "stale owner marker removed (no temp worktree behind it)")
		}
		return nil
	}
	if !exists {
		// registered, directory gone: remove only the admin dir proved ours - never a
		// repo-wide prune, which would drop the registration of every missing worktree
		m, e := t.readMarker()
		if e != nil {
			return e
		}
		admin, e := t.ownAdminDir(m.Nonce)
		if e != nil {
			return e
		}
		if admin == "" {
			r.step(StepWarn, "no admin dir under "+filepath.Join(t.commonDir, "worktrees")+" names "+t.path+" with this run's nonce - left as is")
			return foreign(t.path, "registered with its directory missing, and no admin dir proves it ours")
		}
		if err := os.RemoveAll(admin); err != nil {
			return ioErr(err)
		}
		_ = os.Remove(t.marker)
		r.step(StepOK, "stale temp registration removed: "+t.path+" (admin dir "+admin+")")
		return nil
	}
	if e := t.removeOwned(ctx, root); e != nil {
		return e
	}
	r.step(StepOK, "stale temp worktree removed: "+t.path)
	return nil
}

// ownAdminDir returns the one `<common-dir>/worktrees/*` admin dir whose gitdir
// file names exactly t.path/.git and that holds nonce, or "" when none does.
func (t *temp) ownAdminDir(nonce string) (string, *Error) {
	dirs, err := filepath.Glob(filepath.Join(t.commonDir, "worktrees", "*"))
	if err != nil {
		return "", ioErr(err)
	}
	want := filepath.Join(t.path, ".git")
	for _, d := range dirs {
		gd, err := os.ReadFile(filepath.Join(d, "gitdir"))
		if err != nil || filepath.Clean(strings.TrimSpace(string(gd))) != want {
			continue
		}
		if n, err := os.ReadFile(filepath.Join(d, nonceFile)); err == nil && strings.TrimSpace(string(n)) == nonce {
			return d, nil
		}
	}
	return "", nil
}

// readMarker returns the owner marker when it exists and names this run, chunk and path.
func (t *temp) readMarker() (*marker, *Error) {
	b, err := os.ReadFile(t.marker)
	if err != nil {
		return nil, foreign(t.path, "no owner marker")
	}
	var m marker
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, foreign(t.path, "unreadable owner marker")
	}
	if m.Run != t.run || m.Chunk != t.chunk || m.Path != t.path || m.Nonce == "" {
		return nil, foreign(t.path, "owner marker names another run, chunk or path")
	}
	return &m, nil
}

// listed reports whether `git worktree list --porcelain` registers exactly t.path.
func (t *temp) listed(ctx context.Context, root string) (bool, *Error) {
	out, err := tempGit(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return false, gitErr("worktree list", err)
	}
	for _, line := range strings.Split(out, "\n") {
		p, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		if filepath.Clean(p) == t.path {
			return true, nil
		}
	}
	return false, nil
}

// removeOwned proves the temp worktree ours - marker matches, the path is a
// registered worktree, and its own gitdir holds the marker's nonce - and only
// then runs `worktree remove --force` (it deletes the admin dir too) and drops
// the marker.
func (t *temp) removeOwned(ctx context.Context, root string) *Error {
	m, e := t.readMarker()
	if e != nil {
		return e
	}
	listed, e := t.listed(ctx, root)
	if e != nil {
		return e
	}
	if !listed {
		return foreign(t.path, "not a registered worktree")
	}
	gitdir, err := tempGit(ctx, t.path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return foreign(t.path, "no gitdir")
	}
	if real, err := filepath.EvalSymlinks(gitdir); err == nil {
		gitdir = real
	}
	nonce, err := os.ReadFile(filepath.Join(gitdir, nonceFile))
	if gitdir == t.commonDir || err != nil || strings.TrimSpace(string(nonce)) != m.Nonce {
		return foreign(t.path, "its gitdir does not hold this run's nonce")
	}
	if _, err := tempGit(ctx, root, "worktree", "remove", "--force", t.path); err != nil {
		return gitErr("worktree remove", err)
	}
	if err := os.Remove(t.marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ioErr(err)
	}
	return nil
}

// create writes the owner marker, adds the detached temp worktree at base and
// stores the nonce in the worktree's own gitdir (never its working tree, so it
// never enters a diff).
func (t *temp) create(ctx context.Context, root, base string) *Error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o700); err != nil {
		return ioErr(err)
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ioErr(err)
	}
	m := marker{Run: t.run, Chunk: t.chunk, Pid: os.Getpid(), Created: time.Now().UTC().Format(time.RFC3339), Path: t.path, Nonce: hex.EncodeToString(raw)}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := fsutil.AtomicWriteBytes(t.marker, append(b, '\n'), 0o600); err != nil {
		return ioErr(err)
	}
	if _, err := tempGit(ctx, root, "worktree", "add", "--detach", t.path, base); err != nil {
		_ = os.Remove(t.marker)
		return gitErr("worktree add", err)
	}
	gitdir, err := tempGit(ctx, t.path, "rev-parse", "--absolute-git-dir")
	if err == nil {
		err = os.WriteFile(filepath.Join(gitdir, nonceFile), []byte(m.Nonce+"\n"), 0o600)
	}
	if err != nil {
		// just added by this call and not yet provable: remove it directly
		_, _ = tempGit(ctx, root, "worktree", "remove", "--force", t.path)
		_ = os.Remove(t.marker)
		return ioErr(err)
	}
	return nil
}

// pick cherry-picks since..from in the temp worktree. A conflict names its files,
// aborts and returns ExitConflict; the caller's tree is never touched.
func (t *temp) pick(ctx context.Context, since, from string, r *Result) *Error {
	_, err := tempGit(ctx, t.path, "cherry-pick", "--allow-empty", "--empty=drop", since+".."+from)
	if err == nil {
		return nil
	}
	unmerged, _ := tempGit(ctx, t.path, "diff", "--name-only", "--diff-filter=U")
	commit, _ := tempGit(ctx, t.path, "rev-parse", "--verify", "--quiet", "CHERRY_PICK_HEAD")
	_, _ = tempGit(ctx, t.path, "cherry-pick", "--abort")
	if unmerged == "" {
		return gitErr("cherry-pick", err)
	}
	r.Conflict = &Conflict{Commit: commit, Files: strings.Split(unmerged, "\n")}
	return &Error{Code: ExitConflict, Kind: "conflict", Message: "cherry-pick of " + commit + " conflicts in: " + strings.ReplaceAll(unmerged, "\n", ", "), Remediation: "send the implementer an amendment, or stop; the caller tree is untouched"}
}

// lock takes .lets/locks/integrate.lock until deadline.
func lock(root string, deadline time.Duration) (func(), *Error) {
	if deadline == 0 {
		deadline = defaultLockDeadline
	}
	dir := filepath.Join(root, ".lets", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, ioErr(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "integrate.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, ioErr(err)
	}
	if err := fsutil.TryLockFile(f, time.Now().Add(deadline)); err != nil {
		_ = f.Close()
		if errors.Is(err, fsutil.ErrLockBusy) {
			return nil, &Error{Code: ExitGeneric, Kind: "lock_busy", Message: "another lets integrate call holds " + f.Name(), Remediation: "retry"}
		}
		return nil, ioErr(err)
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

var gitVersionRe = regexp.MustCompile(`(\d+)\.(\d+)`)

func checkGitVersion(ctx context.Context, root string) *Error {
	out, err := gitOut(ctx, root, "version")
	if err != nil {
		return gitErr("version", err)
	}
	m := gitVersionRe.FindStringSubmatch(out)
	if m == nil {
		return &Error{Code: ExitGitError, Kind: "git_too_old", Message: "cannot read the git version from " + strconv.Quote(out)}
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < minGitMajor || (major == minGitMajor && minor < minGitMinor) {
		return &Error{Code: ExitGitError, Kind: "git_too_old", Message: fmt.Sprintf("%s; lets integrate needs git >= %d.%d (cherry-pick --empty=drop)", out, minGitMajor, minGitMinor), Remediation: "upgrade git"}
	}
	return nil
}

// tempGit runs git in or on the temp worktree with hooks off (and no signing:
// its commits are never published), trimmed.
func tempGit(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := tempGitRaw(ctx, dir, args...)
	return strings.TrimSpace(string(out)), err
}

func tempGitRaw(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitRaw(ctx, dir, append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgSign=false"}, args...)...)
}

// gitOut runs git in the caller's tree, trimmed.
func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitRaw(ctx, dir, args...)
	return strings.TrimSpace(string(out)), err
}

func gitRaw(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, &gitError{args: args, err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return out, nil
}

// gitEnv drops the variables that would point git at another repository or index
// than the -C directory (set, for one, when lets runs inside a git hook).
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY":
			continue
		}
		env = append(env, kv)
	}
	return env
}

type gitError struct {
	args   []string
	err    error
	stderr string
}

func (g *gitError) Error() string {
	if g.stderr != "" {
		return fmt.Sprintf("git %s: %v: %s", strings.Join(g.args, " "), g.err, g.stderr)
	}
	return fmt.Sprintf("git %s: %v", strings.Join(g.args, " "), g.err)
}

func (g *gitError) Unwrap() error { return g.err }

// exitStatus is the exit status of a failed git call, or -1.
func exitStatus(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func gitErr(what string, err error) *Error {
	return &Error{Code: ExitGitError, Kind: "git_error", Message: what + ": " + err.Error()}
}

func ioErr(err error) *Error {
	return &Error{Code: ExitGeneric, Kind: "io_error", Message: err.Error()}
}
