package trackeradapter

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sectionWith(lines string) string {
	return "# Tracker adapter: x\n\n## Degradation\n\nnone\n\n## Worktree\n\n" + lines + "\n\n## Notes\n\nlinks: `.not/this` (0600).\n"
}

func TestParseWorktree_Shapes(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		links []Link
	}{
		{"beads", "links: `.beads/.env` (0600).", []Link{{".beads/.env", 0o600}}},
		{"none", "links: nothing.", []Link{}},
		{"fake", "links: `.fake/token.json` (0600).", []Link{{".fake/token.json", 0o600}}},
		{"two", "links: `.a/x` (0600), `b/y.json` (0644).", []Link{{".a/x", 0o600}, {"b/y.json", 0o644}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseWorktree(sectionWith(c.body))
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(got) != len(c.links) {
				t.Fatalf("links = %v, want %v", got, c.links)
			}
			for i := range got {
				if got[i] != c.links[i] {
					t.Errorf("link %d = %v, want %v", i, got[i], c.links[i])
				}
			}
		})
	}
}

func TestParseWorktree_Rejects(t *testing.T) {
	for _, body := range []string{
		"links: `../x` (0600).",
		"links: `/etc/x` (0600).",
		"links: `.git/config` (0600).",
		"links: `.lets/x` (0600).",
		"links: `a/../../x` (0600).",
		"links: `.beads/.env` (0777).",
		"links: `.beads/.env` (0600)", // missing period: not a declaration line at all
		"links: .beads/.env.",         // not backticked
		"links: `.a` (0600).\nlinks: nothing.",
	} {
		_, err := ParseWorktree(sectionWith(body))
		if err == nil {
			t.Errorf("%q: want an error", body)
		}
	}
	if _, err := ParseWorktree("# no worktree section\n"); !errors.Is(err, ErrUndeclared) {
		t.Errorf("no section: err = %v, want ErrUndeclared", err)
	}
	if _, err := ParseWorktree(sectionWith("links: `.beads/.env` (0600)")); !errors.Is(err, ErrUndeclared) {
		t.Errorf("missing period must read as undeclared, got %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	main, plugin := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(plugin, "rules", "tracker-beads.md"), sectionWith("links: `.beads/.env` (0600)."))

	// installed and declared
	writeFile(t, adapterFile(main, "beads"), sectionWith("links: `.x/y` (0600)."))
	if w, reason := Load(main, "beads", plugin); reason != "" || w.Source != SourceInstalled || w.Links[0].Path != ".x/y" {
		t.Errorf("installed: %+v %q", w, reason)
	}

	// installed predates the declaration: plugin fallback
	writeFile(t, adapterFile(main, "beads"), "# old adapter\n\n## Degradation\n\nnone\n")
	if w, reason := Load(main, "beads", plugin); reason != ReasonAdapterLagsPlugin || w.Source != SourcePlugin || w.Links[0].Path != ".beads/.env" {
		t.Errorf("lags plugin: %+v %q", w, reason)
	}
	if w, reason := Load(main, "beads", ""); reason != ReasonStoreLinksUndeclared || len(w.Links) != 0 {
		t.Errorf("undeclared without plugin: %+v %q", w, reason)
	}

	// invalid declaration: no fallback
	writeFile(t, adapterFile(main, "beads"), sectionWith("links: `../escape` (0600)."))
	if _, reason := Load(main, "beads", plugin); reason != ReasonStoreLinksDeclarationInvalid {
		t.Errorf("invalid: reason %q", reason)
	}

	// missing adapter
	if _, reason := Load(main, "planfix", ""); reason != ReasonAdapterMissing {
		t.Errorf("missing: reason %q", reason)
	}

	// invalid tracker name never reaches the filesystem
	if _, reason := Load(main, "../../etc/x", plugin); reason != ReasonTrackerNameInvalid {
		t.Errorf("invalid name: reason %q", reason)
	}
}
