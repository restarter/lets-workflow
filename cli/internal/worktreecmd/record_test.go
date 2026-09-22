//go:build unix

package worktreecmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func recordRepo(t *testing.T) (repo, letsDir string) {
	t.Helper()
	repo, _ = filepath.EvalSymlinks(t.TempDir())
	gitOut(t, repo, "-c", "init.defaultBranch=main", "init")
	gitOut(t, repo, "config", "user.email", "t@example.com")
	gitOut(t, repo, "config", "user.name", "t")
	gitOut(t, repo, "commit", "--allow-empty", "-m", "one")
	letsDir = filepath.Join(repo, ".lets")
	_ = os.MkdirAll(filepath.Join(letsDir, "sessions"), 0o755)
	return repo, letsDir
}

func writeSnap(t *testing.T, letsDir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(letsDir, "sessions", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecordOf_States(t *testing.T) {
	ctx := context.Background()
	repo, letsDir := recordRepo(t)
	tip := gitOut(t, repo, "rev-parse", "HEAD")

	if r := recordOf(ctx, repo, letsDir, "lets-a1", tip); r.State != RecordMissing {
		t.Errorf("no snapshot: %+v", r)
	}
	writeSnap(t, letsDir, "2026-09-01-1000-lets-a1-snapshot.md", "## RESUME\n### Record\n- end\n")
	if r := recordOf(ctx, repo, letsDir, "lets-a1", tip); r.State != RecordStale || r.Detail != "unanchored" {
		t.Errorf("legacy snapshot: %+v", r)
	}
	writeSnap(t, letsDir, "2026-09-02-1000-lets-a1-snapshot.md", "### Record\n- end\n- head: "+tip+"\n")
	if r := recordOf(ctx, repo, letsDir, "lets-a1", tip); r.State != RecordPresent || r.Head != tip {
		t.Errorf("anchored at tip: %+v", r)
	}
	gitOut(t, repo, "commit", "--allow-empty", "-m", "two")
	tip2 := gitOut(t, repo, "rev-parse", "HEAD")
	if r := recordOf(ctx, repo, letsDir, "lets-a1", tip2); r.State != RecordStale || r.Detail != "" {
		t.Errorf("commits after the snapshot: %+v", r)
	}
	if r := recordOf(ctx, repo, letsDir, "lets-a1", ""); r.State != RecordStale || r.Detail != "tip_unknown" {
		t.Errorf("no tip: %+v", r)
	}
	if r := recordOf(ctx, repo, letsDir, "lets-a", tip); r.State != RecordMissing {
		t.Errorf("a shorter id must not match lets-a1's snapshots: %+v", r)
	}
}

func TestNewestFirst_StampThenVersion(t *testing.T) {
	files := []string{
		"/s/2026-09-02-1000-lets-a1-snapshot.md",
		"/s/2026-09-02-1000-lets-a1-snapshot-v9.md",
		"/s/2026-09-01-2359-lets-a1-snapshot-v2.md",
		"/s/2026-09-02-1000-lets-a1-snapshot-v10.md",
		"/s/2026-09-02-1000-lets-a1-snapshot-v2.md",
	}
	newestFirst(files)
	want := []string{
		"/s/2026-09-02-1000-lets-a1-snapshot-v10.md",
		"/s/2026-09-02-1000-lets-a1-snapshot-v9.md",
		"/s/2026-09-02-1000-lets-a1-snapshot-v2.md",
		"/s/2026-09-02-1000-lets-a1-snapshot.md",
		"/s/2026-09-01-2359-lets-a1-snapshot-v2.md",
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("order = %v", files)
		}
	}
}

func TestRecordOf_StaleNamesTheNewestSnapshot(t *testing.T) {
	ctx := context.Background()
	repo, letsDir := recordRepo(t)
	gitOut(t, repo, "commit", "--allow-empty", "-m", "two")
	writeSnap(t, letsDir, "2026-09-02-1000-lets-b1-snapshot.md", "- end\n")
	writeSnap(t, letsDir, "2026-09-02-1000-lets-b1-snapshot-v2.md", "- end\n")
	r := recordOf(ctx, repo, letsDir, "lets-b1", gitOut(t, repo, "rev-parse", "HEAD"))
	if r.State != RecordStale || filepath.Base(r.Snapshot) != "2026-09-02-1000-lets-b1-snapshot-v2.md" {
		t.Errorf("stale must name the newest snapshot: %+v", r)
	}
}
