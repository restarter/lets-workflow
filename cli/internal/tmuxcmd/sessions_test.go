//go:build unix

package tmuxcmd

import (
	"context"
	"errors"
	"testing"
)

func TestListPanes_ParsesRows(t *testing.T) {
	orig := listPanesRaw
	defer func() { listPanesRaw = orig }()
	listPanesRaw = func(_ context.Context, _ string) ([]byte, error) {
		return []byte("work\t0\teditor\t/tmp/a\nwork\t1\tshell\t/tmp/b\nbroken-row\n"), nil
	}
	panes, err := listPanes(context.Background(), "tmux")
	if err != nil {
		t.Fatalf("listPanes: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("got %d panes, want 2 (malformed row must be skipped)", len(panes))
	}
	if got := panes[1].Target(); got != "work:1" {
		t.Errorf("Target() = %q, want %q", got, "work:1")
	}
}

func TestListPanes_NoServer(t *testing.T) {
	orig := listPanesRaw
	defer func() { listPanesRaw = orig }()
	listPanesRaw = func(_ context.Context, _ string) ([]byte, error) {
		return nil, errors.New("no server running on /tmp/tmux-501/default")
	}
	if _, err := listPanes(context.Background(), "tmux"); err == nil {
		t.Fatal("want error when no tmux server is running")
	}
}

func TestFindByPath(t *testing.T) {
	panes := []paneEntry{{Session: "s", Window: "0", Title: "t", Path: "/tmp"}}
	if findByPath(panes, "/tmp") == nil {
		t.Error("findByPath(/tmp) = nil, want match")
	}
	if findByPath(panes, "/nonexistent-xyz") != nil {
		t.Error("findByPath(/nonexistent-xyz) != nil, want nil")
	}
	if findByPath(panes, "") != nil {
		t.Error("findByPath(\"\") != nil, want nil")
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"tmux-launcher": "tmux-launcher",
		"lets:0np5i":    "lets-0np5i",
		"a.b":           "a-b",
		"with space":    "with-space",
		"":              "lets",
		":::":           "lets",
		"tab\there":     "tab-here",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
