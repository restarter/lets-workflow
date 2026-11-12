//go:build unix

package worktreecmd_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// includeRepo is a main checkout with .lets and a committed .gitignore and
// .worktreeinclude (create needs a clean main tree).
func includeRepo(t *testing.T, gitignore, include string) string {
	t.Helper()
	repo := initRepo(t)
	mustMkdir(t, filepath.Join(repo, ".lets", "sessions"))
	mustWrite(t, filepath.Join(repo, ".gitignore"), ".lets/\n.worktrees/\n"+gitignore, 0o644)
	runIn(t, repo, "git", "add", ".gitignore")
	if include != "" {
		mustWrite(t, filepath.Join(repo, ".worktreeinclude"), include, 0o644)
		runIn(t, repo, "git", "add", ".worktreeinclude")
	}
	runIn(t, repo, "git", "commit", "-q", "-m", "ignore + include")
	return repo
}

func mustCreate(t *testing.T, repo string) *worktreecmd.CreateResult {
	t.Helper()
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "x", Mode: worktreecmd.BranchAuto})
	if err != nil || !res.OK {
		t.Fatalf("create: err=%v res=%+v", err, res)
	}
	return res
}

// includeSteps returns the steps whose message starts with "worktreeinclude:".
func includeSteps(steps []worktreecmd.Step) []worktreecmd.Step {
	var out []worktreecmd.Step
	for _, s := range steps {
		if strings.HasPrefix(s.Message, "worktreeinclude:") {
			out = append(out, s)
		}
	}
	return out
}

func includeStep(steps []worktreecmd.Step, status, substr string) bool {
	for _, s := range steps {
		if s.Status == status && strings.Contains(s.Message, substr) {
			return true
		}
	}
	return false
}

func porcelain(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestWorktreeInclude_NoFile(t *testing.T) {
	repo := includeRepo(t, "local.md\n", "")
	mustWrite(t, filepath.Join(repo, "local.md"), "l", 0o644)
	res := mustCreate(t, repo)
	if s := includeSteps(res.Steps); len(s) != 0 {
		t.Errorf("no .worktreeinclude -> no step, got %+v", s)
	}
	if exists(filepath.Join(res.Worktree.Path, "local.md")) {
		t.Error("nothing is copied without .worktreeinclude")
	}
}

func TestWorktreeInclude_CreateCopies(t *testing.T) {
	repo := includeRepo(t, "local.md\n.claude/settings.local.json\n.env\n", "local.md\n.claude/settings.local.json\n")
	mustWrite(t, filepath.Join(repo, "local.md"), "local notes\n", 0o600)
	mustWrite(t, filepath.Join(repo, ".claude", "settings.local.json"), `{"model":"opus","permissions":{"allow":["Bash(ls)"]}}`, 0o644)
	mustWrite(t, filepath.Join(repo, ".env"), "SECRET=1\n", 0o644)

	res := mustCreate(t, repo)
	wt := res.Worktree.Path
	if !includeStep(res.Steps, worktreecmd.StepOK, "worktreeinclude: copied 2 file(s)") {
		t.Errorf("want a copied-2 step: %+v", res.Steps)
	}
	data, err := os.ReadFile(filepath.Join(wt, "local.md"))
	if err != nil || string(data) != "local notes\n" {
		t.Errorf("local.md content: %q %v", data, err)
	}
	si, _ := os.Stat(filepath.Join(repo, "local.md"))
	di, _ := os.Stat(filepath.Join(wt, "local.md"))
	if si == nil || di == nil || si.Mode().Perm() != di.Mode().Perm() {
		t.Errorf("local.md mode: main %v, worktree %v", si, di)
	}
	if exists(filepath.Join(wt, ".env")) {
		t.Error(".env is not listed - must not be copied")
	}
	// settings.local.json: the user's keys survive AND the Step 10.5 merge adds the main .lets.
	raw, err := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("settings.local.json: %v\n%s", err, raw)
	}
	if got["model"] != "opus" {
		t.Errorf("user key model lost: %s", raw)
	}
	perms, _ := got["permissions"].(map[string]any)
	if allow, _ := perms["allow"].([]any); len(allow) != 1 || allow[0] != "Bash(ls)" {
		t.Errorf("user permissions.allow lost: %s", raw)
	}
	wantDir, _ := filepath.EvalSymlinks(filepath.Join(repo, ".lets"))
	dirs, _ := perms["additionalDirectories"].([]any)
	if !slices.Contains(dirs, any(wantDir)) {
		t.Errorf("additionalDirectories lacks %s: %s", wantDir, raw)
	}
	if p := porcelain(t, wt); p != "" {
		t.Errorf("worktree must stay clean:\n%s", p)
	}
}

func TestWorktreeInclude_TrackedNotCopied(t *testing.T) {
	repo := includeRepo(t, "", "tracked.md\n")
	mustWrite(t, filepath.Join(repo, "tracked.md"), "v1", 0o644)
	runIn(t, repo, "git", "add", "tracked.md")
	runIn(t, repo, "git", "commit", "-q", "-m", "tracked")
	res := mustCreate(t, repo)
	if includeStep(res.Steps, worktreecmd.StepOK, "worktreeinclude: copied") {
		t.Errorf("a tracked file arrives through git, not a copy: %+v", res.Steps)
	}
}

func TestWorktreeInclude_NotIgnoredNotCopied(t *testing.T) {
	repo := includeRepo(t, "", "loose.md\n")
	// Untracked but NOT ignored: create would refuse a dirty main, so adopt.
	wt := filepath.Join(realTempDir(t), "ext")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "ext", wt)
	mustWrite(t, filepath.Join(repo, "loose.md"), "l", 0o644)
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("adopt: %v %+v", err, res)
	}
	if exists(filepath.Join(wt, "loose.md")) {
		t.Error("an untracked, non-ignored file must not be copied")
	}
}

func TestWorktreeInclude_DirectoryPatternAndNegation(t *testing.T) {
	repo := includeRepo(t, ".claude/rules/\n*.local.md\n", ".claude/rules/\n*.local.md\n!keep.local.md\n")
	mustWrite(t, filepath.Join(repo, ".claude", "rules", "a.md"), "a", 0o644)
	mustWrite(t, filepath.Join(repo, ".claude", "rules", "sub", "b.md"), "b", 0o644)
	mustWrite(t, filepath.Join(repo, "notes.local.md"), "n", 0o644)
	mustWrite(t, filepath.Join(repo, "keep.local.md"), "k", 0o644)
	res := mustCreate(t, repo)
	wt := res.Worktree.Path
	for _, rel := range []string{".claude/rules/a.md", ".claude/rules/sub/b.md", "notes.local.md"} {
		if !exists(filepath.Join(wt, rel)) {
			t.Errorf("%s not copied: %+v", rel, res.Steps)
		}
	}
	if exists(filepath.Join(wt, "keep.local.md")) {
		t.Error("a negated file must not be copied")
	}
}

func TestWorktreeInclude_AdoptKeepsDifferentAndIsIdempotent(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-2")
	mustWrite(t, filepath.Join(repo, ".gitignore"), "local.md\nother.md\n", 0o644)
	mustWrite(t, filepath.Join(repo, ".worktreeinclude"), "local.md\nother.md\n", 0o644)
	mustWrite(t, filepath.Join(repo, "local.md"), "main", 0o644)
	mustWrite(t, filepath.Join(repo, "other.md"), "o", 0o644)
	mustWrite(t, filepath.Join(wt, "local.md"), "mine", 0o644)

	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("adopt: %v", err)
	}
	if !includeStep(res.Steps, worktreecmd.StepWarn, "kept the existing local.md") {
		t.Errorf("want a kept-the-existing warn: %+v", res.Steps)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, "local.md")); string(data) != "mine" {
		t.Errorf("an existing destination is never overwritten, got %q", data)
	}
	if !includeStep(res.Steps, worktreecmd.StepOK, "worktreeinclude: copied 1 file(s): other.md") {
		t.Errorf("other.md not copied: %+v", res.Steps)
	}

	_ = os.Remove(filepath.Join(wt, "local.md"))
	if _, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true}); err != nil {
		t.Fatal(err)
	}
	res, err = worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("re-adopt: %v", err)
	}
	if s := includeSteps(res.Steps); len(s) != 0 {
		t.Errorf("a repeated adopt copies nothing and warns nothing: %+v", s)
	}
}

func TestWorktreeInclude_ListFailureIsAWarn(t *testing.T) {
	repo := includeRepo(t, "", "")
	mustMkdir(t, filepath.Join(repo, ".worktreeinclude"))
	mustWrite(t, filepath.Join(repo, ".worktreeinclude", ".gitkeep"), "", 0o644)
	runIn(t, repo, "git", "add", ".worktreeinclude")
	runIn(t, repo, "git", "commit", "-q", "-m", "a dir")
	res := mustCreate(t, repo)
	s := includeSteps(res.Steps)
	if len(s) != 1 || s[0].Status != worktreecmd.StepWarn || !strings.Contains(s[0].Message, "could not list") {
		t.Errorf("want one could-not-list warn, got %+v", s)
	}
}

func TestWorktreeInclude_CopyRegularContainment(t *testing.T) {
	tmp := realTempDir(t)
	src := filepath.Join(tmp, "src.md")
	mustWrite(t, src, "payload", 0o644)
	outside := filepath.Join(tmp, "outside")
	mustMkdir(t, outside)
	dst := filepath.Join(tmp, "wt")
	mustMkdir(t, dst)
	if err := os.Symlink(outside, filepath.Join(dst, "link")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := worktreecmd.CopyRegularForTesting(root, src, "link/x.md", 0o644); err == nil {
		t.Error("a copy through a symlink leaving the worktree must fail")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("outside changed: %v", entries)
	}

	// no-replace: a destination created between the scan and the copy keeps its bytes.
	mustWrite(t, filepath.Join(dst, "late.md"), "theirs", 0o644)
	if err := worktreecmd.CopyRegularForTesting(root, src, "late.md", 0o644); !errors.Is(err, os.ErrExist) {
		t.Errorf("want ErrExist, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "late.md")); string(data) != "theirs" {
		t.Errorf("existing file replaced: %q", data)
	}
}

func TestWorktreeInclude_LiteralExclude(t *testing.T) {
	for in, want := range map[string]string{
		"a[1]*.md":  `/a\[1\]\*.md`,
		"local.md ": `/local.md\ `,
		"#x?":       `/#x\?`,
		`b\c`:       `/b\\c`,
	} {
		if got := worktreecmd.LiteralExcludeForTesting(in); got != want {
			t.Errorf("literalExclude(%q) = %q, want %q", in, got, want)
		}
	}

	// End to end: the main's ignore rules are uncommitted, so in the worktree only
	// info/exclude hides the copies - and only those exact paths.
	repo, wt := adoptRepo(t, "ext-3")
	mustWrite(t, filepath.Join(repo, ".gitignore"), "a*.md\nlocal.md\\ \n", 0o644)
	mustWrite(t, filepath.Join(repo, ".worktreeinclude"), "a*.md\nlocal.md\\ \n", 0o644)
	mustWrite(t, filepath.Join(repo, "a[1]*.md"), "glob", 0o644)
	mustWrite(t, filepath.Join(repo, "local.md "), "space", 0o644)
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("adopt: %v", err)
	}
	if !includeStep(res.Steps, worktreecmd.StepOK, "copied 2 file(s)") {
		t.Fatalf("want both copied: %+v", res.Steps)
	}
	if p := porcelain(t, wt); p != "" {
		t.Errorf("the copies must be excluded:\n%s", p)
	}
	mustWrite(t, filepath.Join(wt, "a1x.md"), "u", 0o644)
	mustWrite(t, filepath.Join(wt, "local.md"), "u", 0o644)
	p := porcelain(t, wt)
	if !strings.Contains(p, "a1x.md") || !strings.Contains(p, "?? local.md") {
		t.Errorf("the exclude entries must be literal - unrelated files stay visible:\n%s", p)
	}
}

func TestWorktreeInclude_ExcludeRepair(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-4")
	mustWrite(t, filepath.Join(repo, ".gitignore"), "local.md\n", 0o644)
	mustWrite(t, filepath.Join(repo, ".worktreeinclude"), "local.md\n", 0o644)
	mustWrite(t, filepath.Join(repo, "local.md"), "l", 0o644)

	restore := worktreecmd.SetWriteExcludesForTesting(func(context.Context, string, []string) error {
		return errors.New("disk full")
	})
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	restore()
	if err != nil || !res.OK {
		t.Fatalf("adopt: %v", err)
	}
	if !exists(filepath.Join(wt, "local.md")) || !includeStep(res.Steps, worktreecmd.StepWarn, "could not update info/exclude") {
		t.Fatalf("want the copy and an exclude warn: %+v", res.Steps)
	}

	res, err = worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("re-adopt: %v", err)
	}
	if s := includeSteps(res.Steps); len(s) != 0 {
		t.Errorf("re-adopt copies nothing and warns nothing: %+v", s)
	}
	if p := porcelain(t, wt); p != "" {
		t.Errorf("the re-adopt must write the owed exclude:\n%s", p)
	}
}

// infoExclude returns the shared info/exclude content of repo.
func infoExclude(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		t.Fatal(err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(repo, p)
	}
	data, _ := os.ReadFile(p)
	return string(data)
}

// An escaped trailing-space entry is written once, not on every run (fix 3).
func TestWorktreeInclude_TrailingSpaceExcludeWrittenOnce(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-7")
	mustWrite(t, filepath.Join(repo, ".gitignore"), "local.md\\ \n", 0o644)
	mustWrite(t, filepath.Join(repo, ".worktreeinclude"), "local.md\\ \n", 0o644)
	mustWrite(t, filepath.Join(repo, "local.md "), "space", 0o644)
	for i := 0; i < 2; i++ {
		if res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true}); err != nil || !res.OK {
			t.Fatalf("adopt %d: %v", i, err)
		}
	}
	if n := strings.Count(infoExclude(t, repo), "/local.md\\ \n"); n != 1 {
		t.Errorf("want one exclude line, got %d:\n%s", n, infoExclude(t, repo))
	}
}

// A failing ignore check warns and copies nothing (fix 4).
func TestWorktreeInclude_IgnoreCheckFailureWarns(t *testing.T) {
	repo, wt := adoptRepo(t, "ext-8")
	mustWrite(t, filepath.Join(repo, ".gitignore"), "local.md\n", 0o644)
	mustWrite(t, filepath.Join(repo, ".worktreeinclude"), "local.md\n", 0o644)
	mustWrite(t, filepath.Join(repo, "local.md"), "l", 0o644)
	restore := worktreecmd.SetCheckIgnoredForTesting(func(context.Context, string, []string) (map[string]bool, error) {
		return nil, errors.New("exit status 128")
	})
	defer restore()
	res, err := worktreecmd.Adopt(context.Background(), wt, worktreecmd.AdoptOptions{LinksOnly: true})
	if err != nil || !res.OK {
		t.Fatalf("adopt: %v", err)
	}
	if !includeStep(res.Steps, worktreecmd.StepWarn, "worktreeinclude: could not check ignore rules: exit status 128") {
		t.Errorf("want the ignore-check warn: %+v", res.Steps)
	}
	if exists(filepath.Join(wt, "local.md")) {
		t.Error("nothing is copied when the ignore check fails")
	}
}
