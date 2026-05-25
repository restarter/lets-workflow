package cli_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSkillWorktreeArgs_ValidSubcommands enforces the markdown→cobra contract:
// every Skill(skill: "lets:worktree", args: "...") invocation in the plugin
// source must call a subcommand that exists in cobra (create/remove/list/info).
// Detects drift where a command renames a subcommand without updating callers.
func TestSkillWorktreeArgs_ValidSubcommands(t *testing.T) {
	valid := map[string]bool{"create": true, "remove": true, "list": true, "info": true}
	re := regexp.MustCompile(`Skill\(\s*skill:\s*"lets:worktree"\s*,\s*args:\s*"([^"]+)"`)
	// cli/internal/cli/ → repo root via three "..".
	root := filepath.Join("..", "..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "docs-local" || name == ".lets" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, _ := os.ReadFile(path)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			parts := strings.Fields(m[1])
			if len(parts) == 0 || !valid[parts[0]] {
				t.Errorf("%s: Skill(lets:worktree, args: %q) — %q not a valid subcommand", path, m[1], parts[0])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
