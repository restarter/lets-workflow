package initcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The session-boundary skill's bash block is the ONE reader of a branch's session
// boundary; a session: line recorded by another session (the SessionStart guard now
// leaves a live foreign holder's line in place) must never read as this session's
// exact range. These tests EXECUTE the block, extracted from the committed skill,
// under bash as the skill runs it - reading it would miss a quoting slip.

// boundaryFence returns the skill's resolve block with {LETS_MERGE_BRANCH} filled.
func boundaryFence(t *testing.T) string {
	t.Helper()
	for _, f := range bashFences(readPlugin(t, filepath.Join("skills", "session-boundary", "SKILL.md"))) {
		if strings.Contains(f, "SESSION_TRUST=") {
			return strings.ReplaceAll(f, "{LETS_MERGE_BRANCH}", "main")
		}
	}
	t.Fatal("session-boundary: no bash fence prints SESSION_TRUST - block moved?")
	return ""
}

// runBoundary runs the block in a repo on feature/x whose task-state session: line
// names storedSid at the feature branch's first commit, as session envSid.
func runBoundary(t *testing.T, storedSid, envSid string) (stdout, stderr string) {
	t.Helper()
	repo := t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = repo, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "base")
	git("checkout", "-q", "-b", "feature/x")
	git("commit", "-q", "--allow-empty", "-m", "c1")
	start := git("rev-parse", "HEAD")
	git("commit", "-q", "--allow-empty", "-m", "c2")
	if err := os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".lets", "sessions", ".task-feature-x"),
		[]byte("task: lets-x\nsession: "+start+" "+storedSid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// `bash -c` is the point: the artifact under test IS the skill's shell block,
	// extracted from committed markdown in this repo. No external input reaches it.
	cmd := exec.Command("bash", "-c", boundaryFence(t))
	cmd.Dir = repo
	cmd.Env = append(env, "CLAUDE_CODE_SESSION_ID="+envSid)
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("boundary block: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

const (
	sbOwn     = "aaaaaaaa-0000-4000-8000-000000000001"
	sbForeign = "bbbbbbbb-0000-4000-8000-000000000002"
)

func TestSessionBoundary_ForeignSidNotExact(t *testing.T) {
	stdout, stderr := runBoundary(t, sbForeign, sbOwn)
	if !strings.Contains(stdout, "SESSION_TRUST=prior-session\n") {
		t.Errorf("a foreign sid must never be exact:\n%s", stdout)
	}
	if !strings.Contains(stdout, "SESSION_RANGE_DESC=approx:") {
		t.Errorf("DESC must carry the approx qualifier:\n%s", stdout)
	}
	if !strings.Contains(stderr, "may include work that is not this session's") || !strings.Contains(stderr, "/lets:start here rewrites it") {
		t.Errorf("the prior-session NOTE must say the range may be another session's:\n%s", stderr)
	}
}

func TestSessionBoundary_OwnSidExact(t *testing.T) {
	stdout, stderr := runBoundary(t, sbOwn, sbOwn)
	if !strings.Contains(stdout, "SESSION_TRUST=exact\n") || !strings.Contains(stdout, "SESSION_COMMITS=1\n") {
		t.Errorf("own sid: want exact, 1 commit:\n%s", stdout)
	}
	if strings.Contains(stderr, "NOTE:") {
		t.Errorf("an exact range needs no NOTE:\n%s", stderr)
	}
}
