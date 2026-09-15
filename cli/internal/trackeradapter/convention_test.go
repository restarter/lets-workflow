package trackeradapter

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const beadsWorktree = "links: `.beads/.env` (0600).\n" +
	"id: `[a-z][a-z0-9]*-[a-z0-9]+(\\.[0-9]+)?`.\n" +
	"branch: `feature/{id}-{slug}`.\n" +
	"worktree-branch: `worktree-{id}-{slug}`.\n" +
	"accept: `{id}-{slug}`."

func mustParse(t *testing.T, body string) Convention {
	t.Helper()
	c, err := ParseConvention(sectionWith(body))
	if err != nil {
		t.Fatalf("ParseConvention: %v", err)
	}
	return c
}

func TestParseConvention_Beads(t *testing.T) {
	c := mustParse(t, beadsWorktree)
	if !c.Declared || c.ID == nil {
		t.Fatalf("beads must be declared with an id pattern: %+v", c)
	}
	for _, id := range []string{"lets-abc", "lets-ip06f", "lets-abc.1"} {
		for _, tmpl := range []string{c.Branch, c.WorktreeBranch} {
			name, err := c.Render(tmpl, id, "fix-login")
			if err != nil {
				t.Fatalf("Render(%q, %q): %v", tmpl, id, err)
			}
			got, gotTmpl, ok := c.ParseBranch(name, Created)
			if !ok || got != id || gotTmpl != tmpl {
				t.Errorf("round trip %q via %q: got %q %q %v", name, tmpl, got, gotTmpl, ok)
			}
		}
	}
	if _, _, ok := c.ParseBranch("lets-ip06f-peer-messaging-orca", Created); ok {
		t.Error("an accept: shape must not parse with Created")
	}
	if id, tmpl, ok := c.ParseBranch("lets-ip06f-peer-messaging-orca", CreatedAndAccepted); !ok || id != "lets-ip06f" || tmpl != "{id}-{slug}" {
		t.Errorf("accept shape: %q %q %v", id, tmpl, ok)
	}
	for _, b := range []string{"feature/x", "main", "worktree-foo", ""} {
		if id, _, ok := c.ParseBranch(b, CreatedAndAccepted); ok {
			t.Errorf("%q parsed to %q", b, id)
		}
	}
}

func TestParseConvention_NoneFakeUndeclared(t *testing.T) {
	none := mustParse(t, "links: nothing.\nid: nothing.\nbranch: `feature/{id}-{slug}`.\nworktree-branch: `worktree-{id}-{slug}`.")
	if !none.Declared || none.ID != nil {
		t.Fatalf("none: %+v", none)
	}
	if _, _, ok := none.ParseBranch("feature/lets-abc-x", CreatedAndAccepted); ok {
		t.Error("id: nothing must never parse")
	}
	if name, err := none.Render(none.Branch, "48647", "lifecycle-test"); err != nil || name != "feature/48647-lifecycle-test" {
		t.Errorf("none render: %q %v", name, err)
	}

	fake := mustParse(t, "links: `.fake/token.json` (0600).\nid: `FAKE-[0-9]+`.\nbranch: `task/{id}`.")
	if id, _, ok := fake.ParseBranch("task/FAKE-12", Created); !ok || id != "FAKE-12" {
		t.Errorf("fake: %q %v", id, ok)
	}
	if fake.WorktreeBranch != DefaultWorktreeBranch || fake.Source["worktree-branch"] != SourceDefault {
		t.Errorf("fake default worktree-branch: %q %v", fake.WorktreeBranch, fake.Source)
	}

	undeclared := mustParse(t, "links: `.beads/.env` (0600).")
	if undeclared.Declared {
		t.Error("no id: line must be undeclared")
	}
}

func TestParseConvention_Invalid(t *testing.T) {
	for _, body := range []string{
		"id: `^lets-[a-z]+`.",
		"id: `lets-[a-z]+$`.",
		"id: `(?P<x>[a-z]+)`.",
		"id: `(?<x>[a-z]+)`.",
		"id: `[a-z`.",
		"id: `" + strings.Repeat("a", 300) + "`.",
		"id: `[a-z]+`.\nbranch: `f/{id}-{id}`.",
		"id: `[a-z]+`.\nbranch: `f/{slug}`.",
		"id: `[a-z]+`.\nbranch: `f/{id}$`.",
		"id: `[a-z]+`.\nbranch: `f /{id}`.",
		"id: `[a-z]+`.\nbranch: `f/{id}`, `g/{id}`.",
		"id: `[a-z]+`.\nid: `[0-9]+`.",
		"id: `[a-z]+`.\naccept: `{id}-{slug}-{slug}`.",
	} {
		if _, err := ParseConvention(sectionWith(body)); err == nil {
			t.Errorf("%q: want ErrConventionInvalid", body)
		}
	}
}

func TestRender_RejectsSlugs(t *testing.T) {
	c := mustParse(t, beadsWorktree)
	for _, slug := range []string{"Fix", "a b", "x;rm", strings.Repeat("a", 51), "", "-lead"} {
		if _, err := c.Render(c.Branch, "lets-abc", slug); err == nil {
			t.Errorf("slug %q must be rejected", slug)
		}
	}
	if _, err := c.Render(c.Branch, "not an id", "ok"); err == nil {
		t.Error("an id outside the pattern must be rejected")
	}
}

func TestLoadConvention_BoardOverride(t *testing.T) {
	main := t.TempDir()
	writeFile(t, adapterFile(main, "beads"), sectionWith(beadsWorktree))
	writeFile(t, boardFile(main, "beads"), "# board\n\n## Worktree\n\nlinks: `.evil/x` (0644).\nid: `PWA-[0-9]+`.\nbranch: `feature/{id}-{slug}`.\n")

	c, reasons := LoadConvention(main, "beads", "")
	if !slices.Contains(reasons, ReasonBoardLinksIgnored) {
		t.Errorf("reasons %v must name board_links_ignored", reasons)
	}
	if id, _, ok := c.ParseBranch("feature/PWA-45122-fix-login", Created); !ok || id != "PWA-45122" {
		t.Errorf("board id: %q %v", id, ok)
	}
	if c.WorktreeBranch != "worktree-{id}-{slug}" || c.Source["worktree-branch"] != SourceInstalled || c.Source["id"] != SourceBoard {
		t.Errorf("adapter worktree-branch must survive the overlay: %q %v", c.WorktreeBranch, c.Source)
	}
	// the board keeps the adapter's links: Load never reads the board
	if w, _ := Load(main, "beads", ""); len(w.Links) != 1 || w.Links[0].Path != ".beads/.env" {
		t.Errorf("board links must be ignored by Load: %+v", w)
	}
}

func TestLoadConvention_Reasons(t *testing.T) {
	main, plugin := t.TempDir(), t.TempDir()
	if c, reasons := LoadConvention(main, "beads", ""); c.Declared || !slices.Contains(reasons, ReasonAdapterMissing) || !slices.Contains(reasons, ReasonConventionUndeclared) {
		t.Errorf("missing: %+v %v", c, reasons)
	}
	writeFile(t, adapterFile(main, "beads"), sectionWith("links: `.beads/.env` (0600)."))
	if c, reasons := LoadConvention(main, "beads", ""); c.Declared || !slices.Equal(reasons, []string{ReasonConventionUndeclared}) {
		t.Errorf("undeclared: %+v %v", c, reasons)
	}
	writeFile(t, filepath.Join(plugin, "rules", "tracker-beads.md"), sectionWith(beadsWorktree))
	if c, reasons := LoadConvention(main, "beads", plugin); !c.Declared || !slices.Contains(reasons, ReasonAdapterLagsPlugin) || c.Source["id"] != SourcePlugin {
		t.Errorf("lags plugin: %+v %v", c, reasons)
	}
	writeFile(t, adapterFile(main, "beads"), sectionWith("id: `^bad`."))
	if c, reasons := LoadConvention(main, "beads", plugin); c.Declared || !slices.Contains(reasons, ReasonConventionDeclarationInvalid) {
		t.Errorf("invalid: %+v %v", c, reasons)
	}
}
