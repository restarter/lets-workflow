package initcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// lets-mpjhc: /lets:review and /lets:check on a standing team's skeptic. review's
// verify pass hands every finding to the team skeptic in one brief, under a member
// rule that never drops a BLOCKER on one vote and replaces the Asymmetric drop rule
// for member verdicts; the plan review never routes the architect lens (the plan's
// designer); check hands its whole pass to the skeptic member from Step 0 - local
// modes, --file and --plan, never PR mode or --json - and still spawns nothing; the
// skeptic gains a CHECK mode and verifies one or more findings, each on its own.
// teamModeReviewProblems reads only its argument, so the test feeds it mutants.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

const (
	memberRuleDowngrade = "`real=false` from the team skeptic drops a SUGGESTION and downgrades a BLOCKER to SUGGESTION - a BLOCKER is never dropped on one vote."
	memberRuleReplaces  = "This rule REPLACES the Asymmetric drop rule for member verdicts"
	memberRuleNoVerdict = "An OK member report with no verdict for an item -> that item `verify_failed`"
	architectOneShot    = "`architect: one-shot - the plan's designer does not review its own plan`"
	checkTeamHeading    = "### Team mode (team worktree)"
	checkRoute          = `Skill(skill: "lets:member-run", args: "op=route command=check roles=skeptic ask=no fallback=inline")`
	checkJSONInline     = "- `--solo` or `--json` -> inline"
	rolesCheckSentence  = "`/lets:check` hands its pass to the team's skeptic member - a message, not a spawn"
	skepticCheckMode    = "### CHECK (a team's quick check)"
	prContextBan        = "**The PR CONTEXT block NEVER goes to a skeptic**"
	memberReportRoute   = "the drop rule below runs over the fresh skeptics' reports; the member's report goes through the **Member-verdict rule** above"
	reviewJSONSolo      = "- In a team worktree `--json` passes `solo` to every route of the run"
)

func teamModeReviewProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }
	review, check := f["review"], f["check"]

	// review Step 6.6: the member-verdict rule.
	verify := sectionSpan(review, "## Step 6.6: Verify Findings (Adversarial)")
	if !strings.Contains(verify, memberRuleDowngrade) {
		add("review.md 6.6 member rule must downgrade a BLOCKER - never dropped on one vote")
	}
	if !strings.Contains(verify, memberRuleReplaces) || !strings.Contains(verify, memberRuleNoVerdict) {
		add("review.md 6.6 member rule must say it REPLACES the Asymmetric drop rule for member verdicts and map a missing item verdict to verify_failed")
	}
	// F9: the collect paragraph never sends the member's report into the Asymmetric drop rule.
	if after := between(verify, "**After the skeptics return**", "**Skeptic prompt template.**"); !strings.Contains(after, memberReportRoute) || strings.Contains(after, "the drop rule below runs over the reports read") {
		add("review.md 6.6's collect paragraph must send the member's report to the Member-verdict rule, never to the drop rule")
	}
	// F10: --json in a team worktree passes solo - a programmatic caller cannot answer or wait.
	if !strings.Contains(sectionSpan(review, "### JSON output flag"), reviewJSONSolo) {
		add("review.md's JSON output flag must pass solo to every route in a team worktree")
	}
	if !strings.Contains(verify, prContextBan) {
		add("review.md 6.6 must keep the ban: the PR CONTEXT block NEVER goes to a skeptic")
	}

	// review P4: the architect lens is never routed.
	p4 := sectionSpan(review, "### P4: Launch Plan Review Agents (Parallel)")
	roles := ""
	if at := strings.Index(p4, `op=route command=review roles=`); at >= 0 {
		roles = p4[at+len(`op=route command=review roles=`):]
		if end := strings.IndexAny(roles, " \""); end >= 0 {
			roles = roles[:end]
		}
	}
	if roles == "" || strings.Contains(roles, "architect") || !strings.Contains(p4, architectOneShot) {
		add("review.md P4 must route pragmatist and domain roles, never architect, and announce the architect one-shot")
	}

	// check: the team branch in Step 0, before Plan Mode; routes only there; spawns nothing.
	step0, branch, plan := strings.Index(check, "## Step 0: Determine Mode"), strings.Index(check, checkTeamHeading), strings.Index(check, "## Plan Mode (--plan)")
	team := sectionSpan(check, checkTeamHeading)
	if step0 < 0 || branch < step0 || plan < branch {
		add("check.md's Team mode branch must sit in Step 0, before ## Plan Mode (--plan)")
	}
	if n := strings.Count(check, "op=route"); n == 0 || n != strings.Count(team, "op=route") || !strings.Contains(team, checkRoute) {
		add("check.md must route only inside its Team mode branch, with ask=no and fallback=inline")
	}
	if !strings.Contains(team, "Runs for the local modes, `--file` and `--plan`") || !strings.Contains(team, "- PR mode stays inline:") {
		add("check.md's Team mode branch must cover --plan and keep PR mode inline")
	}
	if !strings.Contains(team, checkJSONInline) {
		add("check.md's Team mode branch must keep --json inline")
	}
	if strings.Contains(check, "Task(") || strings.Contains(check, "Agent(") {
		add("check.md must spawn no subagent - no Task( / Agent( call")
	}

	// roles.md: who dispatches, in a team.
	if !strings.Contains(sectionSpan(f["roles"], "## Who dispatches"), rolesCheckSentence) {
		add("roles.md ## Who dispatches must say /lets:check hands its pass to the team's skeptic member")
	}

	// skeptic.md: CHECK mode inside ## Modes before ## Report; batched VERIFY.
	sk := f["skeptic"]
	if modes := between(sk, "\n## Modes\n", "\n## Report\n"); !strings.Contains(modes, "\n"+skepticCheckMode+"\n") {
		add("skeptic.md must carry " + skepticCheckMode + " inside ## Modes, before ## Report")
	}
	if !strings.Contains(sectionSpan(sk, "### VERIFY (review)"), "Refute one or more findings against the code") {
		add("skeptic.md VERIFY (review) must take one or more findings")
	}
	if strings.Contains(sk, "ONE review finding") {
		add("skeptic.md's intro must not say ONE review finding - it verifies one or more")
	}
	return p
}

func teamModeReviewFiles(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"review":  readPlugin(t, filepath.Join("commands", "review.md")),
		"check":   readPlugin(t, filepath.Join("commands", "check.md")),
		"roles":   readPlugin(t, filepath.Join("protocol", "roles.md")),
		"skeptic": readPlugin(t, filepath.Join("agents", "skeptic.md")),
	}
}

func TestTeamModeReview(t *testing.T) {
	for _, problem := range teamModeReviewProblems(teamModeReviewFiles(t)) {
		t.Error(problem)
	}
}

// TestTeamModeReview_Mutants proves each guard of teamModeReviewProblems can fail.
func TestTeamModeReview_Mutants(t *testing.T) {
	files := teamModeReviewFiles(t)
	mutants := []struct{ name, key, old, repl, want string }{
		{"member rule drops a BLOCKER", "review", memberRuleDowngrade, "`real=false` from the team skeptic drops a SUGGESTION and drops a BLOCKER.", "never dropped on one vote"},
		{"member rule loses REPLACES", "review", memberRuleReplaces, "This rule sits beside the Asymmetric drop rule for member verdicts", "REPLACES"},
		{"missing verdict counted as clean", "review", memberRuleNoVerdict, "An OK member report with no verdict for an item -> that item is clean", "REPLACES"},
		{"P4 routes the architect", "review", "op=route command=review roles=pragmatist,", "op=route command=review roles=architect,pragmatist,", "never architect"},
		{"P4 loses the architect one-shot line", "review", architectOneShot, "`architect: one-shot`", "never architect"},
		{"check routes PR mode", "check", "- PR mode stays inline:", "- PR mode routes too:", "keep PR mode inline"},
		{"check routes outside the branch", "check", "## Step 3.5: Verify Findings (--fix only)", "`Skill(skill: \"lets:member-run\", args: \"op=route command=check roles=skeptic\")`\n\n## Step 3.5: Verify Findings (--fix only)", "route only inside"},
		{"check drops --json inline", "check", checkJSONInline + " (`--json`: tooling needs the result in this turn).\n", "", "keep --json inline"},
		{"check spawns", "check", "## Step 3.5: Verify Findings (--fix only)", "Task(\n  subagent_type=\"lets:skeptic\",\n)\n\n## Step 3.5: Verify Findings (--fix only)", "no Task( / Agent("},
		{"roles loses the check sentence", "roles", rolesCheckSentence, "`/lets:check` runs inline", "Who dispatches"},
		{"skeptic VERIFY back to a single finding", "skeptic", "Refute one or more findings against the code", "Refute a single finding against the code", "one or more findings"},
		{"skeptic intro back to ONE finding", "skeptic", "You are handed one or more review findings", "You are handed ONE review finding", "ONE review finding"},
		{"the member report enters the drop rule", "review", memberReportRoute, "the drop rule below runs over the reports read", "never to the drop rule"},
		{"--json routes to members", "review", reviewJSONSolo, "- In a team worktree `--json` routes like any run", "pass solo to every route"},
		{"review loses the PR CONTEXT ban", "review", prContextBan, "**The PR CONTEXT block may go to a skeptic**", "PR CONTEXT block NEVER"},
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
			for _, problem := range teamModeReviewProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}

	// Order mutants: the check branch moved after Step 1, CHECK moved after ## Report.
	moved := func(key, block, anchor string) map[string]string {
		m := make(map[string]string, len(files))
		for k, v := range files {
			m[k] = v
		}
		src := files[key]
		if !strings.Contains(src, block) || !strings.Contains(src, anchor) {
			t.Fatalf("order mutant cannot apply in %s", key)
		}
		src = strings.Replace(src, block, "", 1)
		m[key] = strings.Replace(src, anchor, anchor+block, 1)
		return m
	}
	checkBranch := checkTeamHeading + sectionSpan(files["check"], checkTeamHeading)
	skCheck := "\n" + skepticCheckMode + sectionSpan(files["skeptic"], skepticCheckMode)
	for name, c := range map[string]struct {
		m    map[string]string
		want string
	}{
		"check branch after Step 1":  {moved("check", checkBranch, "## Step 2: Gather Context\n"), "must sit in Step 0"},
		"CHECK mode after ## Report": {moved("skeptic", skCheck, "No `REPORT_FILE` in the prompt -> return the report as your final message.\n"), "inside ## Modes"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, problem := range teamModeReviewProblems(c.m) {
				if strings.Contains(problem, c.want) {
					return
				}
			}
			t.Errorf("order mutant produced no problem containing %q", c.want)
		})
	}
}
