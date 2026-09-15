package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrcArgStripped: every command that follows detect-task's explicit-argument
// convention also names the `--orc` strip, and detect-task strips it BEFORE the
// id-shape test - otherwise `/lets:plan-workflow <id> --orc=<name>` becomes a
// free-text goal (no claim, no pipeline marker, no gate notifications).
func TestOrcArgStripped(t *testing.T) {
	for _, rel := range []string{"commands/start.md", "commands/plan.md", "commands/plan-workflow.md", "commands/execute.md"} {
		b, err := os.ReadFile(filepath.Join(pluginDir(t), rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "detect-task") {
			t.Fatalf("%s no longer references detect-task - update this test", rel)
		}
		if !strings.Contains(string(b), "`--orc` is stripped per") {
			t.Errorf("%s follows the explicit-argument convention but does not name the --orc strip", rel)
		}
	}
	raw, err := os.ReadFile(filepath.Join(pluginDir(t), "skills/detect-task/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	sec := sectionSpan(string(raw), "### Explicit task-id argument")
	strip := strings.Index(sec, "**`--orc` is stripped first.**")
	claim := strings.Index(sec, "treat that id as **authoritative**")
	if strip < 0 || claim < 0 || strip > claim {
		t.Errorf("detect-task must strip --orc before the id is used (strip=%d claim=%d)", strip, claim)
	}
	if !strings.Contains(sec, "lets worktree task-state set --create --orc") {
		t.Error("an already-claimed task must still record the binding through task-state")
	}
}
