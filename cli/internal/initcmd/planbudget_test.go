package initcmd

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The plan-workflow budget lives twice: the BUDGET block in plan.workflow.js
// (the only code that can cap stages, since the Workflow runtime forbids
// imports) and the Step 2.5 panel in plan-workflow.md (the only place a user
// sees and changes it). A new agent() call without agentOpts() silently ignores
// the chosen model; a new fan-out without capStage() silently ignores the
// budget; a new return path without budgetOut() hides the drop log. None of
// these fail at runtime, so this file pins them structurally.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

func planWorkflowJS(t *testing.T) string {
	t.Helper()
	return readPlugin(t, filepath.Join("skills", "plan-workflow", "plan.workflow.js"))
}

// TestPlanBudget_EveryAgentCarriesModel: each agent( call wraps its options in
// agentOpts(, so a model override reaches every spawned agent. The -1 is the
// helper's own definition.
func TestPlanBudget_EveryAgentCarriesModel(t *testing.T) {
	js := planWorkflowJS(t)
	agents := strings.Count(js, "agent(")
	opts := strings.Count(js, "agentOpts(") - 1
	if agents == 0 {
		t.Fatal("plan.workflow.js: no agent( call found - renamed? update this guard")
	}
	if agents != opts {
		t.Errorf("plan.workflow.js: %d agent( calls but %d agentOpts( wrappers - a new agent ignores the model override", agents, opts)
	}
}

// TestPlanBudget_EveryFanOutIsCapped: each parallel(X.map fan-out iterates a
// list assigned by capStage(, so a budget reaches every panel and logs its drops.
func TestPlanBudget_EveryFanOutIsCapped(t *testing.T) {
	js := planWorkflowJS(t)
	fanouts := regexp.MustCompile(`parallel\((\w+)\.map`).FindAllStringSubmatch(js, -1)
	if len(fanouts) == 0 {
		t.Fatal("plan.workflow.js: no parallel(X.map fan-out found - renamed? update this guard")
	}
	for _, m := range fanouts {
		assigned := regexp.MustCompile(`(?:const|let)\s+` + regexp.QuoteMeta(m[1]) + `\s*=\s*capStage\(`)
		if !assigned.MatchString(js) {
			t.Errorf("plan.workflow.js: parallel(%s.map is not assigned by capStage( - the budget does not reach this panel", m[1])
		}
	}
}

// TestPlanBudget_EveryReturnReportsBudget: every counts object (the typed error
// returns and the success return) carries budget: budgetOut().
func TestPlanBudget_EveryReturnReportsBudget(t *testing.T) {
	js := planWorkflowJS(t)
	counts := strings.Count(js, "counts: {")
	budgets := strings.Count(js, "budget: budgetOut()")
	if counts == 0 {
		t.Fatal("plan.workflow.js: no `counts: {` found - renamed? update this guard")
	}
	if counts != budgets {
		t.Errorf("plan.workflow.js: %d counts objects but %d budget: budgetOut() - a return path hides the drop log", counts, budgets)
	}
}

// TestPlanBudget_PanelMatchesScript: the command has the panel, passes model and
// budget in its Step 3 args, and names exactly the stages the script caps.
func TestPlanBudget_PanelMatchesScript(t *testing.T) {
	js := planWorkflowJS(t)
	md := readPlugin(t, filepath.Join("commands", "plan-workflow.md"))

	panel := region(t, md, "plan-workflow.md Step 2.5", "## Step 2.5: Budget panel", "## Step 3:")
	if !strings.Contains(panel, "KEEP IN SYNC") || !strings.Contains(js, "KEEP IN SYNC with plan-workflow.md Step 2.5") {
		t.Error("plan-workflow: the BUDGET block and the Step 2.5 panel must point at each other with KEEP IN SYNC")
	}

	step3 := region(t, md, "plan-workflow.md Step 3", "## Step 3: Preflight + invoke", "## Step 4:")
	fence := region(t, step3, "plan-workflow.md Step 3 args fence", "Workflow({", "}})")
	for _, key := range []string{"model:", "budget:"} {
		if !strings.Contains(fence, key) {
			t.Errorf("plan-workflow.md Step 3: the Workflow args do not pass %q", key)
		}
	}

	minBlock := region(t, js, "plan.workflow.js MIN", "const MIN = {", "}")
	stages := regexp.MustCompile(`(\w+):`).FindAllStringSubmatch(minBlock, -1)
	if len(stages) == 0 {
		t.Fatal("plan.workflow.js: no stages in MIN - renamed? update this guard")
	}
	for _, s := range stages {
		if !strings.Contains(panel, "| `"+s[1]+"`") {
			t.Errorf("plan-workflow.md Step 2.5: stage %q from plan.workflow.js MIN has no panel row", s[1])
		}
	}
}
