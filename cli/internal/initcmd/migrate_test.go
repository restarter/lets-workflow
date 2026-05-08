package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLegacyYaml(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Prefs
	}{
		{"empty", ``, Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
		{"all keys", "language: Ukrainian\nmerge-branch: develop\ngithub: true\n", Prefs{Language: "Ukrainian", MergeBranch: "develop", PRFlow: "github"}},
		{"github false", "github: false\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
		{"github bitbucket", "github: bitbucket\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "bitbucket"}},
		{"with inline comments", "language: English # default\nmerge-branch: main\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
		{"unsafe value rejected", "language: foo;rm -rf /\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
		{"CRLF line endings", "language: Ukrainian\r\nmerge-branch: develop\r\n", Prefs{Language: "Ukrainian", MergeBranch: "develop", PRFlow: "local"}},
		{"UTF-8 BOM prefix", "\xEF\xBB\xBFlanguage: English\nmerge-branch: main\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
		{"BOM + CRLF", "\xEF\xBB\xBFlanguage: Ukrainian\r\n", Prefs{Language: "Ukrainian", MergeBranch: "main", PRFlow: "local"}},
		{"trailing whitespace + comment", "github: true   # use github\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "github"}},
		{"single-quoted value", "language: 'English'\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
		{"double-quoted value", `language: "English"` + "\n", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLegacyYaml([]byte(tt.body))
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseLegacyYaml_RefusesBlockScalar(t *testing.T) {
	for _, body := range []string{
		"language: |\n  Multi\n  Line\n",
		"language: >\n  Folded\n  Line\n",
		"merge-branch: |\n  block\n",
		"github: >\n  folded\n",
	} {
		t.Run(body[:len(body)-1], func(t *testing.T) {
			_, err := parseLegacyYaml([]byte(body))
			if err == nil || !strings.Contains(err.Error(), "block scalar") {
				t.Errorf("expected block scalar error for %q, got %v", body, err)
			}
		})
	}
}

func TestMigrateStatuslineSh_DeletesShim(t *testing.T) {
	tmp := t.TempDir()
	letsDir := filepath.Join(tmp, ".lets")
	os.MkdirAll(letsDir, 0o755)
	shimPath := filepath.Join(letsDir, "statusline.sh")
	if err := os.WriteFile(shimPath, embeddedStatuslineShim, 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err := MigrateStatuslineSh(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "removed") {
		t.Errorf("msg = %q, want 'removed'", msg)
	}
	if _, err := os.Stat(shimPath); !os.IsNotExist(err) {
		t.Errorf("shim still present")
	}
}

func TestMigrateStatuslineSh_DeletesLegacyBash(t *testing.T) {
	// Closes B12 from the 2026-05-08 review: legacy bash branch of
	// MigrateStatuslineSh was untested. Without this guard, a future tightening
	// of detectStatuslineSh's size/marker heuristics could silently flip
	// legacy-bash files to StatuslineForeign, causing migration to no-op
	// while users stay on broken bash.
	tmp := t.TempDir()
	letsDir := filepath.Join(tmp, ".lets")
	os.MkdirAll(letsDir, 0o755)
	shimPath := filepath.Join(letsDir, "statusline.sh")
	// Reuses makeLegacyBash from state_test.go (same package).
	if err := os.WriteFile(shimPath, makeLegacyBash(), 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err := MigrateStatuslineSh(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "removed") {
		t.Errorf("msg = %q, want 'removed' for legacy bash", msg)
	}
	if _, err := os.Stat(shimPath); !os.IsNotExist(err) {
		t.Errorf("legacy bash shim still present (migration should have deleted)")
	}
}

func TestMigrateStatuslineSh_PreservesForeign(t *testing.T) {
	tmp := t.TempDir()
	letsDir := filepath.Join(tmp, ".lets")
	os.MkdirAll(letsDir, 0o755)
	shimPath := filepath.Join(letsDir, "statusline.sh")
	custom := []byte("#!/bin/bash\necho 'my custom thing'\n")
	if err := os.WriteFile(shimPath, custom, 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err := MigrateStatuslineSh(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "customized") {
		t.Errorf("msg = %q, want 'customized'", msg)
	}
	if _, err := os.Stat(shimPath); err != nil {
		t.Errorf("foreign script lost: %v", err)
	}
}

func TestMigrateYamlToEnv(t *testing.T) {
	tmp := t.TempDir()
	letsDir := filepath.Join(tmp, ".lets")
	os.MkdirAll(letsDir, 0o755)
	yamlPath := filepath.Join(letsDir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("language: Ukrainian\nmerge-branch: main\ngithub: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg, did, err := MigrateYamlToEnv(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !did {
		t.Errorf("expected migration, did=false")
	}
	if !strings.Contains(msg, ".env") {
		t.Errorf("msg = %q", msg)
	}
	envData, err := os.ReadFile(filepath.Join(letsDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envData), "LETS_LANGUAGE=Ukrainian") {
		t.Errorf(".env missing LETS_LANGUAGE=Ukrainian")
	}
	if _, err := os.Stat(yamlPath); !os.IsNotExist(err) {
		t.Errorf("yaml not renamed")
	}
	if _, err := os.Stat(yamlPath + ".deprecated"); err != nil {
		t.Errorf(".deprecated rename missing: %v", err)
	}
}

func TestMigrateYamlToEnv_SkipsWhenEnvExists(t *testing.T) {
	tmp := t.TempDir()
	letsDir := filepath.Join(tmp, ".lets")
	os.MkdirAll(letsDir, 0o755)
	envPath := filepath.Join(letsDir, ".env")
	if err := os.WriteFile(envPath, []byte("# already here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(letsDir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("language: Ukrainian\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, did, err := MigrateYamlToEnv(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if did {
		t.Errorf("did=true, want false (env existed)")
	}
	// yaml should still be present (not renamed)
	if _, err := os.Stat(yamlPath); err != nil {
		t.Errorf("yaml renamed unexpectedly: %v", err)
	}
}

func TestEnsureGitignore_AppendMissing(t *testing.T) {
	tmp := t.TempDir()
	gitignore := filepath.Join(tmp, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignore(tmp, []string{".lets/", ".beads/", ".worktrees/", "node_modules/"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(gitignore)
	for _, entry := range []string{"node_modules/", ".lets/", ".beads/", ".worktrees/"} {
		if !strings.Contains(string(data), entry) {
			t.Errorf("missing %q", entry)
		}
	}
	// Idempotent
	if err := EnsureGitignore(tmp, []string{".lets/"}); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(gitignore)
	if strings.Count(string(data2), ".lets/") != 1 {
		t.Errorf("idempotency broken, count = %d", strings.Count(string(data2), ".lets/"))
	}
}

func TestEnsureGitignore_CreatesFromScratch(t *testing.T) {
	// Closes S16 from the 2026-05-08 review: the absent-file path of
	// EnsureGitignore (most common case for `lets init` on a fresh repo!)
	// was uncovered. Coverage was 91.7%, missing exactly this branch.
	tmp := t.TempDir()
	gitignore := filepath.Join(tmp, ".gitignore")
	if _, err := os.Stat(gitignore); !os.IsNotExist(err) {
		t.Fatalf("test setup invariant violated: .gitignore should not exist yet")
	}

	if err := EnsureGitignore(tmp, []string{".lets/", ".beads/", ".worktrees/"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
	for _, want := range []string{".lets/", ".beads/", ".worktrees/"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in fresh .gitignore: %s", want, data)
		}
	}
}

func TestEnsureGitignore_SkipsCommentedEntries(t *testing.T) {
	// A commented-out entry should NOT block re-adding the real entry.
	tmp := t.TempDir()
	gitignore := filepath.Join(tmp, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# .lets/\nnode_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignore(tmp, []string{".lets/"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(gitignore)
	// .lets/ should appear as both `# .lets/` (comment) and `.lets/` (real entry)
	if strings.Count(string(data), ".lets/") != 2 {
		t.Errorf("expected commented + real entry (count=2), got %d:\n%s", strings.Count(string(data), ".lets/"), data)
	}
}
