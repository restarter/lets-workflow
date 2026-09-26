package gitutil

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// remoteGit runs git in dir and returns its trimmed stdout; t.Fatal on failure.
func remoteGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// remoteSetup is a bare remote and a clone of it with one pushed commit on main.
func remoteSetup(t *testing.T) (bare, clone string) {
	t.Helper()
	root := t.TempDir()
	bare = filepath.Join(root, "remote.git")
	clone = filepath.Join(root, "a")
	remoteGit(t, root, "init", "-q", "--bare", "-b", "main", bare)
	remoteGit(t, root, "clone", "-q", bare, clone)
	remoteGit(t, clone, "config", "user.email", "t@e")
	remoteGit(t, clone, "config", "user.name", "t")
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "base")
	remoteGit(t, clone, "push", "-q", "origin", "main")
	return bare, clone
}

// recordArgs records every git argv RemoteContainsAny runs.
func recordArgs(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	orig := runGit
	runGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return orig(ctx, dir, args...)
	}
	t.Cleanup(func() { runGit = orig })
	return &calls
}

func ran(calls [][]string, verb string) bool {
	for _, c := range calls {
		if len(c) > 0 && c[0] == verb {
			return true
		}
	}
	return false
}

func TestRemoteContainsAny_PushedToOtherBranchAfterLastFetch(t *testing.T) {
	bare, clone := remoteSetup(t)
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local")
	sha := remoteGit(t, clone, "rev-parse", "HEAD")
	// pushed by URL to a differently named branch: this checkout's tracking refs never learn it
	remoteGit(t, clone, "push", "-q", bare, "HEAD:refs/heads/other")

	state, head, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0)
	if state != RemotePushed || head != "other" || err != nil {
		t.Errorf("state=%s head=%s err=%v, want pushed on other", state, head, err)
	}
}

func TestRemoteContainsAny_NoHeadContains(t *testing.T) {
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local only")
	sha := remoteGit(t, clone, "rev-parse", "HEAD")
	state, head, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0)
	if state != RemoteNotPushed || head != "" || err != nil {
		t.Errorf("state=%s head=%s err=%v, want not_pushed", state, head, err)
	}
}

func TestRemoteContainsAny_UnreachableUnverified(t *testing.T) {
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local only")
	sha := remoteGit(t, clone, "rev-parse", "HEAD")
	remoteGit(t, clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	state, _, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0)
	if state != RemoteUnverified || err == nil {
		t.Errorf("state=%s err=%v, want unverified with a reason", state, err)
	}
}

// fetchSetup: a second clone builds on this checkout's local commit and pushes it
// as a branch this checkout has no object for; it returns the local commit's sha.
func fetchSetup(t *testing.T) (clone, sha string) {
	t.Helper()
	bare, clone := remoteSetup(t)
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local")
	sha = remoteGit(t, clone, "rev-parse", "HEAD")
	b := filepath.Join(t.TempDir(), "b")
	remoteGit(t, clone, "clone", "-q", bare, b)
	remoteGit(t, b, "config", "user.email", "t@e")
	remoteGit(t, b, "config", "user.name", "t")
	remoteGit(t, b, "fetch", "-q", clone, "main")
	remoteGit(t, b, "checkout", "-q", "-b", "fromb", "FETCH_HEAD")
	remoteGit(t, b, "commit", "-q", "--allow-empty", "-m", "on top")
	remoteGit(t, b, "push", "-q", "origin", "fromb")
	return clone, sha
}

func TestRemoteContainsAny_FetchesByRefName(t *testing.T) {
	clone, sha := fetchSetup(t)
	calls := recordArgs(t)
	state, head, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0)
	if state != RemotePushed || head != "fromb" || err != nil {
		t.Fatalf("state=%s head=%s err=%v, want pushed on fromb", state, head, err)
	}
	fetched := false
	for _, c := range *calls {
		if len(c) == 0 || c[0] != "fetch" {
			continue
		}
		fetched = true
		joined := strings.Join(c, " ")
		if !strings.Contains(joined, "refs/heads/fromb") || !strings.Contains(joined, "--refmap=") || !strings.Contains(joined, "--no-tags") {
			t.Errorf("fetch argv = %v, want refs/heads/fromb by name, --refmap= and --no-tags", c)
		}
		for _, a := range c {
			if len(a) == 40 && strings.Trim(a, "0123456789abcdef") == "" {
				t.Errorf("fetch argv carries a bare sha %s", a)
			}
			if strings.HasPrefix(a, "--filter") {
				t.Errorf("fetch argv carries %s", a)
			}
		}
	}
	if !fetched {
		t.Error("the missing tip was never fetched")
	}
}

func TestRemoteContainsAny_TrackingRefsUnchanged(t *testing.T) {
	clone, sha := fetchSetup(t)
	before := remoteGit(t, clone, "for-each-ref", "refs/remotes")
	if _, _, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0); err != nil {
		t.Fatal(err)
	}
	if after := remoteGit(t, clone, "for-each-ref", "refs/remotes"); after != before {
		t.Errorf("tracking refs changed across a fetch round:\nbefore %s\nafter  %s", before, after)
	}
}

func TestRemoteContainsAny_LocalTrackingShortCircuit(t *testing.T) {
	_, clone := remoteSetup(t)
	sha := remoteGit(t, clone, "rev-parse", "HEAD") // pushed, and origin/main knows it
	calls := recordArgs(t)
	state, head, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0)
	if state != RemotePushed || head != "main" || err != nil {
		t.Fatalf("state=%s head=%s err=%v, want pushed on main", state, head, err)
	}
	if ran(*calls, "ls-remote") || ran(*calls, "fetch") {
		t.Errorf("a local tracking ref must answer without a network call: %v", *calls)
	}
}

func TestRemoteContainsAny_TooManyHeads(t *testing.T) {
	bare, clone := remoteSetup(t)
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local only")
	sha := remoteGit(t, clone, "rev-parse", "HEAD")
	tip := remoteGit(t, bare, "rev-parse", "main")
	var stdin strings.Builder
	for i := 0; i < maxRemoteHeads+1; i++ {
		fmt.Fprintf(&stdin, "create refs/heads/b%03d %s\n", i, tip)
	}
	cmd := exec.Command("git", "-C", bare, "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(stdin.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-ref: %v\n%s", err, out)
	}
	state, _, err := RemoteContainsAny(context.Background(), clone, "origin", sha, 0)
	if state != RemoteUnverified || !errors.Is(err, ErrTooManyHeads) {
		t.Errorf("state=%s err=%v, want unverified too_many_heads", state, err)
	}
}

func TestRemotesContainAny_EveryRemote(t *testing.T) {
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local")
	sha := remoteGit(t, clone, "rev-parse", "HEAD")
	fork := filepath.Join(t.TempDir(), "fork.git")
	remoteGit(t, clone, "init", "-q", "--bare", fork)
	remoteGit(t, clone, "remote", "add", "fork", fork)

	if state, _, _, err := RemotesContainAny(context.Background(), clone, sha, 0); state != RemoteNotPushed || err != nil {
		t.Fatalf("on no remote yet: state=%s err=%v", state, err)
	}
	remoteGit(t, clone, "push", "-q", fork, "HEAD:refs/heads/topic") // by URL: no tracking ref learns it
	state, remote, head, err := RemotesContainAny(context.Background(), clone, sha, 0)
	if state != RemotePushed || remote != "fork" || head != "topic" || err != nil {
		t.Errorf("state=%s remote=%s head=%s err=%v, want pushed on fork/topic", state, remote, head, err)
	}

	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local 2")
	sha2 := remoteGit(t, clone, "rev-parse", "HEAD")
	remoteGit(t, clone, "remote", "set-url", "fork", filepath.Join(t.TempDir(), "gone.git"))
	if state, _, _, err := RemotesContainAny(context.Background(), clone, sha2, 0); state != RemoteUnverified || err == nil {
		t.Errorf("one unreachable remote: state=%s err=%v, want unverified", state, err)
	}
}
