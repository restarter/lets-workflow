package taskstate

import (
	"errors"
	"os"
	"path/filepath"
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
	write(t, d, "a.12345.tmp", "stranded go temp\n") // atomicWrite temp
	if err := Remove(d, "a", time.Time{}); err != nil {
		t.Fatal(err)
	}
	for slug, want := range map[string]bool{"a": false, "a.12345.tmp": false, "a.b": true, "a.1234": true} {
		if _, err := os.Stat(Path(d, slug)); (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", slug, err == nil, want)
		}
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
