//go:build unix

package integratecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// requireGit245 skips below git 2.45 - Run refuses there (cherry-pick --empty=drop).
func requireGit245(t *testing.T) {
	t.Helper()
	out, err := exec.Command("git", "version").Output()
	if err != nil {
		t.Skip("git not available")
	}
	m := regexp.MustCompile(`(\d+)\.(\d+)`).FindStringSubmatch(string(out))
	if m == nil {
		t.Skipf("cannot read git version %q", out)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < 2 || (major == 2 && minor < 45) {
		t.Skipf("git %s.%s < 2.45 (cherry-pick --empty=drop)", m[1], m[2])
	}
}

func realTempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// git runs git in dir and returns its trimmed stdout; t.Fatal on failure.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commit stages everything in dir, commits and returns the new sha.
func commit(t *testing.T, dir, msg string) string {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
	return git(t, dir, "rev-parse", "HEAD")
}

// newRepo is a caller repository on main with a.txt and .lets/ ignored; it
// returns the repo and its base sha.
func newRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := realTempDir(t)
	git(t, repo, "-c", "init.defaultBranch=main", "init", "-q")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "test")
	write(t, filepath.Join(repo, ".gitignore"), ".lets/\n")
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\nthree\n")
	return repo, commit(t, repo, "base")
}

// agentWorktree is an isolated implementer's worktree on a new branch from start.
func agentWorktree(t *testing.T, repo, branch, start string) string {
	t.Helper()
	wt := filepath.Join(realTempDir(t), "agent")
	git(t, repo, "worktree", "add", "-q", "-b", branch, wt, start)
	return wt
}

func run(t *testing.T, repo string, o Options) (*Result, error) {
	t.Helper()
	if o.Run == "" {
		o.Run = "r1"
	}
	if o.Chunk == "" {
		o.Chunk = "c1"
	}
	return Run(context.Background(), repo, o)
}

func exitOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.ExitCode()
	}
	return -1
}

func kindOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

// assertNoTemp fails when a lets-integrate worktree or owner marker is left.
func assertNoTemp(t *testing.T, repo string) {
	t.Helper()
	if list := git(t, repo, "worktree", "list", "--porcelain"); strings.Contains(list, "lets-integrate") {
		t.Errorf("a lets-integrate worktree is left:\n%s", list)
	}
	common := git(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if m, _ := filepath.Glob(filepath.Join(common, "lets-integrate", "*.owner")); len(m) > 0 {
		t.Errorf("owner markers left: %v", m)
	}
}

// assertUntouched fails when the caller moved HEAD or has any change.
func assertUntouched(t *testing.T, repo, head string) {
	t.Helper()
	if got := git(t, repo, "rev-parse", "HEAD"); got != head {
		t.Errorf("caller HEAD moved: %s -> %s", head, got)
	}
	if st := git(t, repo, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Errorf("caller tree changed:\n%s", st)
	}
}

func tempFor(t *testing.T, repo, runID, chunk string) *temp {
	t.Helper()
	tp, e := newTemp(context.Background(), repo, runID, chunk)
	if e != nil {
		t.Fatal(e)
	}
	return tp
}

func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion changed to %d - update consumers (commands/execute.md) and this test", SchemaVersion)
	}
	toMap := func(v any) map[string]any {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	require := func(m map[string]any, keys ...string) {
		t.Helper()
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("missing key %q in %v", k, m)
			}
		}
	}
	core := []string{"schema_version", "ok", "subcommand", "steps", "picked", "files", "patch_path"}

	ok := newResult("integrate")
	ok.OK = true
	ok.Picked = []string{"abc"}
	require(toMap(ok), core...)

	r := newResult("integrate")
	_ = fail(r, &Error{Code: ExitConflict, Kind: "conflict", Message: "m"})
	r.Conflict = &Conflict{Commit: "abc", Files: []string{"a.txt"}}
	m := toMap(r)
	require(m, append(core, "error", "conflict")...)
	require(m["error"].(map[string]any), "kind", "message")
	require(m["conflict"].(map[string]any), "commit", "files")
	if m["picked"] == nil || m["files"] == nil {
		t.Error("picked and files must be [] on failure, never null")
	}
}

func TestIntegrate_Clean(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "a.txt"), "one\nTWO\nthree\n")
	write(t, filepath.Join(wt, "b.txt"), "new\n")
	src := commit(t, wt, "c1")

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK || len(res.Picked) != 1 || strings.Join(res.Files, ",") != "a.txt,b.txt" {
		t.Fatalf("result = %+v", res)
	}
	if git(t, repo, "rev-parse", "HEAD") != base {
		t.Error("integrate must never commit")
	}
	if staged := git(t, repo, "diff", "--cached", "--name-only"); staged != "a.txt\nb.txt" {
		t.Errorf("staged = %q", staged)
	}
	if git(t, repo, "diff", "--cached", src) != "" {
		t.Error("the index must equal the source commit's tree")
	}
	want := filepath.Join(repo, ".lets", "cache", "integrate-r1-c1.patch")
	if res.PatchPath != want {
		t.Errorf("patch_path = %q, want %q", res.PatchPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("patch file: %v", err)
	}
	assertNoTemp(t, repo)

	// an empty range integrates nothing
	git(t, repo, "commit", "-q", "-m", "lead commit")
	res, err = run(t, repo, Options{From: src, Since: src, Chunk: "c2"})
	if err != nil || !res.OK || len(res.Picked) != 0 {
		t.Fatalf("empty range: %+v, %v", res, err)
	}
}

func TestIntegrate_FromSha(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	sha1 := commit(t, wt, "c1")
	write(t, filepath.Join(wt, "c.txt"), "c\n")
	commit(t, wt, "c2 - not part of this chunk")

	res, err := run(t, repo, Options{From: sha1, Since: base})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Join(res.Files, ",") != "b.txt" {
		t.Errorf("a sha --from must stop at that sha, files = %v", res.Files)
	}
	assertNoTemp(t, repo)
}

func TestIntegrate_ConflictCallerUntouched(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "a.txt"), "one\nAGENT\nthree\n")
	commit(t, wt, "agent edit")
	write(t, filepath.Join(repo, "a.txt"), "one\nCALLER\nthree\n")
	head := commit(t, repo, "caller edit")

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if exitOf(err) != ExitConflict {
		t.Fatalf("exit = %d (%v), want %d", exitOf(err), err, ExitConflict)
	}
	if res.Conflict == nil || strings.Join(res.Conflict.Files, ",") != "a.txt" || res.Conflict.Commit == "" {
		t.Errorf("conflict = %+v", res.Conflict)
	}
	assertUntouched(t, repo, head)
	assertNoTemp(t, repo)
}

func TestIntegrate_EmptyCommitDropped(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "same\n")
	commit(t, wt, "already in the caller")
	write(t, filepath.Join(wt, "c.txt"), "c\n")
	commit(t, wt, "new")
	write(t, filepath.Join(repo, "b.txt"), "same\n")
	commit(t, repo, "caller has b.txt")

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Picked) != 1 || strings.Join(res.Files, ",") != "c.txt" {
		t.Errorf("the commit that became empty must be dropped: %+v", res)
	}
	assertNoTemp(t, repo)
}

func TestIntegrate_BinaryAndRename(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	git(t, wt, "mv", "a.txt", "renamed.txt")
	bin := []byte{0, 1, 2, 255, 0, 'x', 0}
	if err := os.WriteFile(filepath.Join(wt, "bin.dat"), bin, 0o644); err != nil {
		t.Fatal(err)
	}
	src := commit(t, wt, "rename + binary")

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.Join(res.Files, ","); got != "a.txt,bin.dat,renamed.txt" {
		t.Errorf("files = %q, want both sides of the rename and the binary", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "a.txt")); !os.IsNotExist(err) {
		t.Error("a.txt must be gone after the rename")
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "bin.dat")); !bytes.Equal(got, bin) {
		t.Errorf("binary content = %v, want %v", got, bin)
	}
	if git(t, repo, "diff", "--cached", src) != "" {
		t.Error("the index must equal the source commit's tree")
	}
	assertNoTemp(t, repo)
}

func TestIntegrate_RefusesMerge(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	commit(t, wt, "b")
	git(t, wt, "checkout", "-q", "-b", "side", base)
	write(t, filepath.Join(wt, "c.txt"), "c\n")
	commit(t, wt, "c")
	git(t, wt, "checkout", "-q", "worktree-agent-x")
	git(t, wt, "merge", "-q", "--no-ff", "-m", "merge side", "side")

	_, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if exitOf(err) != ExitMergeInRange {
		t.Fatalf("exit = %d (%v), want %d", exitOf(err), err, ExitMergeInRange)
	}
	assertUntouched(t, repo, base)
	assertNoTemp(t, repo)
}

func TestIntegrate_DirtyIncludingUntracked(t *testing.T) {
	requireGit245(t)
	for _, tc := range []struct{ name, path, content string }{
		{"untracked", "new/untracked.txt", "x\n"},
		{"tracked", "a.txt", "changed\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, base := newRepo(t)
			wt := agentWorktree(t, repo, "worktree-agent-x", base)
			write(t, filepath.Join(wt, "b.txt"), "b\n")
			commit(t, wt, "b")
			write(t, filepath.Join(repo, tc.path), tc.content)

			_, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
			if exitOf(err) != ExitDirtyTree {
				t.Fatalf("exit = %d (%v), want %d", exitOf(err), err, ExitDirtyTree)
			}
			if git(t, repo, "diff", "--cached", "--name-only") != "" {
				t.Error("nothing may be staged")
			}
			assertNoTemp(t, repo)
		})
	}
}

func TestIntegrate_Preconditions(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	commit(t, wt, "b")
	write(t, filepath.Join(repo, "c.txt"), "c\n")
	callerOnly := commit(t, repo, "caller only")

	if _, err := run(t, repo, Options{From: "no-such-branch", Since: base}); exitOf(err) != ExitFromMissing {
		t.Errorf("missing --from: exit %d (%v)", exitOf(err), err)
	}
	if _, err := run(t, repo, Options{From: "worktree-agent-x", Since: callerOnly}); exitOf(err) != ExitSinceNotAncestor {
		t.Errorf("--since not an ancestor: exit %d (%v)", exitOf(err), err)
	}
	git(t, repo, "checkout", "-q", "--detach")
	if _, err := run(t, repo, Options{From: "worktree-agent-x", Since: base}); exitOf(err) != ExitDetachedHead {
		t.Errorf("detached HEAD: exit %d (%v)", exitOf(err), err)
	}
	assertNoTemp(t, repo)
}

func TestIntegrate_RejectsPathLikeRunChunk(t *testing.T) {
	repo, base := newRepo(t)
	common := git(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	for _, bad := range []string{"..", "../x", "a/b", "A", "a.b", "a b", strings.Repeat("a", 41)} {
		for _, o := range []Options{{Run: bad, Chunk: "c1"}, {Run: "r1", Chunk: bad}} {
			o.From, o.Since = base, base
			_, err := Run(context.Background(), repo, o)
			if exitOf(err) != ExitUsage || kindOf(err) != "invalid_name" {
				t.Errorf("run %q chunk %q: exit %d (%v), want %d invalid_name", o.Run, o.Chunk, exitOf(err), err, ExitUsage)
			}
		}
	}
	for _, p := range []string{filepath.Join(repo, ".lets"), filepath.Join(common, "lets-integrate")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s was created before the names were validated", p)
		}
	}
}

func TestIntegrate_OwnStaleRemoved(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	commit(t, wt, "b")
	stale := tempFor(t, repo, "r1", "c1")
	if e := stale.create(context.Background(), repo, base); e != nil {
		t.Fatal(e) // an earlier call that died before its cleanup
	}

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stepsText(res), "stale temp worktree removed") {
		t.Errorf("steps = %v", res.Steps)
	}
	assertNoTemp(t, repo)
}

func TestIntegrate_StaleTempPruned(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	commit(t, wt, "b")
	stale := tempFor(t, repo, "r1", "c1")
	if e := stale.create(context.Background(), repo, base); e != nil {
		t.Fatal(e)
	}
	if err := os.RemoveAll(stale.path); err != nil { // directory gone, registration left
		t.Fatal(err)
	}

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stepsText(res), "stale temp registration removed") {
		t.Errorf("steps = %v", res.Steps)
	}
	assertNoTemp(t, repo)
}

func TestIntegrate_ForeignOccupantRefused(t *testing.T) {
	requireGit245(t)
	for _, tc := range []struct {
		name   string
		marker *marker
	}{
		{"no marker", nil},
		{"marker of another run", &marker{Run: "r9", Chunk: "c1", Nonce: "abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, base := newRepo(t)
			wt := agentWorktree(t, repo, "worktree-agent-x", base)
			write(t, filepath.Join(wt, "b.txt"), "b\n")
			commit(t, wt, "b")
			tp := tempFor(t, repo, "r1", "c1")
			git(t, repo, "worktree", "add", "-q", "--detach", tp.path, base)
			if tc.marker != nil {
				tc.marker.Path = tp.path
				b, _ := json.Marshal(tc.marker)
				write(t, tp.marker, string(b))
			}
			assertForeign(t, repo, tp, base)
		})
	}
}

func TestIntegrate_SamePathDifferentWorktreeRefused(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	commit(t, wt, "b")
	tp := tempFor(t, repo, "r1", "c1")
	b, _ := json.Marshal(marker{Run: "r1", Chunk: "c1", Path: tp.path, Nonce: "0123456789abcdef0123456789abcdef"})
	write(t, tp.marker, string(b))                                   // a valid marker ...
	git(t, repo, "worktree", "add", "-q", "--detach", tp.path, base) // ... over a worktree with no nonce
	assertForeign(t, repo, tp, base)
}

// assertForeign runs integrate against an occupant it cannot prove its own and
// checks the refusal touched nothing.
func assertForeign(t *testing.T, repo string, tp *temp, base string) {
	t.Helper()
	_, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if exitOf(err) != ExitGitError || kindOf(err) != "temp_foreign" {
		t.Fatalf("exit %d kind %q (%v), want %d temp_foreign", exitOf(err), kindOf(err), err, ExitGitError)
	}
	if !strings.Contains(git(t, repo, "worktree", "list", "--porcelain"), "worktree "+tp.path) {
		t.Error("the foreign worktree must stay registered")
	}
	if _, err := os.Stat(tp.path); err != nil {
		t.Errorf("the foreign worktree must stay on disk: %v", err)
	}
	assertUntouched(t, repo, base)
}

func TestIntegrate_HooksOffSentinel(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	commit(t, wt, "b")

	sentinel := filepath.Join(realTempDir(t), "sentinel")
	common := git(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	for _, hook := range []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit", "post-checkout", "post-rewrite"} {
		p := filepath.Join(common, "hooks", hook)
		write(t, p, "#!/bin/sh\necho "+hook+" >> '"+sentinel+"'\n")
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := run(t, repo, Options{From: "worktree-agent-x", Since: base}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if b, err := os.ReadFile(sentinel); err == nil {
		t.Fatalf("a hook ran during integrate:\n%s", b)
	}
	assertNoTemp(t, repo)

	git(t, repo, "commit", "-q", "-m", "lead commit") // control: the hooks are live
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal("control failed - the hooks never run, so this test proves nothing")
	}
}

func TestIntegrate_SecondRangeFromSourceSha(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "a.txt"), "one\nTWO\nthree\n")
	sha1 := commit(t, wt, "c1 edit a")

	if _, err := run(t, repo, Options{From: sha1, Since: base, Chunk: "c1"}); err != nil {
		t.Fatalf("integrate c1: %v", err)
	}
	git(t, repo, "commit", "-q", "-m", "c1 edit a") // the lead's commit; its sha is caller-side

	write(t, filepath.Join(wt, "d.txt"), "d\n")
	commit(t, wt, "c2 add d")
	write(t, filepath.Join(wt, "a.txt"), "one\nTWO!\nthree\n")
	commit(t, wt, "fixup! c1 edit a")

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: sha1, Chunk: "c2"})
	if err != nil {
		t.Fatalf("integrate c2 since the source sha: %v", err)
	}
	if len(res.Picked) != 2 || strings.Join(res.Files, ",") != "a.txt,d.txt" {
		t.Errorf("result = %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(got) != "one\nTWO!\nthree\n" {
		t.Errorf("a.txt = %q", got)
	}
	assertNoTemp(t, repo)
}

func stepsText(r *Result) string {
	var b strings.Builder
	for _, s := range r.Steps {
		b.WriteString(s.Status + " " + s.Message + "\n")
	}
	return b.String()
}

func TestIntegrate_VerifyMismatch(t *testing.T) {
	requireGit245(t)
	repo, _ := newRepo(t)
	// a clean filter that is not idempotent: the worktree never cleans to the index
	git(t, repo, "config", "filter.drift.clean", "sed s/^/x/")
	write(t, filepath.Join(repo, ".gitattributes"), "*.dat filter=drift\n")
	base := commit(t, repo, "attributes")
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.dat"), "b\n")
	commit(t, wt, "b")

	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: base})
	if exitOf(err) != ExitVerifyMismatch {
		t.Fatalf("exit = %d (%v), want %d", exitOf(err), err, ExitVerifyMismatch)
	}
	if res.OK {
		t.Error("a verify mismatch is never ok")
	}
	// the worktree cannot clean to the index here, so `apply -R --index` refuses
	// and the error must say so and name the staged files, never claim a revert
	if !strings.Contains(err.Error(), "reverse-apply failed") || !strings.Contains(err.Error(), "b.dat") {
		t.Errorf("a failed reverse-apply must be reported with its files: %v", err)
	}
	assertNoTemp(t, repo)
}

// userMissingWorktree registers a worktree LETS did not create and deletes its
// directory, as an unmounted volume or a hand move would.
func userMissingWorktree(t *testing.T, repo string) string {
	t.Helper()
	p := filepath.Join(realTempDir(t), "user-wt")
	git(t, repo, "worktree", "add", "-q", "--detach", p, "HEAD")
	if err := os.RemoveAll(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIntegrate_ForeignMissingWorktreeKept(t *testing.T) {
	requireGit245(t)
	repo, base := newRepo(t)
	user := userMissingWorktree(t, repo)
	stillListed := func(when string) {
		t.Helper()
		if !strings.Contains(git(t, repo, "worktree", "list", "--porcelain"), "worktree "+user) {
			t.Errorf("after %s: the user's missing worktree lost its registration", when)
		}
	}
	wt := agentWorktree(t, repo, "worktree-agent-x", base)
	write(t, filepath.Join(wt, "b.txt"), "b\n")
	sha1 := commit(t, wt, "b")

	if _, err := run(t, repo, Options{From: sha1, Since: base, Chunk: "c1"}); err != nil {
		t.Fatalf("normal run: %v", err)
	}
	stillListed("a normal run")
	git(t, repo, "commit", "-q", "-m", "lead commit")

	write(t, filepath.Join(wt, "c.txt"), "c\n")
	commit(t, wt, "c")
	stale := tempFor(t, repo, "r1", "c2")
	if e := stale.create(context.Background(), repo, git(t, repo, "rev-parse", "HEAD")); e != nil {
		t.Fatal(e)
	}
	if err := os.RemoveAll(stale.path); err != nil {
		t.Fatal(err)
	}
	res, err := run(t, repo, Options{From: "worktree-agent-x", Since: sha1, Chunk: "c2"})
	if err != nil {
		t.Fatalf("run over a stale registration: %v", err)
	}
	if !strings.Contains(stepsText(res), "stale temp registration removed") {
		t.Errorf("steps = %v", res.Steps)
	}
	stillListed("the stale cleanup")
	assertNoTemp(t, repo)
}
