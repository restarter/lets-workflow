package initcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// lets-mpjhc / lets-kwd6e: member-run decides which standing-team member takes a
// dispatch (op=route), waits for a member's round report (op=receive: peek, one
// nudge, collect) and messages every member but a recorded-dismissed one by its id -
// SendMessage resumes a stopped member, so LETS no longer reads "not listed" as gone.
// memberRunRouteProblems reads only its argument, so TestMemberRunRoute feeds it
// in-memory mutants and proves each guard can fail.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

// teamFileLiterals are the team-file `## 7` line forms member-run reads and its
// callers write - one spelling, or a line written by one is never found by another.
var teamFileLiterals = []string{
	"`Member: <name> = <agentId> lets:<role> (lead <session6>)`",
	"`Member: <name> = gone (lead <session6>)`",
	"`Dismissed: <name> (lead <session6>)`",
	"`Members asked: (lead <session6>)`",
}

const (
	writeReportNudge = "Write your final report to REPORT_FILE"
	resumeNudge      = "You stopped before your report. Continue {brief-file} from where you stopped - do not redo finished work - then write REPORT_FILE"
	receiveRule1     = "   1. this is the caller's own check right after it sent this round's NEXT or ran the phase's Task calls, with no notification from that member since -> return `WAITING`"
	rule1GoesOn      = "a notification from that member - even in the same turn - a resumed session or the lead re-checking goes on"
	membersByRole    = "every name whose LATEST line of this lead session is `Member: <name> = <agentId> lets:<role> (lead <session6>)`"
	teamSkillCall    = `Skill(skill: "lets:team"`
)

func memberRunRouteProblems(skill string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	step2 := sectionSpan(skill, "## Step 2: Spawn")
	step3 := sectionSpan(skill, "## Step 3: Next / Correct")
	step5 := sectionSpan(skill, "## Step 5: Route (op=route)")
	step6 := sectionSpan(skill, "## Step 6: Receive (op=receive)")
	if step2 == "" || step3 == "" || step5 == "" || step6 == "" {
		add("member-run must keep Step 2: Spawn, Step 3: Next / Correct, Step 5: Route (op=route) and Step 6: Receive (op=receive)")
		return p
	}

	// Step 3: resume by id, never refuse; never NEXT a busy member.
	send := strings.Index(step3, "SendMessage(")
	pre, post := "", ""
	if send >= 0 {
		pre, post = step3[:send], step3[send:]
	}
	if !strings.Contains(pre, "recorded dismissed is never messaged") || !strings.Contains(pre, "resumes a stopped member") {
		add("member-run Step 3 must say, before its SendMessage, that a member the caller recorded dismissed is never messaged and that SendMessage resumes a stopped member")
	}
	if strings.Contains(skill, "LETS refuses to") {
		add("member-run must not refuse a resume - no \"LETS refuses to\" anywhere (lets-kwd6e)")
	}
	busy := ""
	for _, l := range strings.Split(pre, "\n") {
		if strings.Contains(l, "`member_busy: {agent}`") {
			busy = l
		}
	}
	if !strings.Contains(busy, "`op=next`") || !strings.Contains(busy, "listed `running`") || !strings.Contains(busy, "send nothing") {
		add("member-run Step 3 must return member_busy for an op=next to an id listed running, and send nothing, before its SendMessage")
	}
	if strings.Contains(skill, "Not listed: with `id=`") {
		add("member-run must drop the old refusal \"Not listed: with `id=`\" - a member is messaged by its id, listed or not")
	}
	if !strings.Contains(post, "The address does not resolve -> run `ListAgents`, return `agent_gone: {agent}`") || !strings.Contains(post, "Never spawn a replacement here") {
		add("member-run Step 3 must return agent_gone, after its SendMessage, when the address does not resolve - and never spawn a replacement there")
	}

	// Step 6: peek, one nudge, collect.
	if !strings.Contains(step6, "`GAP <name> <state> gone` when this call reached 4 through `agent_gone`") || !strings.Contains(step6, "member-run never writes the team file") {
		add("member-run Step 6 must return GAP <name> <state> gone after an agent_gone, and say member-run never writes the team file")
	}
	peek, write, correct, collect := strings.Index(step6, "op=peek"), strings.Index(step6, writeReportNudge), strings.Index(step6, "`op=correct`"), strings.Index(step6, "op=collect")
	if peek < 0 || write < peek || correct < write || collect < correct {
		add("member-run Step 6 must run op=peek, then the one nudge (the write-report text, sent as op=correct), then op=collect")
	}
	if n := strings.Count(step6, "op=collect"); n == 0 || n != strings.Count(step6, "retry=no") {
		add("member-run Step 6: every op=collect carries retry=no - agent-report never retries a member")
	}
	for _, need := range []string{"READ EVERY REPORT IN FULL", "never a second nudge", resumeNudge} {
		if !strings.Contains(step6, need) {
			add("member-run Step 6 must carry " + need)
		}
	}
	if r1 := strings.Index(step6, receiveRule1); r1 < 0 || write < r1 || !strings.Contains(step6, rule1GoesOn) {
		add("member-run Step 6 rule 1 must come before the nudge rule and return WAITING only for the caller's own post-dispatch check with no notification from that member since - a member notification always goes on")
	}

	// The team-file literals, spelled once.
	for _, lit := range teamFileLiterals {
		if !strings.Contains(skill, lit) {
			add("member-run must use the exact team-file literal " + lit)
		}
	}

	// R1: activation.
	r1 := between(step5, "**R1 Activation**", "**R2 Per role**")
	if !strings.Contains(r1, "1. `solo` -> `TEAM off solo`") {
		add("member-run R1 must turn team mode off on solo")
	}
	info := ""
	for _, l := range strings.Split(r1, "\n") {
		if strings.Contains(l, "`lets worktree info --json`") {
			info = l
		}
	}
	if !strings.Contains(info, "ANY failure") || !strings.Contains(info, "-> `TEAM off no-team`") || strings.Contains(info, "stop") {
		add("member-run R1: ANY failure of lets worktree info -> TEAM off no-team, never a stop (fail open to today)")
	}
	other, unknown, lead := strings.Index(r1, "`other` -> `TEAM off lead-other`"), strings.Index(r1, "`unknown` ->"), strings.Index(r1, "`lead` -> `TEAM on <c>`")
	unknownClause := ""
	if unknown >= 0 && lead > unknown {
		unknownClause = r1[unknown:lead]
	}
	if other < 0 || !strings.Contains(unknownClause, "`TEAM off lead-unknown`") || strings.Contains(unknownClause, "TEAM on") || strings.Count(r1, "`TEAM on <c>`") != 2 || !strings.Contains(r1, "2. `team=<c>` (the caller's earlier route in this run) -> `TEAM on <c>`") {
		add("member-run R1: TEAM on only after team_lead.match lead (or the caller's earlier team=<c>); other and unknown are TEAM off (lead-other, lead-unknown)")
	}

	// R2: the roster's role column first, then members by the lets:<role> of their Member: lines.
	r2 := between(step5, "**R2 Per role**", "**R3 Members question**")
	notInRoster, members := strings.Index(r2, "`ROUTE <role> <fallback> not-in-roster`"), strings.Index(r2, membersByRole)
	if notInRoster < 0 || members < 0 || notInRoster > members || !strings.Contains(r2, "the `role` column") || !strings.Contains(r2, "whatever `Member:` lines exist") || !strings.Contains(r2, "has no roster row and is still the architect") {
		add("member-run R2 must check the roster's role column first (not-in-roster whatever Member: lines exist), then find members by the lets:<role> of their Member: lines, not by roster name")
	}
	if !strings.Contains(r2, "`ROUTE <role> <fallback> busy`") {
		add("member-run R2 must route a role whose every reachable member is busy to its fallback (busy)")
	}

	// R3: the only lets:team call, with members.
	r3 := sectionSpan(step5, "**R3 Members question**")
	if strings.Count(skill, teamSkillCall) != 1 || !strings.Contains(r3, teamSkillCall+`, args: "members `) {
		add("member-run R3 must be its only " + teamSkillCall + " call, and its args start with members - lets:team never calls op=route back")
	}

	// Step 2: the Agent block passes no name=; the prose makes it conditional.
	call := ""
	if at := strings.Index(step2, "Agent("); at >= 0 {
		call = step2[at:]
		for i, line := range strings.Split(call, "\n") {
			if i > 0 && strings.TrimSpace(line) == ")" {
				call = strings.Join(strings.Split(call, "\n")[:i], "\n")
				break
			}
		}
	}
	if call == "" || strings.Contains(call, "name=") || !strings.Contains(step2, "Pass `name=\"{agent}\"` too when the Agent tool offers a `name` parameter") {
		add("member-run Step 2: the Agent block passes no name= - pass name=\"{agent}\" only when the Agent tool offers a name parameter; the recorded id is the address")
	}
	return p
}

func TestMemberRunRoute(t *testing.T) {
	skill := readPlugin(t, filepath.Join("skills", "member-run", "SKILL.md"))
	for _, problem := range memberRunRouteProblems(skill) {
		t.Error(problem)
	}
}

// TestMemberRunRoute_Mutants proves each guard of memberRunRouteProblems can fail.
func TestMemberRunRoute_Mutants(t *testing.T) {
	skill := readPlugin(t, filepath.Join("skills", "member-run", "SKILL.md"))
	mutants := []struct{ name, old, repl, want string }{
		{"refusal re-inserted", "   - `link: peer`", "   - Never message it: the harness can still resume a `gone` or dismissed agent by name or id, and LETS refuses to.\n   - `link: peer`", "LETS refuses to"},
		{"member_busy clause deleted", "   - `op=next` and the member's id is listed `running` -> return `member_busy: {agent}` and send nothing: a NEXT replaces the round the member is working on. ", "   - ", "member_busy"},
		{"old not-listed refusal restored", "   - Address: with `id=` -> send to the id;", "   - Not listed: with `id=` -> send to the id;", "Not listed: with `id=`"},
		{"GAP loses gone", "or `GAP <name> <state> gone` when this call reached 4 through `agent_gone`", "or `GAP <name> <state>` when this call reached 4 through `agent_gone`", "GAP <name> <state> gone"},
		{"peek and collect swapped", `args: "op=peek dir=`, `args: "op=collect dir=`, "op=peek, then the one nudge"},
		{"collect retries", ` retry=no")`, `")`, "retry=no"},
		{"rule 1 deleted", receiveRule1, "   1. never mind", "rule 1"},
		{"rule 1 covers a member notification", "with no notification from that member since -> return `WAITING`", "or on a notification from that member -> return `WAITING`", "rule 1"},
		{"Member literal respelled", "is `Member: <name> = <agentId> lets:<role> (lead <session6>)`", "is `Member: <name> = ...`", "exact team-file literal"},
		{"R2 keyed on roster names only", "every name whose LATEST line of this lead session is", "every roster name of that role whose LATEST line of this lead session is", "role column first"},
		{"unknown turns team mode on", "`unknown` -> `TEAM off lead-unknown`", "`unknown` -> `TEAM on <c>`", "TEAM on only after"},
		{"an info failure stops", "or no top-level `team` -> `TEAM off no-team`", "or no top-level `team` -> stop", "ANY failure"},
		{"busy route dropped", "  - every reachable member busy -> `ROUTE <role> <fallback> busy`;\n", "", "busy"},
		{"lets:team args gain op=route", teamSkillCall + `, args: "members `, teamSkillCall + `, args: "op=route members `, "args start with members"},
		{"name= back in the Agent block", "     subagent_type=\"{role}\",\n", "     subagent_type=\"{role}\",\n     name=\"{agent}\",\n", "passes no name="},
	}
	// R2 looks up Member: lines before the roster check: the two R2 bullets swap places.
	swapped := ""
	if r2 := between(skill, "**R2 Per role**", "**R3 Members question**"); r2 != "" {
		var roster, members string
		for _, l := range strings.Split(r2, "\n") {
			switch {
			case strings.HasPrefix(l, "- First eligibility: "):
				roster = l
			case strings.HasPrefix(l, "- Otherwise the role's members: "):
				members = l
			}
		}
		if roster != "" && members != "" {
			swapped = strings.Replace(strings.Replace(skill, roster, "\x00", 1), members, roster, 1)
			swapped = strings.Replace(swapped, "\x00", members, 1)
		}
	}
	if swapped == "" {
		t.Fatal("mutant cannot apply: the two R2 bullets (- First eligibility: / - Otherwise the role's members: ) are not in member-run")
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(skill, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in member-run - update the mutant with the text it guards", m.old)
			}
			for _, problem := range memberRunRouteProblems(strings.Replace(skill, m.old, m.repl, 1)) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
	t.Run("R2 looks up Member: lines before the roster", func(t *testing.T) {
		for _, problem := range memberRunRouteProblems(swapped) {
			if strings.Contains(problem, "role column first") {
				return
			}
		}
		t.Error("mutant produced no problem containing \"role column first\" - that guard cannot fail")
	})
}
