package rulesdrift_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/rulesdrift"
)

const (
	letsRel    = ".claude/rules/lets-rules.md"
	adapterRel = ".claude/rules/tracker-beads.md"
)

// writeRules writes a rules file under <root>/.claude/rules/; version "" -> no
// frontmatter version.
func writeRules(t *testing.T, root, name, version string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: x\n---\n"
	if version != "" {
		body = "---\nname: x\nversion: " + version + "\n---\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pluginRoot builds a plugin root whose rules/ holds lets-rules.md + tracker-beads.md at 0.11.2.
func pluginRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"lets-rules.md", "tracker-beads.md"} {
		body := "---\nname: x\nversion: 0.11.2\n---\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCheck_Table(t *testing.T) {
	plugin := pluginRoot(t)

	type setup struct {
		lets, adapter string // "-" -> file absent
	}
	cases := []struct {
		name    string
		files   setup
		scope   string
		tracker string
		global  bool
		noPlug  bool
		want    []rulesdrift.File
	}{
		{name: "all equal", files: setup{"0.11.2", "0.11.2"}, tracker: "beads"},
		{name: "adapter outdated", files: setup{"0.11.2", "0.11.1"}, tracker: "beads",
			want: []rulesdrift.File{{Path: adapterRel, State: drift.StateOutdated, Installed: "0.11.1", Plugin: "0.11.2"}}},
		{name: "adapter no version", files: setup{"0.11.2", ""}, tracker: "beads",
			want: []rulesdrift.File{{Path: adapterRel, State: drift.StateUnknown, Plugin: "0.11.2"}}},
		{name: "adapter missing", files: setup{"0.11.2", "-"}, tracker: "beads",
			want: []rulesdrift.File{{Path: adapterRel, State: drift.StateMissing, Plugin: "0.11.2"}}},
		{name: "adapter ahead", files: setup{"0.11.2", "0.12.0"}, tracker: "beads"},
		{name: "lets missing scope user", files: setup{"-", "0.11.2"}, scope: "user", tracker: "beads"},
		{name: "lets missing global present", files: setup{"-", "0.11.2"}, scope: "project", tracker: "beads", global: true},
		{name: "lets missing no global", files: setup{"-", "0.11.2"}, scope: "project", tracker: "beads",
			want: []rulesdrift.File{{Path: letsRel, State: drift.StateMissing, Plugin: "0.11.2"}}},
		{name: "tracker empty skips adapter", files: setup{"0.11.2", "-"}},
		{name: "tracker without plugin copy skipped", files: setup{"0.11.2", "-"}, tracker: "custom"},
		{name: "plugin root empty", files: setup{"0.1.0", "-"}, tracker: "beads", noPlug: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkout := t.TempDir()
			if tc.files.lets != "-" {
				writeRules(t, checkout, "lets-rules.md", tc.files.lets)
			}
			if tc.files.adapter != "-" {
				writeRules(t, checkout, "tracker-beads.md", tc.files.adapter)
			}
			home := t.TempDir()
			if tc.global {
				writeRules(t, home, "lets-rules.md", "0.11.2")
			}
			o := rulesdrift.Options{PluginRoot: plugin, CheckoutRoot: checkout, Scope: tc.scope, Tracker: tc.tracker, HomeDir: home}
			if tc.noPlug {
				o.PluginRoot = ""
			}
			got := rulesdrift.Check(o)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Check = %#v, want %#v", got, tc.want)
			}
		})
	}

	t.Run("worktree fills Main", func(t *testing.T) {
		main := t.TempDir()
		writeRules(t, main, "lets-rules.md", "0.11.2")
		writeRules(t, main, "tracker-beads.md", "0.11.2")
		wt := t.TempDir()
		writeRules(t, wt, "lets-rules.md", "0.11.2")
		writeRules(t, wt, "tracker-beads.md", "0.11.1")
		got := rulesdrift.Check(rulesdrift.Options{PluginRoot: plugin, CheckoutRoot: wt, MainRoot: main, Scope: "project", Tracker: "beads"})
		want := []rulesdrift.File{{Path: adapterRel, State: drift.StateOutdated, Installed: "0.11.1", Plugin: "0.11.2", Main: "0.11.2"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Check = %#v, want %#v", got, want)
		}
		if !got[0].IsAdapter() {
			t.Fatal("IsAdapter = false for the tracker adapter")
		}
	})

	t.Run("main checkout leaves Main empty", func(t *testing.T) {
		root := t.TempDir()
		writeRules(t, root, "lets-rules.md", "0.11.2")
		writeRules(t, root, "tracker-beads.md", "0.11.1")
		got := rulesdrift.Check(rulesdrift.Options{PluginRoot: plugin, CheckoutRoot: root, MainRoot: root, Tracker: "beads"})
		if len(got) != 1 || got[0].Main != "" {
			t.Fatalf("Check = %#v, want one row with empty Main", got)
		}
	})
}

func TestWorktreeMessage_LiveCase(t *testing.T) {
	files := []rulesdrift.File{{Path: adapterRel, State: drift.StateOutdated, Installed: "0.11.1", Plugin: "0.11.2", Main: "0.11.2"}}
	got := rulesdrift.WorktreeMessage(files, "main")
	want := "LETS rules in this worktree lag the plugin v0.11.2: `.claude/rules/tracker-beads.md` v0.11.1 - a worktree gets its branch's committed copy, and /lets:update does not write it." +
		" The main checkout already has v0.11.2: land `.claude/rules/` on `main` there (commit + push, or a PR), then rebase this branch on it; a file you keep out of git goes in `.worktreeinclude` (then `lets worktree adopt`)."
	if got != want {
		t.Fatalf("WorktreeMessage =\n%q\nwant\n%q", got, want)
	}
	if !strings.HasPrefix(got, "LETS rules in this worktree lag the plugin v0.11.2: `.claude/rules/tracker-beads.md` v0.11.1") {
		t.Fatal("prefix mismatch")
	}
	if !strings.Contains(got, "The main checkout already has v0.11.2: land `.claude/rules/` on `main` there") {
		t.Fatal("main-current remediation missing")
	}
	if rulesdrift.WorktreeMessage(nil, "main") != "" {
		t.Fatal("WorktreeMessage(nil) != \"\"")
	}
}

func TestWorktreeMessage_MainLags(t *testing.T) {
	files := []rulesdrift.File{{Path: adapterRel, State: drift.StateOutdated, Installed: "0.11.1", Plugin: "0.11.2", Main: "0.11.1"}}
	got := rulesdrift.WorktreeMessage(files, "main")
	if !strings.Contains(got, "In the main checkout: /lets:update") {
		t.Fatalf("WorktreeMessage = %q, want the main-lags remediation", got)
	}
}

func TestWorktreeMessage_Bound(t *testing.T) {
	files := []rulesdrift.File{
		{Path: letsRel, State: drift.StateMissing, Plugin: "0.11.2"},
		{Path: adapterRel, State: drift.StateUnknown, Plugin: "0.11.2", Main: "0.11.2"},
	}
	got := rulesdrift.WorktreeMessage(files, strings.Repeat("m", 200))
	if len(got) > 700 {
		t.Fatalf("len(WorktreeMessage) = %d, want <= 700: %q", len(got), got)
	}
}

func TestAdapterMessage(t *testing.T) {
	cases := []struct {
		f    rulesdrift.File
		want string
	}{
		{rulesdrift.File{Path: adapterRel, State: drift.StateMissing, Plugin: "0.11.2"},
			"Tracker adapter `.claude/rules/tracker-beads.md` not installed. Run `/lets:update`."},
		{rulesdrift.File{Path: adapterRel, State: drift.StateUnknown, Plugin: "0.11.2"},
			"Tracker adapter `.claude/rules/tracker-beads.md` version unknown. Run `/lets:update`, then commit it - worktrees and clones check out the committed copy."},
		{rulesdrift.File{Path: adapterRel, State: drift.StateOutdated, Installed: "0.11.1", Plugin: "0.11.2"},
			"Tracker adapter `.claude/rules/tracker-beads.md` outdated (installed v0.11.1 < plugin v0.11.2). Run `/lets:update`, then commit it - worktrees and clones check out the committed copy."},
	}
	for _, tc := range cases {
		if got := rulesdrift.AdapterMessage(tc.f); got != tc.want {
			t.Errorf("AdapterMessage(%s) =\n%q\nwant\n%q", tc.f.State, got, tc.want)
		}
	}
}
