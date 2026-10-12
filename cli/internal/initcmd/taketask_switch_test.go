package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The busy-writer gate left Go with members_live (exit 32, lets-gbfpd): take-task
// Step 2T now asks the harness (ListAgents) before it asks about uncommitted work
// or switches. These sentences are the gate; this lint keeps them, in order.
var takeTaskBusyWriter = []string{
	"Before anything else, run `ListAgents`.",
	"A busy agent of THIS session that writes in this tree stops the switch",
	"`isolation: null`",
	"role is `implementer`",
	"-> stop: \"`{name}` is still working in this tree",
	"nothing is switched",
	"The same agent idle -> one line",
	"is idle and keeps",
	"Isolated implementers and read-only roles never block",
}

// takeTaskSwitchProblems lists what is wrong with take-task's Step 2T (from
// `### Step 2T` to the next `### `) as the busy-writer gate.
func takeTaskSwitchProblems(md string) []string {
	start := strings.Index(md, "### Step 2T")
	if start < 0 {
		return []string{"no ### Step 2T section"}
	}
	sec := md[start+len("### Step 2T"):]
	if end := strings.Index(sec, "\n### "); end >= 0 {
		sec = sec[:end]
	}
	var probs []string
	for _, want := range takeTaskBusyWriter {
		if !strings.Contains(sec, want) {
			probs = append(probs, "missing "+want)
		}
	}
	order := []string{"`ListAgents`", `label: "Commit first"`, `label: "Park"`, "lets worktree switch --task"}
	prev := -1
	for _, mark := range order {
		i := strings.Index(sec, mark)
		if i < 0 {
			probs = append(probs, "missing "+mark)
			continue
		}
		if i < prev {
			probs = append(probs, mark+" comes before the step it must follow")
		}
		prev = i
	}
	if busy, idle := strings.Index(sec, "stops the switch"), strings.Index(sec, "is idle and keeps"); busy < 0 || idle < 0 || busy > idle {
		probs = append(probs, "the busy agent must stop the switch before the idle one is mentioned")
	}
	if strings.Contains(sec, "members_live") {
		probs = append(probs, "members_live is retired (exit 32) - the gate is ListAgents")
	}
	return probs
}

func TestTakeTask_ListAgentsBeforeSwitch(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "skills", "take-task", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	if p := takeTaskSwitchProblems(md); len(p) != 0 {
		t.Fatalf("take-task Step 2T: %v", p)
	}
	para := md[strings.Index(md, "**Busy writer first.**"):]
	para = para[:strings.Index(para, "\n\n")+2]
	park := strings.Index(md, "- **Park** ->")
	mutants := map[string]string{
		"drop the stop clause": strings.Replace(md, "-> stop: \"`{name}` is still working in this tree - wait for its report, or stop it (`/lets:team dismiss <name>`, or the run's Stop), then `/lets:start` again\"; nothing is switched.", "-> note it.", 1),
		"swap busy/idle":       strings.NewReplacer("A busy agent of THIS session", "An idle agent of THIS session", "The same agent idle", "The same agent busy").Replace(md),
		"isolated blocks":      strings.Replace(md, "Isolated implementers and read-only roles never block", "Isolated implementers block", 1),
		"after the Park question": func() string {
			m := strings.Replace(md, para, "", 1)
			at := strings.Index(m, "- **Park** ->")
			return m[:at] + para + m[at:]
		}(),
		"members_live back": strings.Replace(md, "`no_remote_base`, `target_is_merge_branch`", "`members_live` (a live implementer writes here), `no_remote_base`, `target_is_merge_branch`", 1),
	}
	if park < 0 || len(para) < 20 {
		t.Fatal("fixture: the paragraph or the Park bullet is missing")
	}
	for name, m := range mutants {
		if m == md {
			t.Errorf("mutant %q changed nothing", name)
			continue
		}
		if len(takeTaskSwitchProblems(m)) == 0 {
			t.Errorf("mutant %q passed the lint", name)
		}
	}
}
