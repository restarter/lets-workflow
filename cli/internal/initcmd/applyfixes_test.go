package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixContractProblems returns every way the plugin files break the --fix
// contract (lets-lo8ft); an empty result means none. It reads only its
// argument, so TestApplyFixes can feed it in-memory mutants.
func fixContractProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	skill := f["skill"]
	if !strings.Contains(skill, "user-invocable: false") {
		add("apply-fixes must be internal (user-invocable: false)")
	}
	if !strings.Contains(sectionSpan(skill, "## Authorization"), "NEVER commit") {
		add("apply-fixes ## Authorization must forbid a commit")
	}
	if !strings.Contains(sectionSpan(skill, "## Gate 1: verified"), "`UNCLEAR`") {
		add("apply-fixes must keep Gate 1 with the UNCLEAR stop")
	}
	gate2 := sectionSpan(skill, "## Gate 2: nothing to decide")
	for _, need := range []string{"never on a question mark or a keyword", "**In scope**", "**No open question**"} {
		if !strings.Contains(gate2, need) {
			add("apply-fixes Gate 2 must contain " + need)
		}
	}
	if !strings.Contains(sectionSpan(skill, "## Step 1: Decide - before any edit"), "apply NOTHING") {
		add("apply-fixes Step 1 must apply nothing when any finding needs a decision")
	}
	if !strings.Contains(skill, "NEVER text copied from a report") {
		add("apply-fixes must require the Remedy in this session's own words")
	}
	if !strings.Contains(sectionSpan(skill, "## Input"), "**Open items**") {
		add("apply-fixes must turn every open item of the source into a row that stops the run")
	}
	if !strings.Contains(f["handoff"], "--fix skipped - HEAD moved while the agent worked") {
		add("handoff.md must skip --fix when HEAD moved while the agent worked")
	}
	if !strings.Contains(sectionSpan(skill, "## Step 2: Apply"), "Never revert") {
		add("apply-fixes Step 2 must never revert an applied edit")
	}

	for key, calls := range map[string]int{"check": 1, "review": 2, "handoff": 1} {
		body := f[key]
		hint := ""
		if i := strings.Index(body, "argument-hint:"); i >= 0 {
			hint = body[i:]
			if j := strings.Index(hint, "\n"); j >= 0 {
				hint = hint[:j]
			}
		}
		if !strings.Contains(hint, "[--fix]") {
			add(key + ".md: argument-hint must offer [--fix]")
		}
		if n := strings.Count(body, `Skill(skill: "lets:apply-fixes"`); n != calls {
			add(fmt.Sprintf("%s.md must call apply-fixes exactly %d time(s), found %d", key, calls, n))
		}
	}
	for _, key := range []string{"check", "review"} {
		if !strings.Contains(f[key], "--fix edits files - --json has no side effects") {
			add(key + ".md must refuse --fix with --json")
		}
	}
	for _, need := range []string{"`--fix` applies the fixes of a report that comes back", "`--fix` applies a review's findings - an execution brief has none"} {
		if !strings.Contains(f["handoff"], need) {
			add("handoff.md must refuse: " + need)
		}
	}
	if !strings.Contains(f["check"], "## Step 3.5: Verify Findings (--fix only)") {
		add("check.md must verify inline before --fix applies (Step 3.5)")
	}
	if !strings.Contains(f["review"], "the top-K cap does not apply") {
		add("review.md must lift the verify cap under --fix")
	}
	if !strings.Contains(f["rules"], "`--fix` on `/lets:check` / `/lets:review` / `/lets:handoff`") {
		add("lets-rules.md must list --fix among the commands with their own code gate")
	}
	for _, need := range []string{"const verification = {", "refuted_findings: refutedFindings", "judged[i] || { f: toVerify[i], votes: [] }"} {
		if !strings.Contains(f["workflow"], need) {
			add("review.workflow.js must return the per-finding verify outcome for --workflow --fix: " + need)
		}
	}
	return p
}

// TestApplyFixes pins the --fix contract on the real plugin files, then proves
// each guard can fail: every mutant breaks one invariant in memory.
func TestApplyFixes(t *testing.T) {
	read := func(parts ...string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(append([]string{pluginDir(t)}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	files := map[string]string{
		"skill":    read("skills", "apply-fixes", "SKILL.md"),
		"check":    read("commands", "check.md"),
		"review":   read("commands", "review.md"),
		"handoff":  read("commands", "handoff.md"),
		"rules":    read("rules", "lets-rules.md"),
		"workflow": read("skills", "review-workflow", "review.workflow.js"),
	}
	for _, problem := range fixContractProblems(files) {
		t.Error(problem)
	}

	mutants := []struct{ name, key, old, repl, want string }{
		{"skill becomes user-facing", "skill", "user-invocable: false", "user-invocable: true", "internal"},
		{"skill may commit", "skill", "NEVER commit", "may commit", "forbid a commit"},
		{"review loses the plan fix", "review", `Skill(skill: "lets:apply-fixes", args: "source=review mode=plan`, `Skill(skill: "lets:other", args: "source=review mode=plan`, "exactly 2"},
		{"check applies with --json", "check", "--fix edits files - --json has no side effects", "x", "refuse --fix with --json"},
		{"handoff fixes an execution", "handoff", "an execution brief has none", "x", "must refuse"},
		{"rules drop the gate", "rules", "`--fix` on `/lets:check`", "`--fix` in `/lets:check`", "own code gate"},
		{"workflow returns counts only", "workflow", "refuted_findings: refutedFindings", "", "per-finding verify outcome"},
		{"open items stop nothing", "skill", "**Open items**", "Open items", "open item"},
		{"handoff ignores a moved HEAD", "handoff", "--fix skipped - HEAD moved while the agent worked", "x", "HEAD moved"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(files[m.key], m.old) {
				t.Fatalf("mutant cannot apply: %q is not in %s - update the mutant with the text it guards", m.old, m.key)
			}
			mutated := make(map[string]string, len(files))
			for k, v := range files {
				mutated[k] = v
			}
			mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, 1)
			for _, problem := range fixContractProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
