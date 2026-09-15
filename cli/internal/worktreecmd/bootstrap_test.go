//go:build unix

package worktreecmd_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// layout returns a main checkout with .lets/ and a worktree dir. inside=true puts the
// worktree under <main>/.worktrees/ (relative links), else in a separate temp dir.
func layout(t *testing.T, inside bool) (mainRoot, wt string) {
	t.Helper()
	mainRoot = realTempDir(t)
	mustMkdir(t, filepath.Join(mainRoot, ".lets", "sessions"))
	if inside {
		wt = filepath.Join(mainRoot, ".worktrees", "w")
	} else {
		wt = filepath.Join(realTempDir(t), "w")
	}
	mustMkdir(t, wt)
	return mainRoot, wt
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string, mode os.FileMode) {
	t.Helper()
	mustMkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func resolvesTo(t *testing.T, link, want string) bool {
	t.Helper()
	got, err1 := filepath.EvalSymlinks(link)
	w, err2 := filepath.EvalSymlinks(want)
	return err1 == nil && err2 == nil && got == w
}

func errKind(err error) string {
	var e *worktreecmd.Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestLinkShared_LetsSymlinkStates(t *testing.T) {
	mainRoot, wt := layout(t, true)
	// (a) fresh: linked; again: left as is
	if _, _, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, true); err != nil {
		t.Fatal(err)
	}
	if !resolvesTo(t, filepath.Join(wt, ".lets"), filepath.Join(mainRoot, ".lets")) {
		t.Fatal(".lets not linked to main")
	}
	steps, _, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, true)
	if err != nil || !strings.Contains(steps[0].Message, "already linked") {
		t.Errorf("idempotent run: %v %+v", err, steps)
	}
	// a wrong symlink is replaced
	_ = os.Remove(filepath.Join(wt, ".lets"))
	other := realTempDir(t)
	if err := os.Symlink(other, filepath.Join(wt, ".lets")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, true); err != nil {
		t.Fatal(err)
	}
	if !resolvesTo(t, filepath.Join(wt, ".lets"), filepath.Join(mainRoot, ".lets")) {
		t.Error("wrong symlink not replaced")
	}
}

func TestLinkShared_CreateRemovesRealLets(t *testing.T) {
	mainRoot, wt := layout(t, true)
	mustWrite(t, filepath.Join(wt, ".lets", "sessions", "x.md"), "race", 0o644)
	steps, _, moved, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, false)
	if err != nil || moved != "" {
		t.Fatalf("err=%v moved=%q", err, moved)
	}
	if !resolvesTo(t, filepath.Join(wt, ".lets"), filepath.Join(mainRoot, ".lets")) || steps[0].Status != worktreecmd.StepWarn {
		t.Errorf("create mode must replace the real dir with a warning: %+v", steps)
	}
}

func TestLinkShared_AdoptMovesCacheOnlyAside(t *testing.T) {
	mainRoot, wt := layout(t, false)
	mustWrite(t, filepath.Join(wt, ".lets", "cache", "usage"), "u", 0o644)
	mustWrite(t, filepath.Join(wt, ".lets", "cache", "task-status"), "s", 0o644)
	_, _, moved, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, true)
	if err != nil || moved != filepath.Join(wt, ".lets.pre-adopt") {
		t.Fatalf("err=%v moved=%q", err, moved)
	}
	if data, _ := os.ReadFile(filepath.Join(moved, "cache", "usage")); string(data) != "u" {
		t.Error("moved-aside contents lost")
	}
	// a second cache-only .lets goes to -2
	_ = os.Remove(filepath.Join(wt, ".lets"))
	mustWrite(t, filepath.Join(wt, ".lets", "locks", "x.lock"), "", 0o644)
	if _, _, moved, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, true); err != nil || moved != filepath.Join(wt, ".lets.pre-adopt-2") {
		t.Errorf("second move: err=%v moved=%q", err, moved)
	}
	if !resolvesTo(t, filepath.Join(wt, ".lets"), filepath.Join(mainRoot, ".lets")) {
		t.Error(".lets not linked after the move")
	}
}

func TestLinkShared_AdoptRefusesRealData(t *testing.T) {
	mainRoot, wt := layout(t, false)
	mustWrite(t, filepath.Join(wt, ".lets", "sessions", "x.md"), "notes", 0o644)
	_, _, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, nil, true)
	if errKind(err) != "lets_dir_conflict" || worktreecmd.ExitCode(err) != worktreecmd.ExitLetsDirConflict {
		t.Fatalf("err = %v, want lets_dir_conflict (22)", err)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, ".lets", "sessions", "x.md")); string(data) != "notes" {
		t.Error("real .lets must be left untouched")
	}
}

func TestLinkShared_StoreLinks(t *testing.T) {
	env := trackeradapter.Link{Path: ".beads/.env", Mode: 0o644}

	t.Run("absent main source skips", func(t *testing.T) {
		mainRoot, wt := layout(t, true)
		_, links, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{env}, false)
		if err != nil || len(links) != 1 || links[0].Linked {
			t.Errorf("err=%v links=%+v", err, links)
		}
	})

	t.Run("symlinked main source is unsafe", func(t *testing.T) {
		mainRoot, wt := layout(t, true)
		secret := filepath.Join(realTempDir(t), "secret")
		mustWrite(t, secret, "x", 0o600)
		mustMkdir(t, filepath.Join(mainRoot, ".beads"))
		if err := os.Symlink(secret, filepath.Join(mainRoot, ".beads", ".env")); err != nil {
			t.Fatal(err)
		}
		steps, links, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{env}, false)
		if err != nil || links[0].Linked || !strings.Contains(steps[len(steps)-1].Message, "store_link_unsafe") {
			t.Errorf("err=%v links=%+v steps=%+v", err, links, steps)
		}
	})

	t.Run("tighten only, relative link, pre-existing parent mode kept", func(t *testing.T) {
		mainRoot, wt := layout(t, true)
		mustWrite(t, filepath.Join(mainRoot, ".beads", ".env"), "TOKEN=x", 0o600)
		if err := os.Mkdir(filepath.Join(wt, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, links, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{env}, false)
		if err != nil || !links[0].Linked {
			t.Fatalf("err=%v links=%+v", err, links)
		}
		if fi, _ := os.Stat(filepath.Join(mainRoot, ".beads", ".env")); fi.Mode().Perm() != 0o600 {
			t.Errorf("a 0600 file declared 0644 must stay 0600, got %04o", fi.Mode().Perm())
		}
		if fi, _ := os.Stat(filepath.Join(wt, ".beads")); fi.Mode().Perm() != 0o755 {
			t.Errorf("a pre-existing parent keeps its mode, got %04o", fi.Mode().Perm())
		}
		if target, _ := os.Readlink(filepath.Join(wt, ".beads", ".env")); filepath.IsAbs(target) {
			t.Errorf("a worktree under the main root gets a relative link, got %q", target)
		}
		// existing correct symlink is ok
		if _, links, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{env}, false); err != nil || !links[0].Linked {
			t.Errorf("re-run: err=%v links=%+v", err, links)
		}
	})

	t.Run("new parent gets 0700 and a wide file is tightened", func(t *testing.T) {
		mainRoot, wt := layout(t, false)
		mustWrite(t, filepath.Join(mainRoot, ".beads", ".env"), "TOKEN=x", 0o666)
		_, links, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{{Path: ".beads/.env", Mode: 0o600}}, true)
		if err != nil || !links[0].Linked {
			t.Fatalf("err=%v links=%+v", err, links)
		}
		if fi, _ := os.Stat(filepath.Join(mainRoot, ".beads", ".env")); fi.Mode().Perm() != 0o600 {
			t.Errorf("tightened to %04o, want 0600", fi.Mode().Perm())
		}
		if fi, _ := os.Stat(filepath.Join(wt, ".beads")); fi.Mode().Perm() != 0o700 {
			t.Errorf("a parent lets created must be 0700, got %04o", fi.Mode().Perm())
		}
		if target, _ := os.Readlink(filepath.Join(wt, ".beads", ".env")); !filepath.IsAbs(target) {
			t.Errorf("a worktree outside the main root gets an absolute link, got %q", target)
		}
	})

	t.Run("parent committed as a symlink does not receive the link", func(t *testing.T) {
		mainRoot, wt := layout(t, true)
		mustWrite(t, filepath.Join(mainRoot, ".beads", ".env"), "TOKEN=x", 0o600)
		outside := realTempDir(t)
		if err := os.Symlink(outside, filepath.Join(wt, ".beads")); err != nil {
			t.Fatal(err)
		}
		_, links, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{env}, false)
		if err != nil || links[0].Linked {
			t.Errorf("err=%v links=%+v", err, links)
		}
		if _, err := os.Lstat(filepath.Join(outside, ".env")); !os.IsNotExist(err) {
			t.Error("the link escaped the worktree through a symlinked parent")
		}
	})

	t.Run("foreign file conflicts", func(t *testing.T) {
		mainRoot, wt := layout(t, true)
		mustWrite(t, filepath.Join(mainRoot, ".beads", ".env"), "TOKEN=x", 0o600)
		mustWrite(t, filepath.Join(wt, ".beads", ".env"), "OTHER", 0o600)
		_, _, _, err := worktreecmd.LinkSharedForTesting(mainRoot, wt, []trackeradapter.Link{env}, false)
		if errKind(err) != "store_link_conflict" || worktreecmd.ExitCode(err) != worktreecmd.ExitStoreLinkFailed {
			t.Errorf("err = %v, want store_link_conflict (24)", err)
		}
	})
}

func TestCreateSymlink_RelativeInsideAbsoluteOutside(t *testing.T) {
	mainRoot, inside := layout(t, true)
	target := filepath.Join(mainRoot, ".lets")
	if err := worktreecmd.CreateSymlink(filepath.Join(inside, "l"), target, mainRoot); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(inside, "l")); filepath.IsAbs(got) {
		t.Errorf("inside: %q is absolute", got)
	}
	_, outside := layout(t, false)
	if err := worktreecmd.CreateSymlink(filepath.Join(outside, "l"), target, mainRoot); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(outside, "l")); got != target {
		t.Errorf("outside: %q, want absolute %q", got, target)
	}
	if err := worktreecmd.CreateSymlink(filepath.Join(outside, "m"), filepath.Join(outside, "nope"), mainRoot); errKind(err) != "symlink_source_missing" {
		t.Errorf("missing target: %v", err)
	}
}
