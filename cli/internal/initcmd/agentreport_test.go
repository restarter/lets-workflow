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
	{"commands/team.md", "### Step R8: Spawn Teammates", "### Step R10: Completion", true},
}

const skillCall = `Skill(skill: "lets:agent-report"`

// dispatchCall finds a Task / Agent call at a line start - a default-subagent
// dispatch carries no subagent_type= and would otherwise slip past the guard.
var dispatchCall = regexp.MustCompile(`(?m)^\s*(Task|Agent)\(`)

// argPlaceholder matches what Claude Code substitutes in a skill body invoked
// with args - a dollar sign plus a digit, or dollar-ARGUMENTS - fenced code
// included. An awk field reference written that way arrives as an argument.
var argPlaceholder = regexp.MustCompile(`\$([0-9]|ARGUMENTS)`)

// argSkills are invoked with args on every call, so their bodies must render
// as written.
var argSkills = []string{"skills/agent-report/SKILL.md", "skills/artifact-path/SKILL.md"}

// dispatchExempt dispatch agents with a prompt they do not author.
var dispatchExempt = map[string]string{
	"skills/implementer-run/SKILL.md": "its prompt is the caller's chunk file; execute.md hands out the REPORT_FILE",
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
	for _, need := range []string{"user-invocable: false", "## op=open", "## op=add", "## op=peek", "## op=collect", "MISSING", "EMPTY", "UNTERMINATED", "UNREADABLE", "REPORT-END", readInFull, "ONCE", "retry=no", "GAP", "Coverage:"} {
		if !strings.Contains(skill, need) {
			add("the agent-report skill must carry " + need)
		}
	}
	for _, k := range argSkills {
		if m := argPlaceholder.FindString(f[k]); m != "" {
			add(k + " carries the positional placeholder " + m + " - Claude Code replaces it with an argument before the model reads the skill")
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

	r10 := sectionSpan(f["commands/team.md"], "### Step R10: Completion")
	peek := strings.Index(r10, "op=peek")
	nudge := strings.Index(r10, "Write your final report to REPORT_FILE")
	final := strings.LastIndex(r10, "op=collect")
	if peek < 0 || nudge < peek || final < nudge {
		add("team.md Step R10 must peek, nudge once, then collect - in that order, so no gap is declared before its recovery")
	}

	if r8 := sectionSpan(f["commands/team.md"], "### Step R8: Spawn Teammates"); !strings.Contains(r8, `"report_dir"`) || !strings.Contains(r8, `"status": "running"`) {
		add("team.md Step R8 must persist the running team record with report_dir before the spawn")
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
	if !strings.Contains(f["commands/team.md"], `"report_gaps"`) {
		add("team.md must persist report gaps in the team record - the tracker comment covers completed tasks only")
	}
	if s3 := sectionSpan(f["commands/team.md"], "### Step S3: Recovery Detection"); !strings.Contains(s3, skillCall) || !strings.Contains(s3, "op=collect") || !strings.Contains(s3, readInFull) {
		add("team.md S3 must collect the reports of an orphaned team before marking it orphaned")
	}
	stop := sectionSpan(f["commands/team.md"], "## Stop")
	if !strings.Contains(stop, "report_dir") {
		add("team.md ## Stop must read report_dir from the running team record")
	}
	if !strings.Contains(stop, skillCall) || !strings.Contains(stop, "op=collect") || !strings.Contains(stop, readInFull) {
		add("team.md ## Stop must collect the reports that exist before shutting the team down")
	}

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
		{"team concludes gaps before its nudge", "commands/team.md", "### Step R10: Completion", "op=peek", "op=collect", "peek, nudge once, then collect", -1},
		{"a repeat dispatch loses its protocol", "commands/plan.md", "", "- **Re-explore** ->", "- **Re-explore** -> launch an explorer again\n- **Re-explored** ->", "Re-explore** -> repeat dispatch must carry", 1},
		{"the skill loses its single retry", "skills/agent-report/SKILL.md", "", "ONCE", "twice", "must carry ONCE", -1},
		{"an unregistered dispatch appears", "commands/check.md", "", "\n## ", "\nTask(\n  subagent_type=\"lets:qa\",\n)\n\n## ", "unregistered", 1},
		{"an unregistered default-subagent dispatch appears", "commands/check.md", "", "\n## ", "\nTask(\n  prompt=\"look\",\n)\n\n## ", "unregistered", 1},
		{"a call outside every dispatch section", "commands/review.md", "", "\n## Step 7: Determine Verdict", "\nTask(\n  subagent_type=\"lets:qa\",\n)\n\n## Step 7: Determine Verdict", "outside every registered dispatch section", 1},
		{"a second call in a section without its own path", "commands/ask.md", "## Step 4: Launch Agent", "\nTask(", "\nTask(\n  subagent_type=\"lets:qa\",\n)\n\nTask(", "Task/Agent calls but only", 1},
		{"review stops saving coverage", "commands/review.md", "", "## Coverage", "## Scope", "## Coverage section", -1},
		{"the classifier regains an awk field reference", "skills/agent-report/SKILL.md", "", "# last non-blank line", "# last non-blank line: awk '{print $0}'", "positional placeholder", 1},
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
// placeholder lint above makes equal to the text on disk - over one fixture
// per state.
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

	dir := t.TempDir()
	fixtures := map[string]string{
		"good":            "finding\nREPORT-END\n",
		"empty":           "",
		"blank":           "  \n\t\n",
		"only":            "REPORT-END\n",
		"cut":             "finding, no sentinel\n",
		"crlf":            "finding\r\nREPORT-END\r\n",
		"trailing":        "finding\nREPORT-END   \n\n",
		"body-sentinel":   "REPORT-END\nlater line\n",
		"report-lets-1.2": "finding\nREPORT-END\n",
		"locked":          "finding\nREPORT-END\n",
	}
	for name, body := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{
		"good": "OK", "gone": "MISSING", "empty": "EMPTY", "blank": "EMPTY", "only": "EMPTY",
		"cut": "UNTERMINATED", "crlf": "OK", "trailing": "OK", "body-sentinel": "UNTERMINATED",
		"report-lets-1.2": "OK", "locked": "UNREADABLE",
	}
	if os.Geteuid() == 0 {
		delete(want, "locked") // root reads a 000 file
	} else {
		locked := filepath.Join(dir, "locked.md")
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(locked, 0o600) }()
	}
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
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 {
			got[fields[1]] = fields[0]
		}
	}
	for _, n := range names {
		if got[n] != want[n] {
			t.Errorf("%s: classified %q, want %q", n, got[n], want[n])
		}
	}
}
