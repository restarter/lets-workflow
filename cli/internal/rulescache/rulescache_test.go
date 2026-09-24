package rulescache

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

func rules(ver, body string) string {
	return "---\nname: lets-rules\nversion: " + ver + "\n---\n\n" + body + "\n"
}

// install creates <home>/.claude/plugins/cache/lets-workflow/lets/<ver> and returns its root.
func install(t *testing.T, home, ver, body string) string {
	t.Helper()
	root := filepath.Join(home, ".claude", "plugins", "cache", "lets-workflow", "lets", ver)
	mustWrite(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"lets","version":"`+ver+`"}`)
	mustWrite(t, filepath.Join(root, "rules", "lets-rules.md"), rules(ver, body))
	return root
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func create(t *testing.T, home, root string) {
	t.Helper()
	if r := Sync(Options{PluginRoot: root, HomeDir: home, InProject: true, ScopeUser: true}); r.Outcome != OutcomeCreated {
		t.Fatalf("setup create: %+v", r)
	}
}

func TestSync_CreatesOnlyInAUserScopeProject(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	if r := Sync(Options{PluginRoot: root, HomeDir: home}); r.Outcome != OutcomeNoop {
		t.Fatalf("outside a project: got %s, want noop (never create)", r.Outcome)
	}
	if _, err := os.Stat(DstPath(home)); !os.IsNotExist(err) {
		t.Fatal("file created outside a project")
	}
	if _, err := os.Stat(filepath.Join(home, ".lets")); !os.IsNotExist(err) {
		t.Fatal("~/.lets created for a machine that does not use user scope")
	}
	r := Sync(Options{PluginRoot: root, HomeDir: home, InProject: true, ScopeUser: true})
	if r.Outcome != OutcomeCreated || read(t, DstPath(home)) != rules("0.9.2", "A") || r.Notice() == "" {
		t.Fatalf("got %+v", r)
	}
}

func TestSync_NoopIsSilent(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	create(t, home, root)
	if r := Sync(Options{PluginRoot: root, HomeDir: home}); r.Outcome != OutcomeNoop || r.Notice() != "" {
		t.Fatalf("got %+v notice %q", r, r.Notice())
	}
}

func TestSync_RefreshesForwardAndKeepsNewer(t *testing.T) {
	home := t.TempDir()
	old := install(t, home, "0.9.1", "OLD")
	neu := install(t, home, "0.9.2", "NEW")
	create(t, home, old)
	r := Sync(Options{PluginRoot: neu, HomeDir: home})
	if r.Outcome != OutcomeWritten || r.From != "0.9.1" || r.To != "0.9.2" || r.Backup != "" {
		t.Fatalf("forward: got %+v", r)
	}
	r = Sync(Options{PluginRoot: old, HomeDir: home}) // an older session starts later
	if r.Outcome != OutcomeKeptNewer || read(t, DstPath(home)) != rules("0.9.2", "NEW") || !strings.Contains(r.Notice(), "Not an error") {
		t.Fatalf("downgrade must be refused: got %+v %q", r, r.Notice())
	}
}

func TestSync_HandEditIsCopiedToNumberedBackups(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	create(t, home, root)
	for i, want := range []string{"lets-rules.md.bak", "lets-rules.md.bak-2"} {
		edit := rules("0.9.2", "EDIT"+string(rune('1'+i)))
		mustWrite(t, DstPath(home), edit)
		r := Sync(Options{PluginRoot: root, HomeDir: home})
		if r.Outcome != OutcomeWritten || r.Backup != want {
			t.Fatalf("edit %d: got %+v, want backup %s", i+1, r, want)
		}
		if got := read(t, filepath.Join(filepath.Dir(DstPath(home)), want)); got != edit {
			t.Fatalf("backup %s lost the edit", want)
		}
	}
}

func TestSync_PreservedMtimeEditIsCaught(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "AAAA")
	create(t, home, root)
	fi, err := os.Stat(DstPath(home))
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, DstPath(home), rules("0.9.2", "BBBB")) // same size
	if err := os.Chtimes(DstPath(home), fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if r := Sync(Options{PluginRoot: root, HomeDir: home}); r.Outcome != OutcomeWritten || r.Backup != "lets-rules.md.bak" {
		t.Fatalf("a same-size, same-mtime edit must still be caught: %+v", r)
	}
}

func TestSync_FailedReplaceKeepsTheActiveFile(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	create(t, home, root)
	edit := rules("0.9.2", "EDIT")
	mustWrite(t, DstPath(home), edit)
	old := writeFile
	writeFile = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	t.Cleanup(func() { writeFile = old })
	r := Sync(Options{PluginRoot: root, HomeDir: home})
	if r.Outcome != OutcomeFailed || read(t, DstPath(home)) != edit {
		t.Fatalf("the active file must survive a failed replace: %+v", r)
	}
	if read(t, DstPath(home)+".bak") != edit || !strings.Contains(r.Notice(), "left unchanged") {
		t.Fatalf("backup copy / notice wrong: %+v %q", r, r.Notice())
	}
}

func TestSync_BusyLockIsNamedNotAwaited(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock is a no-op on windows")
	}
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	create(t, home, root)
	f, err := os.OpenFile(LockPath(home), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r := Sync(Options{PluginRoot: root, HomeDir: home})
	if r.Outcome != OutcomeSkipped || !strings.Contains(r.Notice(), "another session") || time.Since(start) > 3*time.Second {
		t.Fatalf("got %+v after %s", r, time.Since(start))
	}
}

func TestSync_LegacyPristineCopyNeedsNoBackup(t *testing.T) {
	home := t.TempDir()
	install(t, home, "0.9.1", "OLD")
	neu := install(t, home, "0.9.2", "NEW")
	mustWrite(t, DstPath(home), rules("0.9.1", "OLD")) // key-less copy an old `init --user` wrote
	if r := Sync(Options{PluginRoot: neu, HomeDir: home}); r.Outcome != OutcomeWritten || r.Backup != "" {
		t.Fatalf("got %+v", r)
	}
}

func TestSync_UntrustedRootNeverWrites(t *testing.T) {
	home := t.TempDir()
	install(t, home, "0.9.1", "OLD")
	mustWrite(t, DstPath(home), rules("0.9.1", "OLD"))
	dev := filepath.Join(t.TempDir(), "plugins", "lets") // a --plugin-dir checkout
	mustWrite(t, filepath.Join(dev, ".claude-plugin", "plugin.json"), `{"name":"lets","version":"0.9.2"}`)
	mustWrite(t, filepath.Join(dev, "rules", "lets-rules.md"), rules("0.9.2", "DEV"))
	r := Sync(Options{PluginRoot: dev, HomeDir: home})
	if r.Outcome != OutcomeSkipped || read(t, DstPath(home)) != rules("0.9.1", "OLD") || !strings.Contains(r.Notice(), "not an installed plugin") {
		t.Fatalf("got %+v %q", r, r.Notice())
	}
}

func TestSync_UntrustedRootIsSilentWhenNothingWouldChange(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, DstPath(home), rules("0.9.2", "SAME"))
	dev := filepath.Join(t.TempDir(), "lets")
	mustWrite(t, filepath.Join(dev, ".claude-plugin", "plugin.json"), `{"name":"lets","version":"0.9.2"}`)
	mustWrite(t, filepath.Join(dev, "rules", "lets-rules.md"), rules("0.9.2", "SAME"))
	if r := Sync(Options{PluginRoot: dev, HomeDir: home}); r.Outcome != OutcomeNoop || r.Notice() != "" {
		t.Fatalf("got %+v", r)
	}
	if _, ok := ReadKey(home); ok {
		t.Fatal("an untrusted root must not write the cache key")
	}
}

func TestSync_UnparseableKeyIsNoKey(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	create(t, home, root)
	mustWrite(t, KeyPath(home), "{not json")
	if r := Sync(Options{PluginRoot: root, HomeDir: home}); r.Outcome != OutcomeNoop {
		t.Fatalf("got %+v", r)
	}
	if _, ok := ReadKey(home); !ok {
		t.Fatal("key not re-recorded after the re-hash")
	}
}

func TestReadKey_RejectsAMalformedHash(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, KeyPath(home), `{"sha256":"x","version":"0.9.2"}`)
	if _, ok := ReadKey(home); ok {
		t.Fatal("a short hash must read as no key")
	}
}

func TestSync_FailureIsAResultNotABlock(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	rulesDir := filepath.Dir(DstPath(home))
	mustWrite(t, filepath.Join(rulesDir, "keep.md"), "x")
	if err := os.Chmod(rulesDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(rulesDir, 0o755) })
	r := Sync(Options{PluginRoot: root, HomeDir: home, InProject: true, ScopeUser: true})
	if r.Outcome != OutcomeFailed || !strings.Contains(r.Notice(), "FAILED") {
		t.Fatalf("got %+v", r)
	}
}

// A hand edit gains nothing by claiming a higher version: only content provably
// written by LETS carries a version, so the edit is saved and replaced.
func TestSync_EditClaimingNewerVersionIsReplaced(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	create(t, home, root)
	edit := rules("9.9.9", "EDIT")
	mustWrite(t, DstPath(home), edit)
	r := Sync(Options{PluginRoot: root, HomeDir: home})
	if r.Outcome != OutcomeWritten || r.Backup != "lets-rules.md.bak" || read(t, DstPath(home)) != rules("0.9.2", "A") {
		t.Fatalf("got %+v", r)
	}
	if strings.Contains(r.Notice(), "9.9.9") {
		t.Fatalf("a non-LETS copy's frontmatter must not be quoted as a version: %q", r.Notice())
	}
	if read(t, DstPath(home)+".bak") != edit {
		t.Fatal("the edit was not saved")
	}
}

// While the key names a newer plugin, an older session leaves even an edited
// file alone - the newer session owns the cache and restores it.
func TestSync_OlderSessionLeavesAnEditedNewerCache(t *testing.T) {
	home := t.TempDir()
	old := install(t, home, "0.9.1", "OLD")
	neu := install(t, home, "0.9.2", "NEW")
	create(t, home, neu)
	edit := rules("0.9.2", "EDIT")
	mustWrite(t, DstPath(home), edit)
	if r := Sync(Options{PluginRoot: old, HomeDir: home}); r.Outcome != OutcomeKeptNewer || read(t, DstPath(home)) != edit {
		t.Fatalf("got %+v", r)
	}
	if r := Sync(Options{PluginRoot: neu, HomeDir: home}); r.Outcome != OutcomeWritten || r.Backup == "" {
		t.Fatalf("the newer session must restore it: %+v", r)
	}
}

func TestPlan_WritesNothing(t *testing.T) {
	home := t.TempDir()
	install(t, home, "0.9.1", "OLD")
	neu := install(t, home, "0.9.2", "NEW")
	mustWrite(t, DstPath(home), rules("0.9.1", "OLD"))
	if p := Plan(Options{PluginRoot: neu, HomeDir: home}); p.Outcome != OutcomeWritten || p.From != "0.9.1" || p.To != "0.9.2" {
		t.Fatalf("got %+v", p)
	}
	if read(t, DstPath(home)) != rules("0.9.1", "OLD") {
		t.Fatal("Plan wrote the rules")
	}
	if _, ok := ReadKey(home); ok {
		t.Fatal("Plan wrote the key")
	}
	if _, err := os.Stat(DstPath(home) + ".bak"); !os.IsNotExist(err) {
		t.Fatal("Plan wrote a backup")
	}
}

func TestCheckInstalledRoot(t *testing.T) {
	home := t.TempDir()
	root := install(t, home, "0.9.2", "A")
	if reason := CheckInstalledRoot(root, home); reason != "" {
		t.Fatalf("installed root refused: %s", reason)
	}
	dev := filepath.Join(t.TempDir(), "lets")
	mustWrite(t, filepath.Join(dev, ".claude-plugin", "plugin.json"), `{"name":"lets","version":"0.9.2"}`)
	mustWrite(t, filepath.Join(dev, "rules", "lets-rules.md"), rules("0.9.2", "A"))
	if CheckInstalledRoot(dev, home) == "" {
		t.Fatal("a checkout outside the cache was accepted")
	}
}
