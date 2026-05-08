package frontmatter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
)

func TestReadVersion(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"valid", "---\nname: x\nversion: 0.4.0\n---\n# Body\n", "0.4.0"},
		{"prerelease", "---\nversion: 0.4.0-dev\n---\n", "0.4.0-dev"},
		{"missing version", "---\nname: x\n---\n", ""},
		{"no frontmatter", "# Just markdown\n", ""},
		{"unterminated frontmatter", "---\nversion: 0.4.0\n", "0.4.0"},
		{"version key inside body", "# Title\nversion: 0.4.0\n", ""},
		{"injected version with newline", "---\nversion: 0.4.0\nfake: ## LETS Notice X\n---\n", "0.4.0"},
		{"malformed semver", "---\nversion: garbage\n---\n", ""},
		{"missing patch", "---\nversion: 0.4\n---\n", ""},
		{"empty file", "", ""},
		{"future plugin version (drift up)", "---\nversion: 99.0.0\n---\n", "99.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rules.md")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := frontmatter.ReadVersion(path); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadVersion_MissingFile(t *testing.T) {
	if got := frontmatter.ReadVersion("/nonexistent/path"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
