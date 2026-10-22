package gitutil

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pushFromSecondClone advances the bare remote's main by one commit and returns it.
func pushFromSecondClone(t *testing.T, bare string) string {
	t.Helper()
	b := filepath.Join(t.TempDir(), "b")
	remoteGit(t, filepath.Dir(b), "clone", "-q", bare, b)
	remoteGit(t, b, "config", "user.email", "t@e")
	remoteGit(t, b, "config", "user.name", "t")
	remoteGit(t, b, "commit", "-q", "--allow-empty", "-m", "upstream")
	remoteGit(t, b, "push", "-q", "origin", "main")
	return remoteGit(t, b, "rev-parse", "HEAD")
}

func hasNote(br *BaseResolution, sub string) bool {
	return slices.ContainsFunc(br.Notes, func(n string) bool { return strings.Contains(n, sub) })
}

func TestResolveRemote_Order(t *testing.T) {
	ctx := context.Background()
	bare, clone := remoteSetup(t)
	want := func(name, by string) {
		t.Helper()
		r, refusal := ResolveRemote(ctx, clone, "main")
		if refusal != nil || r.Name != name || r.By != by {
			t.Fatalf("ResolveRemote = %+v, %v; want %s by %s", r, refusal, name, by)
		}
	}
	want("origin", RemoteByUpstream) // a clone records branch.main.remote=origin
	remoteGit(t, clone, "remote", "add", "fork", bare)
	want("origin", RemoteByUpstream)
	remoteGit(t, clone, "config", "branch.main.remote", ".")
	want("origin", RemoteByOrigin) // a local upstream counts as none
	remoteGit(t, clone, "config", "branch.main.remote", "gone")
	want("origin", RemoteByOrigin) // an upstream naming a missing remote falls through
	remoteGit(t, clone, "config", "branch.main.remote", "fork")
	want("fork", RemoteByUpstream)
	remoteGit(t, clone, "config", "--unset", "branch.main.remote")
	remoteGit(t, clone, "remote", "rename", "origin", "up")
	_, refusal := ResolveRemote(ctx, clone, "main")
	if refusal == nil || refusal.Kind != BaseRemoteAmbiguous || !slices.Equal(slices.Sorted(slices.Values(refusal.Remotes)), []string{"fork", "up"}) {
		t.Fatalf("two remotes, no origin, no upstream: refusal %+v, want remote_ambiguous {fork, up}", refusal)
	}
	remoteGit(t, clone, "remote", "remove", "fork")
	want("up", RemoteBySingle)
	remoteGit(t, clone, "remote", "remove", "up")
	want("", RemoteByNone)
}

func TestResolveBase_BehindAheadEqual(t *testing.T) {
	ctx := context.Background()
	bare, clone := remoteSetup(t)
	br, refusal := ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || br.Source != BaseFromRemote || br.Ref != "origin/main" || !br.Fetched || br.LocalAhead != 0 || br.LocalBehind != 0 || len(br.Notes) != 0 {
		t.Fatalf("equal: %+v, %v", br, refusal)
	}
	tip := pushFromSecondClone(t, bare)
	br, refusal = ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || br.Sha != tip || br.LocalBehind != 1 {
		t.Fatalf("behind: %+v, %v; want sha %s behind 1", br, refusal, tip)
	}
	remoteGit(t, clone, "reset", "-q", "--hard", "origin/main")
	remoteGit(t, clone, "commit", "-q", "--allow-empty", "-m", "local only")
	br, refusal = ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || br.Sha != tip || br.LocalAhead != 1 || !hasNote(br, "not in the base origin/main") {
		t.Fatalf("ahead: %+v, %v; want the remote sha and an ahead note", br, refusal)
	}
}

func TestResolveBase_NoRemote(t *testing.T) {
	ctx := context.Background()
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "remote", "remove", "origin")
	br, refusal := ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || br.Source != BaseFromLocal || br.Ref != "main" || br.Sha != remoteGit(t, clone, "rev-parse", "main") || !hasNote(br, "no remote configured") {
		t.Fatalf("no remote: %+v, %v", br, refusal)
	}
	if _, refusal := ResolveBase(ctx, clone, "develop", BaseOptions{}); refusal == nil || refusal.Kind != BaseRefMissing {
		t.Fatalf("no remote, no local develop: %+v, want base_ref_missing", refusal)
	}
}

func TestResolveBase_FetchFailure(t *testing.T) {
	ctx := context.Background()
	_, clone := remoteSetup(t)
	before := remoteGit(t, clone, "rev-parse", "origin/main")
	remoteGit(t, clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	br, refusal := ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || !br.Stale || br.Fetched || br.Sha != before || br.FetchAge == "" || !hasNote(br, "last fetched") {
		t.Fatalf("fetch fails, ref present: %+v, %v", br, refusal)
	}
	remoteGit(t, clone, "update-ref", "-d", "refs/remotes/origin/main")
	if _, refusal := ResolveBase(ctx, clone, "main", BaseOptions{}); refusal == nil || refusal.Kind != BaseNoRemoteBase || !strings.Contains(refusal.Message, "fetch failed") {
		t.Fatalf("fetch fails, ref absent: %+v, want no_remote_base naming the fetch failure", refusal)
	}
}

func TestResolveBase_RemoteWithoutMerge(t *testing.T) {
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "branch", "develop")
	if _, refusal := ResolveBase(context.Background(), clone, "develop", BaseOptions{}); refusal == nil || refusal.Kind != BaseNoRemoteBase {
		t.Fatalf("a remote without develop: %+v, want no_remote_base (never the local develop)", refusal)
	}
}

func TestResolveBase_RemoteOnlyMergeDetachedHead(t *testing.T) {
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "checkout", "-q", "--detach")
	remoteGit(t, clone, "branch", "-q", "-D", "main")
	br, refusal := ResolveBase(context.Background(), clone, "main", BaseOptions{})
	if refusal != nil || br.Ref != "origin/main" || br.RemoteBy != RemoteBySingle || br.LocalAhead != 0 || br.LocalBehind != 0 {
		t.Fatalf("remote-only main, detached HEAD: %+v, %v", br, refusal)
	}
}

func TestResolveBase_ShallowClone(t *testing.T) {
	bare, _ := remoteSetup(t)
	tip := pushFromSecondClone(t, bare)
	sh := filepath.Join(t.TempDir(), "sh")
	remoteGit(t, filepath.Dir(sh), "clone", "-q", "--depth", "1", "file://"+bare, sh)
	br, refusal := ResolveBase(context.Background(), sh, "main", BaseOptions{})
	if refusal != nil || br.Sha != tip {
		t.Fatalf("shallow: %+v, %v; want %s", br, refusal, tip)
	}
}

func TestResolveBase_NonOriginRemote(t *testing.T) {
	_, clone := remoteSetup(t)
	remoteGit(t, clone, "remote", "rename", "origin", "upstream")
	br, refusal := ResolveBase(context.Background(), clone, "main", BaseOptions{})
	if refusal != nil || br.Ref != "upstream/main" || br.Remote != "upstream" {
		t.Fatalf("remote named upstream: %+v, %v", br, refusal)
	}
}

// A narrow remote.origin.fetch that does not map main: the explicit destination
// still refreshes a stale origin/main and recreates an absent one (Codex round 1).
func TestResolveBase_NarrowRefspec(t *testing.T) {
	ctx := context.Background()
	bare, clone := remoteSetup(t)
	remoteGit(t, clone, "config", "remote.origin.fetch", "+refs/heads/other:refs/remotes/origin/other")
	stale := remoteGit(t, clone, "rev-parse", "origin/main")
	tip := pushFromSecondClone(t, bare)
	br, refusal := ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || br.Sha != tip || br.Sha == stale {
		t.Fatalf("narrow refspec, stale tracking ref: %+v, %v; want %s", br, refusal, tip)
	}
	remoteGit(t, clone, "update-ref", "-d", "refs/remotes/origin/main")
	br, refusal = ResolveBase(ctx, clone, "main", BaseOptions{})
	if refusal != nil || br.Sha != tip {
		t.Fatalf("narrow refspec, absent tracking ref: %+v, %v; want %s", br, refusal, tip)
	}
}
