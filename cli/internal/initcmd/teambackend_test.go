package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTeamBackend pins /lets:team v2: the Orca section never mixes in Agent Teams
// calls, the conflict guard is read before any worker starts, records name their
// backend, and the Orca flow reads the orchestration guide instead of pinning flags.
func TestTeamBackend(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	team := string(b)
	orca := sectionSpan(team, "\n## Run (backend orca)\n")
	if orca == "" {
		t.Fatal("## Run (backend orca) section missing")
	}
	for _, call := range []string{"TeamCreate(", "Agent(", "SendMessage("} {
		if strings.Contains(orca, call) {
			t.Errorf("the orca run section must not contain %s", call)
		}
	}
	if !strings.Contains(orca, "orca skills get orchestration") {
		t.Error("the orca run section must read the orchestration guide")
	}
	guard := strings.Index(team, ".lets/execution/team-*.json` whose `status`")
	if guard < 0 || !strings.Contains(team[guard:guard+300], "backend") {
		t.Error("Guard 4's conflict text (team records + backend) is missing")
	}
	if i := strings.Index(team, "worker-start"); i >= 0 && i < guard {
		t.Error("the conflict guard must come before any worker-start mention")
	}
	if !strings.Contains(team, `"backend": "agent-teams"`) {
		t.Error("the completion record example must carry backend")
	}
}
