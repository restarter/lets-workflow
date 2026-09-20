package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handoffLaunchGuard is the 7.3 line that stops --send without LETS_LAUNCHER=orca.
const handoffLaunchGuard = "Only when `{LETS_LAUNCHER}` is `orca`"

// lintHandoff checks the /lets:handoff command and its alias: Orca is reached only
// through `lets handoff` and only after the launcher guard, Codex only through
// `lets handoff codex`, nothing interrupts an agent, no result file is read, and
// the alias delegates without calling Go itself.
func lintHandoff(handoff, alias string) []string {
	var bad []string
	guard := strings.Index(handoff, handoffLaunchGuard)
	for _, call := range []string{"lets handoff targets", "lets handoff send"} {
		if i := strings.Index(handoff, call); i >= 0 && (guard < 0 || i < guard) {
			bad = append(bad, "handoff.md: "+call+" before the LETS_LAUNCHER=orca guard")
		}
	}
	for name, body := range map[string]string{"handoff.md": handoff, "review-handoff.md": alias} {
		for _, lit := range []string{"codex exec", "Orca.app", "orca terminal", "--interrupt"} {
			if strings.Contains(body, lit) {
				bad = append(bad, name+": "+lit+" (Go owns Codex and Orca; there is no interrupt)")
			}
		}
	}
	if strings.Count(alias, `Skill(skill: "lets:handoff"`) != 1 || strings.Contains(alias, "lets handoff ") {
		bad = append(bad, "review-handoff.md: the alias delegates once to lets:handoff and calls nothing itself")
	}
	if strings.Contains(handoff, "-result.json") {
		bad = append(bad, "handoff.md: the result is the background run's own output, never a -result.json")
	}
	return bad
}

// TestHandoffLint pins the delivery boundaries of /lets:handoff (lets-w5tm5).
func TestHandoffLint(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "plugins", "lets", "commands")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	handoff, alias := read("handoff.md"), read("review-handoff.md")
	if !strings.Contains(handoff, handoffLaunchGuard) {
		t.Fatal("handoff.md: the 7.3 launcher guard line is missing")
	}
	for _, v := range lintHandoff(handoff, alias) {
		t.Error(v)
	}
	// mutation self-check: each assertion can fail
	for name, m := range map[string][2]string{
		"send before guard":  {"lets handoff send --brief x\n" + handoff, alias},
		"codex exec":         {handoff + "\ncodex exec -", alias},
		"orca terminal":      {handoff, alias + "\norca terminal send"},
		"interrupt":          {handoff + "\n--interrupt", alias},
		"alias calls Go":     {handoff, alias + "\nlets handoff codex --brief x"},
		"alias delegates 2x": {handoff, alias + "\n" + `Skill(skill: "lets:handoff", args: "x")`},
		"result file":        {handoff + "\nRead <base>-result.json", alias},
	} {
		if len(lintHandoff(m[0], m[1])) == 0 {
			t.Errorf("mutation %q must fail the lint", name)
		}
	}
}

// handoffExecuteLines are the Step 1 refusals and the 5b contract rules an execution
// brief depends on (lets-g5wuz): a lane that cannot write is refused by name, and the
// agent is told what it may never do and how to stop.
var handoffExecuteLines = []string{
	"`--execute` hands over a plan",
	"headless execution needs a writable sandbox",
	"a new session opens read-only",
	"`--execute` goes to an open agent tab",
	"Never push, open or update a pull request, merge, rebase, or touch the task tracker.",
	"STOP and write the reason to the report",
	"the only files you may write outside the scope above",
	"If you cannot write these two files, print the report as your final message instead.",
}

// planBanners are the two banner lines plan.md and plan-workflow.md write. The
// handoff.md filter strips them by these words, so a reworded banner fails here
// instead of reaching a foreign agent unstripped.
var planBanners = []string{"STOP - THIS PLAN IS NOT A GO", "REMINDER: do not start writing code"}

func lintHandoffExecute(handoff, plan, planWorkflow string) []string {
	var bad []string
	for _, l := range handoffExecuteLines {
		if !strings.Contains(handoff, l) {
			bad = append(bad, "handoff.md: missing "+l)
		}
	}
	for _, b := range planBanners {
		for name, body := range map[string]string{"plan.md": plan, "plan-workflow.md": planWorkflow} {
			if !strings.Contains(body, "> **"+b) {
				bad = append(bad, name+": no banner line \"> **"+b+"\" - handoff.md strips the banner by it")
			}
		}
		if !strings.Contains(handoff, b) {
			bad = append(bad, "handoff.md: the plan filter no longer matches "+b)
		}
	}
	return bad
}

// TestHandoffExecuteLint pins the execution brief of /lets:handoff (lets-g5wuz).
func TestHandoffExecuteLint(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "commands", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	handoff, plan, pw := read("handoff.md"), read("plan.md"), read("plan-workflow.md")
	for _, v := range lintHandoffExecute(handoff, plan, pw) {
		t.Error(v)
	}
	// mutation self-check: each assertion can fail
	for name, m := range map[string][3]string{
		"refusal dropped":  {strings.ReplaceAll(handoff, "a new session opens read-only", "x"), plan, pw},
		"contract dropped": {strings.ReplaceAll(handoff, "STOP and write the reason to the report", "x"), plan, pw},
		"banner reworded":  {handoff, strings.ReplaceAll(plan, "THIS PLAN IS NOT A GO", "THIS PLAN IS NOT READY"), pw},
		"filter reworded":  {strings.ReplaceAll(handoff, "REMINDER: do not start writing code", "x"), plan, pw},
	} {
		if len(lintHandoffExecute(m[0], m[1], m[2])) == 0 {
			t.Errorf("mutation %q must fail the lint", name)
		}
	}
}

// handoffAwk extracts the plan filter from the Step 5b assembly block of handoff.md.
var handoffAwk = regexp.MustCompile(`(?s)awk '\n(.*?)' "\$PLAN"`)

// planBannerLine returns the banner line a command file writes into a plan, without
// the indent plan-workflow.md carries it with, so the two copies can be compared and
// the filter can be tested against the real text instead of a copy of it.
func planBannerLine(t *testing.T, body, name, prefix string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^[ \t]*(> \*\*` + regexp.QuoteMeta(prefix) + `.*)$`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("%s: no banner line starting %q", name, "> **"+prefix)
	}
	return m[1]
}

// TestHandoffPlanFilter runs the handoff.md plan filter (lets-g5wuz) over the three
// plan shapes it meets: /lets:plan (banner under the title block), /lets:plan-workflow
// (banner on line 1), and a plan without a banner whose snippet quotes it.
func TestHandoffPlanFilter(t *testing.T) {
	awkBin, err := exec.LookPath("awk")
	if err != nil {
		t.Skip("awk not on PATH")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "commands", "handoff.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := handoffAwk.FindSubmatch(b)
	if m == nil {
		t.Fatal(`handoff.md: no awk '...' "$PLAN" block in Step 5b`)
	}
	// The fixtures carry the REAL banner lines, read from the files that write them: a
	// copy here would keep passing after a reworded banner while the filter stopped
	// matching it (a `GO.` turned into `GO!` fails the awk regex, not a prefix check).
	read := func(name string) string {
		body, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "commands", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	plan, pw := read("plan.md"), read("plan-workflow.md")
	stop := planBannerLine(t, plan, "plan.md", "STOP - THIS PLAN IS NOT A GO")
	rem := planBannerLine(t, plan, "plan.md", "REMINDER: do not start writing code")
	// plan-workflow.md applies the same two lines at save time and says they are
	// byte-identical to plan.md's - otherwise the filter strips one shape, not the other.
	if got := planBannerLine(t, pw, "plan-workflow.md", "STOP - THIS PLAN IS NOT A GO"); got != stop {
		t.Errorf("STOP banner drift:\nplan.md          %q\nplan-workflow.md %q", stop, got)
	}
	if got := planBannerLine(t, pw, "plan-workflow.md", "REMINDER: do not start writing code"); got != rem {
		t.Errorf("REMINDER banner drift:\nplan.md          %q\nplan-workflow.md %q", rem, got)
	}
	snippet := "```markdown\n" + stop + "\n" + rem + "\n```\n"
	for name, c := range map[string]struct{ in, want, stderr string }{
		"plan": {
			in:     "# T\n\n**Task:** x\n\n" + stop + "\n\n---\n\n## Context\n\n" + snippet + "\n---\n\n" + rem + "\n",
			want:   "# T\n\n**Task:** x\n\n\n---\n\n## Context\n\n" + snippet,
			stderr: "stop_removed=1 reminder_removed=1",
		},
		"plan-workflow": {
			in:     stop + "\n\n# T\n\n## Context\nx\n\n---\n\n" + rem + "\n",
			want:   "\n# T\n\n## Context\nx\n",
			stderr: "stop_removed=1 reminder_removed=1",
		},
		"no banner": {
			in:     "# T\n\n## Context\n" + snippet,
			want:   "# T\n\n## Context\n" + snippet,
			stderr: "stop_removed=0 reminder_removed=0",
		},
	} {
		f := filepath.Join(t.TempDir(), "plan.md")
		if err := os.WriteFile(f, []byte(c.in), 0o600); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		cmd := exec.Command(awkBin, string(m[1]), f)
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: awk: %v: %s", name, err, errb.String())
		}
		if out.String() != c.want {
			t.Errorf("%s: got\n%q\nwant\n%q", name, out.String(), c.want)
		}
		if got := strings.TrimSpace(errb.String()); got != c.stderr {
			t.Errorf("%s: stderr %q, want %q", name, got, c.stderr)
		}
	}
}
