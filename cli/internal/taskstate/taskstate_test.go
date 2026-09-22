package taskstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const sid = "2f942be4-23e0-4b17-9eab-df0a4a7298f2"

func write(t *testing.T, letsDir, slug, content string) {
	t.Helper()
	p := Path(letsDir, slug)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, letsDir, slug string) string {
	t.Helper()
	data, err := os.ReadFile(Path(letsDir, slug))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMergeWrite_KeepsUnknownLinesAndOrder(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\nfuture: kept verbatim\nstart: abc1234\nsession: abc1234 old\n\n")
	st, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"session": "5b55fc9 " + sid, "orc": "MAIN-PWA"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "task: lets-x\nfuture: kept verbatim\nstart: abc1234\nsession: 5b55fc9 " + sid + "\norc: MAIN-PWA\n"
	if got := read(t, d, "b"); got != want {
		t.Errorf("file:\n%s\nwant:\n%s", got, want)
	}
	if st.Orc != "MAIN-PWA" || st.Task != "lets-x" || len(st.Other) != 1 {
		t.Errorf("state = %+v", st)
	}
	// an empty value deletes the key
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "", "task": ""}}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, d, "b"); strings.Contains(got, "orc:") || strings.Contains(got, "task:") || !strings.Contains(got, "future: kept verbatim") {
		t.Errorf("delete:\n%s", got)
	}
}

// A writer that decides on the current task must see what another writer recorded
// while it waited for the lock - not what it read before (codex review: A/start-B).
func TestMergeWrite_DeriveSeesStateWrittenWhileWaiting(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-a\nstart: aaaaaaa\n")
	old := acquire
	t.Cleanup(func() { acquire = old })
	acquire = func(letsDir, slug string, deadline time.Time) (func(), error) {
		unlock, err := lock(letsDir, slug, deadline)
		if err == nil { // the other writer held the lock first
			write(t, d, "b", "task: lets-b\nstart: bbbbbbb\n")
		}
		return unlock, err
	}
	errMismatch := errors.New("task mismatch")
	var seen State
	_, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"task": "lets-a"}, Derive: func(cur State, exists bool) (map[string]string, error) {
		seen = cur
		if exists && cur.Task != "lets-a" {
			return nil, errMismatch
		}
		return nil, nil
	}})
	if !errors.Is(err, errMismatch) || seen.Task != "lets-b" || seen.Start != "bbbbbbb" {
		t.Fatalf("derive must run on the state under the lock: err=%v seen=%+v", err, seen)
	}
	if got := read(t, d, "b"); got != "task: lets-b\nstart: bbbbbbb\n" {
		t.Errorf("a refused derive writes nothing:\n%s", got)
	}
	// derived keys are validated and merged over Set
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Derive: func(State, bool) (map[string]string, error) {
		return map[string]string{"start": "not a sha"}, nil
	}}); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("an invalid derived value must be refused: %v", err)
	}
	acquire = old
	st, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Derive: func(cur State, _ bool) (map[string]string, error) {
		return map[string]string{"start": "ccccccc"}, nil
	}})
	if err != nil || st.Start != "ccccccc" || st.Orc != "MAIN" || st.Task != "lets-b" {
		t.Errorf("merged: %+v %v", st, err)
	}
}

func TestMergeWrite_CreateFalseLeavesMissingFile(t *testing.T) {
	d := t.TempDir()
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"session": "5b55fc9 " + sid}}); !errors.Is(err, ErrFileAbsent) {
		t.Fatalf("err = %v, want ErrFileAbsent", err)
	}
	if _, err := os.Stat(Path(d, "b")); !os.IsNotExist(err) {
		t.Error("file must not be created")
	}
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Create: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, d, "b"); got != "orc: MAIN\n" {
		t.Errorf("created: %q", got)
	}
}

func TestMergeWrite_RefusesInvalidValues(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\n")
	for _, set := range []map[string]string{
		{"task": "-rf"},
		{"orc": "MAIN\nsession: evil"},
		{"start": "zzz"},
		{"origin": "x"},
		{"session": "abc1234 not-a-uuid"},
		{"bogus": "x"},
	} {
		if _, err := MergeWrite(d, "b", WriteOpts{Set: set}); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%v: err = %v, want ErrInvalidValue", set, err)
		}
	}
	if got := read(t, d, "b"); got != "task: lets-x\n" {
		t.Errorf("nothing may be written on a refused value: %q", got)
	}
	// SHA-256 object names are accepted
	if _, err := MergeWrite(d, "b", WriteOpts{Set: map[string]string{"start": strings.Repeat("a", 64)}}); err != nil {
		t.Errorf("64-hex start: %v", err)
	}
	if _, err := MergeWrite(d, "", WriteOpts{Set: map[string]string{"task": "x"}}); !errors.Is(err, ErrEmptySlug) {
		t.Errorf("empty slug: %v", err)
	}
}

func TestMergeWrite_ConcurrentWritersBothLand(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\n")
	var wg sync.WaitGroup
	for _, set := range []map[string]string{{"orc": "MAIN"}, {"start": "abc1234"}} {
		wg.Add(1)
		go func(set map[string]string) {
			defer wg.Done()
			if _, err := MergeWrite(d, "b", WriteOpts{Set: set}); err != nil {
				t.Error(err)
			}
		}(set)
	}
	wg.Wait()
	got := read(t, d, "b")
	if !strings.Contains(got, "orc: MAIN") || !strings.Contains(got, "start: abc1234") {
		t.Errorf("a concurrent write was lost:\n%s", got)
	}
}

func TestMergeWrite_LockBusyDeadline(t *testing.T) {
	d := t.TempDir()
	write(t, d, "b", "task: lets-x\n")
	unlock, err := lock(d, "b", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	start := time.Now()
	_, err = MergeWrite(d, "b", WriteOpts{Set: map[string]string{"orc": "MAIN"}, Deadline: time.Now().Add(100 * time.Millisecond)})
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("err = %v, want ErrLockBusy", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("deadline overrun")
	}
}

func TestRemove_TempsOnly(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a", "task: lets-a\n")
	write(t, d, "a.b", "task: lets-ab\n")            // another branch whose slug extends "a"
	write(t, d, "a.1234", "task: lets-a.1234\n")     // a sub-task branch: same shape as a bash mktemp temp, so never removed
	write(t, d, "a.12345.tmp", "task: lets-a-tmp\n") // a legal branch name, NOT a temp
	tmp := filepath.Join(d, "sessions", ".tasktmp-a.98765")
	if err := os.WriteFile(tmp, []byte("stranded go temp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extends := filepath.Join(d, "sessions", ".tasktmp-a.b.98765") // the temp of slug "a.b": not a's
	if err := os.WriteFile(extends, []byte("other temp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(d, "a", time.Time{}); err != nil {
		t.Fatal(err)
	}
	for slug, want := range map[string]bool{"a": false, "a.12345.tmp": true, "a.b": true, "a.1234": true} {
		if _, err := os.Stat(Path(d, slug)); (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", slug, err == nil, want)
		}
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("the stranded .tasktmp- temp must be cleaned")
	}
	if _, err := os.Stat(extends); err != nil {
		t.Error("another slug's temp must be left alone")
	}
}

func TestAtomicWrite_TempOutsideTaskNamespace(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a", "task: lets-a\n")
	if err := atomicWrite(Path(d, "a"), "task: lets-b\n"); err != nil {
		t.Fatal(err)
	}
	got, err := Slugs(d)
	if err != nil || !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("slugs after a write = %v %v", got, err)
	}
	if data, _ := os.ReadFile(Path(d, "a")); string(data) != "task: lets-b\n" {
		t.Errorf("content = %q", data)
	}
}

func TestSlugs_EveryTaskEntry(t *testing.T) {
	d := t.TempDir()
	for _, slug := range []string{"feature", "feature.12345.tmp", "feature.locked", "feature.tmp"} {
		write(t, d, slug, "task: lets-a\n")
	}
	_ = os.WriteFile(filepath.Join(d, "sessions", ".tasktmp-feature.1"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(d, "sessions", "2026-09-22-1000-lets-a-snapshot.md"), []byte("x"), 0o600)
	got, err := Slugs(d)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if want := []string{"feature", "feature.12345.tmp", "feature.locked", "feature.tmp"}; !reflect.DeepEqual(got, want) {
		t.Errorf("slugs = %v, want %v", got, want)
	}
	if got, err := Slugs(filepath.Join(d, "absent")); err != nil || len(got) != 0 {
		t.Errorf("a missing sessions dir is empty: %v %v", got, err)
	}
}

func TestSlug(t *testing.T) {
	if s, ok := Slug("feature/lets-abc-x"); !ok || s != "feature-lets-abc-x" {
		t.Errorf("Slug = %q %v", s, ok)
	}
	if _, ok := Slug(""); ok {
		t.Error("detached HEAD has no slug")
	}
}

func TestMergeWrite_RefreshLeavesNoTrace(t *testing.T) {
	root := t.TempDir()
	letsDir := filepath.Join(root, ".lets")
	if _, err := MergeWrite(letsDir, "main", WriteOpts{Set: map[string]string{"session": "0123456 0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"}}); !errors.Is(err, ErrFileAbsent) {
		t.Fatalf("err = %v, want ErrFileAbsent", err)
	}
	if _, err := os.Lstat(letsDir); !os.IsNotExist(err) {
		t.Errorf("a refresh of a missing file must not create %s (err=%v)", letsDir, err)
	}
}
