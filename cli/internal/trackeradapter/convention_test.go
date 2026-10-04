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
	if undeclared.Branch != DefaultBranch || undeclared.WorktreeBranch != DefaultWorktreeBranch {
		t.Errorf("an undeclared parse still renders: %+v", undeclared)
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

// TestLoadConvention_TemplatesWithoutID pins lets-puvic: a board's branch: names
// new branches even when nothing declares id:, parsing stays off, and the caller
// is told why.
func TestLoadConvention_TemplatesWithoutID(t *testing.T) {
	main := t.TempDir()
	writeFile(t, adapterFile(main, "planfix-mcp"), "# adapter\n\n## Capabilities\n\nnothing here.\n")
	writeFile(t, boardFile(main, "planfix-mcp"), "# board\n\n## Worktree\n\nbranch: `feature/pwa-{id}`.\n")

	c, d := LoadConventionDiagnosed(main, "planfix-mcp", "")
	if c.Declared || c.ID != nil {
		t.Fatalf("no id: -> must stay undeclared: %+v", c)
	}
	if c.Branch != "feature/pwa-{id}" || c.Source["branch"] != SourceBoard {
		t.Fatalf("board branch: must be honoured: %+v", c)
	}
	if c.WorktreeBranch != DefaultWorktreeBranch || c.Source["worktree-branch"] != SourceDefault {
		t.Errorf("undeclared worktree-branch: must stay default: %+v", c)
	}
	if want := []string{ReasonConventionUndeclared, ReasonTemplatesWithoutID}; !slices.Equal(d.Reasons, want) {
		t.Errorf("reasons = %v, want %v", d.Reasons, want)
	}
	if len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], ReasonTemplatesWithoutID+": ") ||
		!strings.Contains(d.Warnings[0], ".claude/rules/tracker-planfix-mcp.board.md") || !strings.Contains(d.Warnings[0], "id:") {
		t.Errorf("warnings = %q", d.Warnings)
	}
	if got, err := c.Render(c.Branch, "49514", "x"); err != nil || got != "feature/pwa-49514" {
		t.Errorf("render = %q, %v", got, err)
	}
	if _, _, ok := c.ParseBranch("feature/pwa-49514", CreatedAndAccepted); ok {
		t.Error("parsing must stay off without id:")
	}

	// accept: without id: is ignored and named.
	writeFile(t, boardFile(main, "planfix-mcp"), "# board\n\n## Worktree\n\nbranch: `feature/pwa-{id}`.\naccept: `{id}`.\n")
	c, d = LoadConventionDiagnosed(main, "planfix-mcp", "")
	if c.Accept != nil || !slices.Contains(d.Reasons, ReasonKeysIgnoredNoID) {
		t.Errorf("accept: without id: must be ignored and named: %+v %v", c, d.Reasons)
	}

	// With id: the board applies, no no-id reason.
	writeFile(t, boardFile(main, "planfix-mcp"), "# board\n\n## Worktree\n\nid: `[0-9]+`.\nbranch: `feature/pwa-{id}`.\n")
	c, d = LoadConventionDiagnosed(main, "planfix-mcp", "")
	if !c.Declared || c.Branch != "feature/pwa-{id}" || c.Source["branch"] != SourceBoard || len(d.Warnings) != 0 {
		t.Fatalf("declared board: %+v %v %q", c, d.Reasons, d.Warnings)
	}
	if slices.Contains(d.Reasons, ReasonTemplatesWithoutID) || slices.Contains(d.Reasons, ReasonKeysIgnoredNoID) {
		t.Errorf("reasons %v must not name a no-id drop", d.Reasons)
	}
}

// TestLoadConvention_AdapterTemplatesWithoutID: an installed adapter's own branch:
// is honoured without id:, unless a plugin copy that declares id: replaces it -
// then the drop is named.
func TestLoadConvention_AdapterTemplatesWithoutID(t *testing.T) {
	cases := []struct {
		name, tracker string
		plugin        bool
		check         func(t *testing.T, c Convention, d Diagnosis)
	}{
		{"installed, no plugin", "planfix-mcp", false, func(t *testing.T, c Convention, d Diagnosis) {
			if c.Declared || c.Branch != "task/{id}" || c.Source["branch"] != SourceInstalled {
				t.Errorf("installed branch: must be honoured: %+v", c)
			}
			if !slices.Contains(d.Reasons, ReasonTemplatesWithoutID) {
				t.Errorf("reasons = %v", d.Reasons)
			}
			if len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], ReasonTemplatesWithoutID+": ") ||
				!strings.Contains(d.Warnings[0], ".claude/rules/tracker-planfix-mcp.md") {
				t.Errorf("warnings = %q", d.Warnings)
			}
		}},
		{"installed lags plugin", "beads", true, func(t *testing.T, c Convention, d Diagnosis) {
			if !c.Declared || c.Source["id"] != SourcePlugin || c.Branch != DefaultBranch {
				t.Errorf("plugin must be used: %+v", c)
			}
			if len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], ReasonAdapterLagsPlugin+": ") ||
				!strings.Contains(d.Warnings[0], ".claude/rules/tracker-beads.md") {
				t.Errorf("warnings = %q", d.Warnings)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			main, plugin := t.TempDir(), ""
			writeFile(t, adapterFile(main, tc.tracker), sectionWith("branch: `task/{id}`."))
			if tc.plugin {
				plugin = t.TempDir()
				writeFile(t, filepath.Join(plugin, "rules", "tracker-"+tc.tracker+".md"), sectionWith(beadsWorktree))
			}
			c, d := LoadConventionDiagnosed(main, tc.tracker, plugin)
			tc.check(t, c, d)
		})
	}
}

// TestLoadConventionDiagnosed_FailuresFallBack: every failed load renders the
// defaults (BranchName relies on it), and says why in one voice.
func TestLoadConventionDiagnosed_FailuresFallBack(t *testing.T) {
	reasons := []string{
		ReasonConventionUndeclared, ReasonConventionDeclarationInvalid, ReasonBoardLinksIgnored,
		ReasonTemplatesWithoutID, ReasonKeysIgnoredNoID, ReasonLinesUnread,
		ReasonTrackerNameInvalid, ReasonAdapterMissing, ReasonAdapterLagsPlugin,
	}
	cases := []struct {
		name, tracker, adapter, board string
		warnPrefix, invalidPrefix     string
	}{
		{"tracker path", "../x", "", "", ReasonTrackerNameInvalid + ": ", ""},
		{"tracker case", "Planfix-MCP", "", "", ReasonTrackerNameInvalid + ": ", ""},
		{"invalid board", "beads", sectionWith(beadsWorktree), "# board\n\n## Worktree\n\nbranch: `feature/{slug}`.\n", "", ".claude/rules/tracker-beads.board.md: "},
		{"invalid adapter", "beads", sectionWith("id: `^bad`."), "", "", ".claude/rules/tracker-beads.md: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			main := t.TempDir()
			if tc.adapter != "" {
				writeFile(t, adapterFile(main, tc.tracker), tc.adapter)
			}
			if tc.board != "" {
				writeFile(t, boardFile(main, tc.tracker), tc.board)
			}
			c, d := LoadConventionDiagnosed(main, tc.tracker, "")
			if c.Branch != DefaultBranch || c.WorktreeBranch != DefaultWorktreeBranch || c.Source["branch"] != SourceDefault || c.Declared {
				t.Errorf("a failed load must fall back to the defaults: %+v", c)
			}
			for _, w := range d.Warnings {
				if strings.Contains(w, "\n") || !slices.ContainsFunc(reasons, func(r string) bool { return strings.HasPrefix(w, r+": ") }) {
					t.Errorf("warning %q is not one kind-prefixed line", w)
				}
			}
			if tc.warnPrefix != "" {
				if len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], tc.warnPrefix) || d.Invalid != "" {
					t.Errorf("warnings = %q, invalid = %q", d.Warnings, d.Invalid)
				}
				return
			}
			if len(d.Warnings) != 0 {
				t.Errorf("an invalid declaration speaks only through Invalid: %q", d.Warnings)
			}
			if !strings.HasPrefix(d.Invalid, tc.invalidPrefix) || strings.Contains(d.Invalid, "convention declaration invalid") {
				t.Errorf("invalid = %q, want prefix %q without the sentinel text", d.Invalid, tc.invalidPrefix)
			}
			if tc.name == "invalid board" && !strings.Contains(d.Invalid, "exactly one {id}") {
				t.Errorf("invalid = %q must name the parser error", d.Invalid)
			}
			if !slices.Contains(d.Reasons, ReasonConventionDeclarationInvalid) {
				t.Errorf("reasons = %v", d.Reasons)
			}
		})
	}
}

// TestLoadConventionDiagnosed_AdapterMissing: a missing adapter is named only when
// no plugin copy stands in for it.
func TestLoadConventionDiagnosed_AdapterMissing(t *testing.T) {
	main, plugin := t.TempDir(), t.TempDir()
	_, d := LoadConventionDiagnosed(main, "beads", "")
	if want := []string{ReasonAdapterMissing, ReasonConventionUndeclared}; !slices.Equal(d.Reasons, want) {
		t.Errorf("reasons = %v, want %v", d.Reasons, want)
	}
	if len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], ReasonAdapterMissing+": ") ||
		!strings.Contains(d.Warnings[0], ".claude/rules/tracker-beads.md") {
		t.Errorf("warnings = %q", d.Warnings)
	}
	writeFile(t, filepath.Join(plugin, "rules", "tracker-beads.md"), sectionWith(beadsWorktree))
	if c, d := LoadConventionDiagnosed(main, "beads", plugin); !c.Declared || len(d.Warnings) != 0 {
		t.Errorf("plugin stands in: %+v %q", c, d.Warnings)
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("a\x1bb\nc"); got != "a?b?c" {
		t.Errorf("oneLine = %q", got)
	}
	got := oneLine(strings.Repeat("x", 300))
	if len(got) != 203 || !strings.HasSuffix(got, "...") {
		t.Errorf("oneLine(300 bytes) = %d bytes %q", len(got), got[len(got)-5:])
	}
}
