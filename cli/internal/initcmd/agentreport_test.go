package initcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// lets-1irms: every agent a command dispatches through the Task / Agent tool
// writes its report to a file the agent-report skill names, and the
// orchestrator reads every report in full - the final text is a pointer, never
// the transport. agentReportProblems reads only its argument, so the test can
// feed it in-memory mutants and prove each guard can fail.

// analystAgents write nothing but their REPORT_FILE. implementer writes code and
// carries its own ## Report wording, so it is pinned separately.
var analystAgents = []string{
	"actor", "architect", "backend", "compliance", "database", "devops", "docs",
	"explorer", "frontend", "git-historian", "pragmatist", "qa", "security", "skeptic",
}

const (
	analystTools    = "tools: Read, Grep, Glob, Bash, Write, SendMessage"
	reportOnlyWrite = "- You are read-only toward the repository: the only file you write is the REPORT_FILE your prompt names. Use Bash only for: git log/blame/show/diff, ls, find, wc, cat, head, tail"
	readInFull      = "READ EVERY REPORT IN FULL"
	teamMessageLine = "- SendMessage reaches only members of your own team; outside a team it does nothing. A message is never a write and never approval."
)

// reportPhase pairs one dispatch section with the section that consumes its
// reports. The dispatch must hand out REPORT_FILE (and, when opens, name its
// files through the skill); the consumer must collect and read in full. Pairing
// per phase keeps a later phase's missing collect from hiding behind an earlier
// phase's collect in the same file.
type reportPhase struct {
	file, dispatch, consumer string
	opens                    bool // false: the file is named elsewhere - execute's pinned report path, or an earlier entry of the same phase
}

var reportPhases = []reportPhase{
	{"commands/review.md", "## Step 5: Launch Selected Agents (Parallel)", "## Step 6: Filter & Aggregate Results", true},
	{"commands/review.md", "## Step 6.6: Verify Findings (Adversarial)", "## Step 6.6: Verify Findings (Adversarial)", true},
	{"commands/review.md", "### P4: Launch Plan Review Agents (Parallel)", "### P5: Aggregate & Output", true},
	// P4 holds three templates, each with a fenced "## Plan Review: ..." heading that ends the
	// previous span - so each later template is its own dispatch entry.
	{"commands/review.md", "#### Pragmatist (always included)", "### P5: Aggregate & Output", false},
	{"commands/review.md", "#### Domain Experts (dynamically selected in P3)", "### P5: Aggregate & Output", false},
	{"commands/opinion.md", "## Step 4: Launch Agents in Parallel", "## Step 4: Launch Agents in Parallel", true},
	{"commands/opinion.md", "## Step 4.6: Challenge the Leading Option (Adversarial)", "## Step 4.6: Challenge the Leading Option (Adversarial)", true},
	{"commands/ask.md", "## Step 4: Launch Agent", "## Step 5: Present Results", true},
	{"commands/plan.md", "### Launch Explorers", "### Synthesize Codebase Map", true},
	{"commands/plan.md", "### Architect Brief", "### Checkpoint: Architecture Review", true},
	{"commands/plan.md", "### Dispatch Experts", "### Checkpoint: Evaluation Results", true},
	{"commands/backlog.md", "### Phase 1: Explorer - Gather Context", "#### Explorer Failure Guard", true},
	{"commands/backlog.md", "### Phase 3: Launch Brainstorm Agents (Parallel)", "### Phase 4: Aggregate & Present", true},
	{"commands/research.md", "### Research (per-sub-question fan-out via the DEFAULT web subagent)", "### Research (per-sub-question fan-out via the DEFAULT web subagent)", true},
	{"commands/research.md", "### Cross-check (per-claim `lets:skeptic` via Task, RESEARCH-VERIFY mode)", "### Cross-check (per-claim `lets:skeptic` via Task, RESEARCH-VERIFY mode)", true},
	{"commands/execute.md", "### 5-D.4 Dispatch - in plan order", "### 5-D.5 Review - one report at a time", false},
	{"commands/team.md", "### Step M1: One Member (`spawn <role> [name]`)", "### Step M5: Member reports", true},
	{"commands/team.md", "### Step M2: The Whole Roster (`spawn --roster`)", "### Step M5: Member reports", true},
}

const skillCall = `Skill(skill: "lets:agent-report"`

// receiveCall is member-run's wait-loop for a member's round report (Step 6).
const receiveCall = `Skill(skill: "lets:member-run", args: "op=receive`

// dispatchCall finds a Task / Agent call at a line start - a default-subagent
// dispatch carries no subagent_type= and would otherwise slip past the guard.
var dispatchCall = regexp.MustCompile(`(?m)^\s*(Task|Agent)\(`)

// argPlaceholder matches what Claude Code substitutes in a command or skill
// body invoked with args - a dollar sign plus a digit, or dollar-ARGUMENTS -
// fenced code included (verified 2026-09-26 on commands and skills alike). An
// awk field reference written that way arrives as an argument: /lets:review 42
// turned a stash lookup into a syntax error, /lets:handoff --execute turned
// every plan line into a number.
var argPlaceholder = regexp.MustCompile(`\$([0-9]|ARGUMENTS)`)

// dispatchExempt dispatch agents with a prompt they do not author.
var dispatchExempt = map[string]string{
	"skills/member-run/SKILL.md": "its prompt carries the caller's brief pointer and the REPORT_FILE line the caller passes as report-file=",
}

func agentReportProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	ref := sectionSpan(f["agents/architect.md"], "## Report")
	if ref == "" {
		add("agents/architect.md must carry the ## Report section the other analysts copy")
	}
	for _, a := range analystAgents {
		key := "agents/" + a + ".md"
		body := f[key]
		if !strings.Contains(body, "\n"+analystTools+"\n") {
			add(key + " must declare " + analystTools)
		}
		if sectionSpan(body, "## Report") != ref {
			add(key + " ## Report must be identical to architect.md's")
		}
		// "\n## " - explorer.md's "### Constraints & Risks" also contains "## Constraints"
		if !strings.Contains(sectionSpan(body, "\n## Constraints"), reportOnlyWrite) {
			add(key + " ## Constraints must carry the report-only write line")
		}
		if !strings.Contains(sectionSpan(body, "\n## Constraints"), teamMessageLine) {
			add(key + " ## Constraints must carry the team-message line")
		}
	}
	if !strings.Contains(f["agents/implementer.md"], "\ntools: Read, Grep, Glob, Bash, Edit, Write, SendMessage\n") {
		add("agents/implementer.md must declare SendMessage")
	}
	team := f["commands/team.md"]
	if strings.Contains(team, "Do NOT message other teammates directly") || !strings.Contains(team, "Message another teammate directly") {
		add("commands/team.md teammate prompt must allow direct messages between teammates")
	}
	implOut := sectionSpan(f["agents/implementer.md"], "## Output")
	for _, need := range []string{"REPORT_FILE", "REPORT_WRITTEN", "REPORT-END"} {
		if !strings.Contains(implOut, need) {
			add("agents/implementer.md ## Output must carry " + need)
		}
	}

	skill := f["skills/agent-report/SKILL.md"]
	for _, need := range []string{"user-invocable: false", "## op=open", "## op=add", "## op=peek", "## op=collect", "MISSING", "EMPTY", "UNTERMINATED", "UNREADABLE", "REPORT-END", "TAKEN", readInFull, "ONCE", "retry=no", "GAP", "Coverage:"} {
		if !strings.Contains(skill, need) {
			add("the agent-report skill must carry " + need)
		}
	}
	for k, body := range f {
		if !strings.HasPrefix(k, "commands/") && !strings.HasPrefix(k, "skills/") {
			continue
		}
		if m := argPlaceholder.FindString(body); m != "" {
			add(k + " carries the positional placeholder " + m + " - Claude Code replaces it with an argument before the model reads the file")
		}
	}
	ap := f["skills/artifact-path/SKILL.md"]
	if !strings.Contains(ap, "reports-*) DIR=reports") || !strings.Contains(ap, "ext=dir") {
		add("artifact-path must resolve kind=reports-* ext=dir to a directory under .lets/reports")
	}

	registered := map[string]bool{}
	for _, ph := range reportPhases {
		registered[ph.file] = true
		body := f[ph.file]
		dispatch, consumer := sectionSpan(body, ph.dispatch), sectionSpan(body, ph.consumer)
		if dispatch == "" || consumer == "" {
			add(ph.file + " lost the section " + ph.dispatch + " or " + ph.consumer)
			continue
		}
		if !strings.Contains(dispatch, "REPORT_FILE:") {
			add(ph.file + " " + ph.dispatch + " must hand every agent a REPORT_FILE")
		}
		// one REPORT_FILE line per call site at least - a second call added to a
		// section must bring its own path
		if calls, paths := len(dispatchCall.FindAllStringIndex(dispatch, -1)), strings.Count(dispatch, "REPORT_FILE:"); paths < calls {
			add(fmt.Sprintf("%s %s has %d Task/Agent calls but only %d REPORT_FILE lines", ph.file, ph.dispatch, calls, paths))
		}
		if ph.opens && (!strings.Contains(dispatch, skillCall) || (!strings.Contains(dispatch, "op=open") && !strings.Contains(dispatch, "op=add"))) {
			add(ph.file + " " + ph.dispatch + " must name its report files through the agent-report skill (op=open or op=add)")
		}
		// a member-run consumer reads through op=receive (peek, one nudge, collect -
		// pinned on member-run Step 6 by memberrun_route_test.go); it still says it
		// reads every report in full.
		if strings.Contains(consumer, receiveCall) && strings.Contains(consumer, readInFull) {
			continue
		}
		for _, need := range []string{skillCall, "op=collect", readInFull} {
			if !strings.Contains(consumer, need) {
				add(ph.file + " " + ph.consumer + " must carry " + need)
			}
		}
	}
	// A call site inside a registered file must sit inside one of that file's
	// dispatch sections - registering the file must not wave through a new call
	// added somewhere else in it.
	for file := range registered {
		body := f[file]
		for _, loc := range dispatchCall.FindAllStringIndex(body, -1) {
			inside := false
			for _, ph := range reportPhases {
				if ph.file != file {
					continue
				}
				start := strings.Index(body, ph.dispatch)
				if start < 0 {
					continue
				}
				end := start + len(ph.dispatch) + len(sectionSpan(body, ph.dispatch))
				if loc[0] >= start && loc[0] < end {
					inside = true
					break
				}
			}
			if !inside {
				add(fmt.Sprintf("%s has a Task/Agent call at byte %d outside every registered dispatch section", file, loc[0]))
			}
		}
	}
	for name, body := range f {
		if !strings.Contains(body, "subagent_type=") && !dispatchCall.MatchString(body) {
			continue
		}
		if registered[name] {
			continue
		}
		if _, ok := dispatchExempt[name]; ok {
			continue
		}
		add(name + " dispatches agents but is unregistered in reportPhases - it must use the agent-report protocol")
	}

	// team.md Step M5 reads a member's round through member-run op=receive; the order
	// peek -> one nudge (op=correct, never op=next) -> collect retry=no lives in
	// member-run Step 6 and is pinned there (memberrun_route_test.go).

	// the Reports line carries its lead session, so a later lead opens its own directory
	const reportsLine = "`Reports: <REPORT_DIR> (lead <session6>)`"
	if m1 := sectionSpan(f["commands/team.md"], "### Step M1: One Member (`spawn <role> [name]`)"); !strings.Contains(m1, reportsLine) || !strings.Contains(m1, "`## 7. Current task`") {
		add("team.md M1 must record `Reports:` in the team file's Current task")
	}
	if m2 := sectionSpan(f["commands/team.md"], "### Step M2: The Whole Roster (`spawn --roster`)"); !strings.Contains(m2, reportsLine) || !strings.Contains(m2, "`## 7. Current task`") {
		add("team.md M2 must record `Reports:` in the team file's Current task - it skips M1.5")
	}
	for _, h := range []string{"- **Re-explore** ->", "- **Combine** ->"} {
		plan := f["commands/plan.md"]
		i := strings.Index(plan, h)
		if i < 0 {
			add("plan.md lost its " + h + " handler")
			continue
		}
		line := plan[i:]
		if j := strings.Index(line, "\n"); j >= 0 {
			line = line[:j]
		}
		for _, need := range []string{"op=add", "REPORT_FILE:", "op=collect", readInFull} {
			if !strings.Contains(line, need) {
				add("plan.md " + h + " repeat dispatch must carry " + need)
			}
		}
	}
	if !strings.Contains(sectionSpan(f["commands/review.md"], "## Step 8.5: JSON Output"), `"no report"`) {
		add("review.md Step 8.5 must write a missing lens's summary as \"no report\", never \"pass\"")
	}
	if strings.Contains(f["commands/review.md"], "{pass/N issues}") || !strings.Contains(f["commands/review.md"], "`no report` for a lens whose report is a `GAP` - never `pass`") {
		add("review.md's per-category summary must render a missing lens as no report, never pass")
	}
	if !strings.Contains(f["commands/review.md"], "INCOMPLETE if no agent flags revision but any plan-review agent is a GAP") {
		add("review.md P5 must give a plan review with a missing lens the INCOMPLETE verdict, never APPROVED")
	}
	gate := ""
	if i := strings.Index(f["commands/plan.md"], "**HARD-GATE"); i >= 0 {
		gate = f["commands/plan.md"][i:]
		if j := strings.Index(gate, "\n"); j >= 0 {
			gate = gate[:j]
		}
	}
	if !strings.Contains(gate, ".lets/reports/") {
		add("plan.md HARD-GATE must permit the agent report files under .lets/reports/")
	}
	if !strings.Contains(f["commands/plan.md"], "### Coverage gaps") {
		add("plan.md must carry phase gaps into the saved plan (### Coverage gaps)")
	}
	if !strings.Contains(sectionSpan(f["commands/opinion.md"], "## Step 4: Launch Agents in Parallel"), "Zero usable reports") {
		add("opinion.md Step 4 must stop before selecting an option when no panel report arrived")
	}
	if !strings.Contains(sectionSpan(f["commands/github-pr.md"], "### 5.3 Submit verdict"), "Coverage") {
		add("github-pr.md 5.3 verdict body must carry the Coverage / Verification lines when gaps exist")
	}
	if !strings.Contains(sectionSpan(f["commands/github-pr.md"], "### 2.5 Run review analysis"), "gaps") {
		add("github-pr.md 2.5 must copy the review JSON's gaps into its state - a partial review must not post as complete")
	}
	if m5 := sectionSpan(f["commands/team.md"], "### Step M5: Member reports"); !strings.Contains(m5, "`GAP` line goes into the team file's `## 8. Decisions`") {
		add("team.md M5 must write each GAP into the team file's Decisions - a missing report is never nothing to report")
	}
	m4 := sectionSpan(f["commands/team.md"], "### Step M4: Dismiss (`dismiss <name>` / `dismiss --all`)")
	all := ""
	if at := strings.Index(m4, "- `dismiss --all` ->"); at >= 0 {
		all = m4[at:]
		if end := strings.Index(all, "\n"); end >= 0 {
			all = all[:end]
		}
	}
	if collect, dismiss := strings.Index(all, "op=collect"), strings.Index(all, `Skill(skill: "lets:member-run", args: "op=dismiss `); !strings.Contains(all, skillCall) || !strings.Contains(all, readInFull) || collect < 0 || dismiss < collect {
		add("team.md M4 must collect the members' reports before dismiss --all")
	}
	// ## Stop stops LETS session workers (N-steps); they are sessions, not Agent-tool
	// dispatches, and report through /lets:orc - outside agent-report.

	review := f["commands/review.md"]
	save := sectionSpan(review, "## Step 8: Save Review (BEFORE output)")
	if !strings.Contains(save, "## Coverage") || !strings.Contains(save, "NEVER ends without") {
		add("review.md Step 8 must save a ## Coverage section and state that a review NEVER ends without the saved file")
	}
	if !strings.Contains(sectionSpan(review, "## Step 8.5: JSON Output"), `"gaps"`) {
		add(`review.md Step 8.5 must write the additive "gaps" array`)
	}
	sort.Strings(p)
	return p
}

// loadPluginMarkdown reads every agent, command and skill file of the plugin,
// keyed by its plugin-relative slash path.
func loadPluginMarkdown(t *testing.T) map[string]string {
	t.Helper()
	root := pluginDir(t)
	files := map[string]string{}
	for _, pattern := range []string{"agents/*.md", "commands/*.md", "skills/*/SKILL.md"} {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matches {
			b, err := os.ReadFile(m)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(root, m)
			if err != nil {
				t.Fatal(err)
			}
			files[filepath.ToSlash(rel)] = string(b)
		}
	}
	return files
}

// TestSectionSpanLevel4 pins the helper fix: a #### span stops at the next ####.
func TestSectionSpanLevel4(t *testing.T) {
	if got := sectionSpan("#### A\nfirst\n#### B\nsecond\n", "#### A"); got != "\nfirst" {
		t.Errorf("a #### span must end at the next #### heading, got %q", got)
	}
}

// TestAgentReport pins the file transport on the real plugin files, then proves
// each guard can fail: every mutant breaks one invariant in memory and must
// produce a problem naming it.
func TestAgentReport(t *testing.T) {
	files := loadPluginMarkdown(t)
	for _, problem := range agentReportProblems(files) {
		t.Error(problem)
	}

	// section, when set, confines the replacement to that section's span - how a
	// mutant removes ONE phase's collect while other phases keep theirs.
	mutants := []struct {
		name, key, section, old, repl, want string
		n                                   int
	}{
		{"an analyst loses Write", "agents/skeptic.md", "", "\n" + analystTools + "\n", "\ntools: Read, Grep, Glob, Bash\n", "must declare", -1},
		{"an analyst loses SendMessage", "agents/architect.md", "", "\n" + analystTools + "\n", "\ntools: Read, Grep, Glob, Bash, Write\n", "must declare", -1},
		{"a dispatch section loses REPORT_FILE", "commands/ask.md", "", "REPORT_FILE:", "REPORT:", "must hand every agent a REPORT_FILE", -1},
		{"a consumer drops the read-in-full phrase", "commands/opinion.md", "", readInFull, "read the reports", "must carry " + readInFull, -1},
		{"a later phase loses its collect", "commands/opinion.md", "## Step 4.6: Challenge the Leading Option (Adversarial)", "op=collect", "op=skip", "Adversarial) must carry op=collect", -1},
		{"a later P4 template loses REPORT_FILE", "commands/review.md", "#### Pragmatist (always included)", "REPORT_FILE:", "REPORT:", "Pragmatist (always included) must hand every agent a REPORT_FILE", -1},
		{"a consumer section loses its collect", "commands/plan.md", "### Checkpoint: Evaluation Results", "op=collect", "op=skip", "Evaluation Results must carry op=collect", -1},
		{"M2 loses the Reports write", "commands/team.md", "### Step M2: The Whole Roster (`spawn --roster`)", ", then append `Reports: <REPORT_DIR> (lead <session6>)` to the team file's `## 7. Current task`", "", "M2 must record `Reports:`", 1},
		{"the Reports line loses its lead session", "commands/team.md", "### Step M1: One Member (`spawn <role> [name]`)", "`Reports: <REPORT_DIR> (lead <session6>)`", "`Reports: <REPORT_DIR>`", "M1 must record `Reports:`", 1},
		{"dismiss --all dismisses before collecting", "commands/team.md", "### Step M4: Dismiss (`dismiss <name>` / `dismiss --all`)", "then `Skill(skill: \"lets:member-run\", args: \"op=dismiss scope=<c> name=<name> id=<agentId>\")` for every reachable member", "then the same call for every reachable member", "collect the members' reports before dismiss --all", 1},
		{"team M5 loses its op=receive", "commands/team.md", "### Step M5: Member reports", `args: "op=receive `, `args: "op=wait `, "Member reports must carry", 1},
		{"a repeat dispatch loses its protocol", "commands/plan.md", "", "- **Re-explore** ->", "- **Re-explore** -> launch an explorer again\n- **Re-explored** ->", "Re-explore** -> repeat dispatch must carry", 1},
		{"the skill loses its single retry", "skills/agent-report/SKILL.md", "", "ONCE", "twice", "must carry ONCE", -1},
		{"an unregistered dispatch appears", "commands/check.md", "", "\n## ", "\nTask(\n  subagent_type=\"lets:qa\",\n)\n\n## ", "unregistered", 1},
		{"an unregistered default-subagent dispatch appears", "commands/check.md", "", "\n## ", "\nTask(\n  prompt=\"look\",\n)\n\n## ", "unregistered", 1},
		{"a call outside every dispatch section", "commands/review.md", "", "\n## Step 7: Determine Verdict", "\nTask(\n  subagent_type=\"lets:qa\",\n)\n\n## Step 7: Determine Verdict", "outside every registered dispatch section", 1},
		{"a second call in a section without its own path", "commands/ask.md", "## Step 4: Launch Agent", "\nTask(", "\nTask(\n  subagent_type=\"lets:qa\",\n)\n\nTask(", "Task/Agent calls but only", 1},
		{"review stops saving coverage", "commands/review.md", "", "## Coverage", "## Scope", "## Coverage section", -1},
		{"the classifier regains an awk field reference", "skills/agent-report/SKILL.md", "", "# last non-blank line", "# last non-blank line: awk '{print $0}'", "positional placeholder", 1},
		{"a command regains an awk field reference", "commands/handoff.md", "", "{ line[++n] = $z }", "{ line[++n] = $0 }", "commands/handoff.md carries the positional placeholder", 1},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			target := files[m.key]
			if m.section != "" {
				target = sectionSpan(files[m.key], m.section)
			}
			if !strings.Contains(target, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in %s %s - update the mutant with the text it guards", m.old, m.key, m.section)
			}
			mutated := make(map[string]string, len(files))
			for k, v := range files {
				mutated[k] = v
			}
			if m.section != "" {
				mutated[m.key] = strings.Replace(files[m.key], target, strings.Replace(target, m.old, m.repl, m.n), 1)
			} else {
				mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, m.n)
			}
			for _, problem := range agentReportProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}

// TestAgentReportClassifier runs the Step 1 block of the agent-report skill -
// extracted from SKILL.md exactly as the model receives it, which the
// placeholder lint above makes equal to the text on disk. Step 1 judges a name's
// own file and its -r2 retry file together and prints ONE resolved line per
// name (lets-bt44m): the test drives every (base, retry) state pair through it,
// plus the richness ties and the single-file fixtures, against a Go oracle of
// the rule.
func TestAgentReportClassifier(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the classifier is a bash block")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	skill, err := os.ReadFile(filepath.Join(pluginDir(t), "skills", "agent-report", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	step := sectionSpan(string(skill), "### Step 1: Classify every expected file")
	i := strings.Index(step, "```bash\n")
	if i < 0 {
		t.Fatal("agent-report Step 1 lost its bash block")
	}
	block := step[i+len("```bash\n"):]
	j := strings.Index(block, "\n```")
	if j < 0 {
		t.Fatal("agent-report Step 1 bash block is not closed")
	}
	block = block[:j]

	root := os.Geteuid() == 0 // root reads a 000 file: every UNREADABLE fixture is skipped
	dir := t.TempDir()
	write := func(file, body string, locked bool) {
		t.Helper()
		path := filepath.Join(dir, file)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if locked {
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
		}
	}
	// side is one file of a name: its state as the rule sees it, and its non-blank
	// line count (only an UNTERMINATED side's count decides anything).
	type side struct {
		state string
		lines int
	}
	type expect struct{ state, path string }
	// oracle is the resolution rule, written independently of the bash.
	oracle := func(name string, base, retry side) expect {
		bp, rp := filepath.Join(dir, name+".md"), filepath.Join(dir, name+"-r2.md")
		switch {
		case retry.state == "OK":
			return expect{"OK", rp}
		case base.state == "OK":
			return expect{"OK", bp}
		case retry.state == "UNTERMINATED" && (base.state != "UNTERMINATED" || retry.lines >= base.lines):
			return expect{"UNTERMINATED", rp}
		case base.state == "UNTERMINATED":
			return expect{"UNTERMINATED", bp}
		case retry.state != "MISSING":
			return expect{retry.state, "none"}
		default:
			return expect{base.state, "none"}
		}
	}
	want := map[string]expect{}

	// The full state matrix: 25 names p-<base>-<retry>.
	bodies := map[string]string{
		"EMPTY":        "",
		"UNTERMINATED": "finding\n",
		"UNREADABLE":   "finding\nREPORT-END\n",
		"OK":           "finding\nREPORT-END\n",
	}
	states := []string{"MISSING", "EMPTY", "UNTERMINATED", "UNREADABLE", "OK"}
	for _, b := range states {
		for _, r := range states {
			if root && (b == "UNREADABLE" || r == "UNREADABLE") {
				continue
			}
			name := "p-" + strings.ToLower(b) + "-" + strings.ToLower(r)
			if b != "MISSING" {
				write(name+".md", bodies[b], b == "UNREADABLE")
			}
			if r != "MISSING" {
				write(name+"-r2.md", bodies[r], r == "UNREADABLE")
			}
			want[name] = oracle(name, side{b, 1}, side{r, 1})
		}
	}

	// Richness: two unfinished files - the larger non-blank line count wins, a tie
	// goes to the retry.
	for name, n := range map[string][2]int{"richbase": {3, 1}, "richretry": {1, 2}, "tie": {2, 2}} {
		write(name+".md", strings.Repeat("finding\n", n[0]), false)
		write(name+"-r2.md", strings.Repeat("finding\n", n[1]), false)
		want[name] = oracle(name, side{"UNTERMINATED", n[0]}, side{"UNTERMINATED", n[1]})
	}

	// Single-file fixtures (no retry file): one per classification edge.
	singles := []struct{ name, body, state string }{
		{"good", "finding\nREPORT-END\n", "OK"},
		{"empty", "", "EMPTY"},
		{"blank", "  \n\t\n", "EMPTY"},
		{"only", "REPORT-END\n", "EMPTY"},
		{"cut", "finding, no sentinel\n", "UNTERMINATED"},
		{"crlf", "finding\r\nREPORT-END\r\n", "OK"},
		{"trailing", "finding\nREPORT-END   \n\n", "OK"},
		{"body-sentinel", "REPORT-END\nlater line\n", "UNTERMINATED"},
		{"run-lets-1.2", "finding\nREPORT-END\n", "OK"},
		{"locked", "finding\nREPORT-END\n", "UNREADABLE"},
	}
	for _, s := range singles {
		if root && s.state == "UNREADABLE" {
			continue
		}
		write(s.name+".md", s.body, s.state == "UNREADABLE")
		want[s.name] = oracle(s.name, side{s.state, 1}, side{"MISSING", 0})
	}
	want["gone"] = oracle("gone", side{"MISSING", 0}, side{"MISSING", 0})

	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	script := strings.NewReplacer("{dir}", dir, "{names}", strings.Join(names, ",")).Replace(block)
	out, err := exec.Command(bash, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("classifier failed: %v\n%s", err, out)
	}

	// The one-line invariant, checked before the map is built so a duplicate line
	// cannot overwrite another: exactly one REPORT <name> <state> <path|none> line
	// per requested name, none for an unrequested one.
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(names) {
		t.Errorf("classifier printed %d lines for %d names:\n%s", len(lines), len(names), out)
	}
	got := map[string]expect{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[0] != "REPORT" {
			t.Errorf("malformed line %q - want REPORT <name> <state> <path|none>", line)
			continue
		}
		if _, ok := want[fields[1]]; !ok {
			t.Errorf("line for an unrequested name: %q", line)
			continue
		}
		if _, dup := got[fields[1]]; dup {
			t.Errorf("a second line for %s: %q", fields[1], line)
			continue
		}
		got[fields[1]] = expect{fields[2], fields[3]}
	}
	for _, n := range names {
		g, ok := got[n]
		if !ok {
			t.Errorf("%s: no line", n)
			continue
		}
		if g != want[n] {
			t.Errorf("%s: resolved %s %s, want %s %s", n, g.state, g.path, want[n].state, want[n].path)
		}
	}
}

// TestAgentReportAdd runs the op=add block of the agent-report skill - extracted
// as TestAgentReportClassifier extracts Step 1 - and requires it to hand out a
// path only for a name with no file yet. An existing file belongs to an earlier
// dispatch; handing it out again would let op=collect read that report as the new
// one's (lets-bt44m). An empty file is taken too: the Write tool will not
// overwrite a file the new agent has not read.
func TestAgentReportAdd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("op=add is a bash block")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	skill, err := os.ReadFile(filepath.Join(pluginDir(t), "skills", "agent-report", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	section := sectionSpan(string(skill), "## op=add - a later phase of the same run")
	i := strings.Index(section, "```bash\n")
	if i < 0 {
		t.Fatal("agent-report op=add lost its bash block")
	}
	block := section[i+len("```bash\n"):]
	j := strings.Index(block, "\n```")
	if j < 0 {
		t.Fatal("agent-report op=add bash block is not closed")
	}
	block = block[:j]

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.md"), []byte("an earlier report\nREPORT-END\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	script := strings.NewReplacer("{dir}", dir, "{names}", "fresh,old,empty").Replace(block)
	out, err := exec.Command(bash, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("op=add failed: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"REPORT_FILE fresh=" + filepath.Join(dir, "fresh.md"),
		"TAKEN old " + filepath.Join(dir, "old.md"),
		"TAKEN empty " + filepath.Join(dir, "empty.md"),
	}, "\n")
	if got != want {
		t.Errorf("op=add output:\n%s\nwant:\n%s", got, want)
	}
}
