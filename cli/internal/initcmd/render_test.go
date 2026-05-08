package initcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderEnv_GoldenParity(t *testing.T) {
	tests := []struct {
		name   string
		prefs  Prefs
		golden string
	}{
		{"default", Prefs{Language: "English", MergeBranch: "main", PRFlow: "local"}, "golden_env_default.txt"},
		{"ukrainian", Prefs{Language: "Ukrainian", MergeBranch: "main", PRFlow: "local"}, "golden_env_ukrainian.txt"},
		{"github", Prefs{Language: "English", MergeBranch: "main", PRFlow: "github"}, "golden_env_github.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", tt.golden))
			if err != nil {
				t.Fatal(err)
			}
			got := renderEnv(tt.prefs)
			if string(got) != string(want) {
				t.Errorf("renderEnv mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}
