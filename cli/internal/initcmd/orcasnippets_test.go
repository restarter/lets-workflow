package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const orcaCardGuard = `[ "{LETS_LAUNCHER}" = "orca" ] && `

// cardLinesUnguarded returns every `lets orca card` line that does not carry the
// addon switch in front of the call.
func cardLinesUnguarded(files map[string]string) []string {
	var bad []string
	for name, body := range files {
		for _, line := range strings.Split(body, "\n") {
			i := strings.Index(line, "lets orca card")
			if i < 0 {
				continue
			}
			if g := strings.Index(line, orcaCardGuard); g < 0 || g > i {
				bad = append(bad, name+": "+strings.TrimSpace(line))
			}
		}
	}
	return bad
}

// TestOrcaCardSnippetsGated: Orca is an opt-in addon, so every phase snippet in a
// command guards the call on LETS_LAUNCHER=orca before `lets` runs.
func TestOrcaCardSnippetsGated(t *testing.T) {
	matches, _ := filepath.Glob(filepath.Join(pluginDir(t), "commands", "*.md"))
	files := map[string]string{}
	calls := 0
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(p)] = string(b)
		calls += strings.Count(string(b), "lets orca card")
	}
	if calls < 5 {
		t.Fatalf("expected the phase snippets (start, done x2, end, execute, plan-workflow); found %d card calls", calls)
	}
	for _, v := range cardLinesUnguarded(files) {
		t.Errorf("unguarded Orca card call: %s", v)
	}
	files["done.md"] += "\nlets orca card --phase pr --json 2>/dev/null || true\n"
	if len(cardLinesUnguarded(files)) == 0 {
		t.Error("mutation: an unguarded card call must fail the test")
	}
}
