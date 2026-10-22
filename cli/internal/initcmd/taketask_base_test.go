package initcmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// mergeBranchCut matches a branch CUT whose start point is the merge branch:
// `checkout [opts] -b <branch> <start>` or `switch [opts] -c|--create <branch> <start>`
// with {LETS_MERGE_BRANCH} (or $LETS_MERGE_BRANCH) in <start>. A prose mention of the
// verb without a start point ("skip `git checkout -b`") is not a cut and does not match.
var mergeBranchCut = regexp.MustCompile(`(?:checkout\s+(?:-\S+\s+)*?-b|switch\s+(?:-\S+\s+)*?(?:-c|--create))\s+\S+\s+\S*(?:\{LETS_MERGE_BRANCH\}|\$\{?LETS_MERGE_BRANCH)`)

// takeTaskBranchHandler returns the start and end of take-task Step 4's **Branch**
// handler (from `- **Branch** ->` to the next top-level `- **` bullet), or -1, -1.
func takeTaskBranchHandler(md string) (int, int) {
	step := strings.Index(md, "### Step 4")
	if step < 0 {
		return -1, -1
	}
	stepEnd := len(md)
	if e := strings.Index(md[step+len("### Step 4"):], "\n### "); e >= 0 {
		stepEnd = step + len("### Step 4") + e
	}
	h := strings.Index(md[step:stepEnd], "- **Branch** ->")
	if h < 0 {
		return -1, -1
	}
	start := step + h
	end := stepEnd
	if e := strings.Index(md[start+1:stepEnd], "\n- **"); e >= 0 {
		end = start + 1 + e + 1
	}
	return start, end
}

// takeTaskBranchProblems lists what is wrong with take-task's main-checkout Branch
// cut (lets-qjt3h, Codex round 1): it resolves the base through Go, cuts the resolved
// sha with --no-track, and stops on a refusal - never a cut from the merge branch.
func takeTaskBranchProblems(md string) []string {
	start, end := takeTaskBranchHandler(md)
	if start < 0 {
		return []string{"no **Branch** handler in ### Step 4"}
	}
	handler := md[start:end]
	var probs []string
	for _, want := range []string{"`lets worktree base --json`", "--no-track -b <branch> <base_resolution.sha>", "`error.remediation`"} {
		if !strings.Contains(handler, want) {
			probs = append(probs, "missing "+want)
		}
	}
	stops := false
	for _, l := range strings.Split(handler, "\n") {
		stops = stops || (strings.Contains(l, "`ok=false`") && strings.Contains(l, "stop"))
	}
	if !stops {
		probs = append(probs, "no `ok=false` line that stops")
	}
	if mergeBranchCut.MatchString(handler) {
		probs = append(probs, "the handler cuts from the merge branch")
	}
	return probs
}

// mergeBranchCuts lists every line of every *.md under root that cuts a branch from
// the merge branch (path:line: text).
func mergeBranchCuts(t *testing.T, root string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, l := range strings.Split(string(b), "\n") {
			if mergeBranchCut.MatchString(l) {
				rel, _ := filepath.Rel(root, p)
				hits = append(hits, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(l))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestTakeTask_BranchCutFromResolvedBase(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "skills", "take-task", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	if p := takeTaskBranchProblems(md); len(p) != 0 {
		t.Fatalf("take-task Step 4 Branch: %v", p)
	}
	start, end := takeTaskBranchHandler(md)
	mutants := map[string]string{
		"the old local cut":      md[:start] + "- **Branch** -> `git checkout -b <branch> {LETS_MERGE_BRANCH}` (from LETS Config)\n" + md[end:],
		"no stop on ok=false":    strings.Replace(md, "`error.remediation`, stop: nothing was cut", "`error.remediation`, then go on", 1),
		"cut from HEAD, not sha": strings.Replace(md, "--no-track -b <branch> <base_resolution.sha>", "--no-track -b <branch>", 1),
		"base read by hand":      strings.Replace(md, "`lets worktree base --json`", "`git fetch`", 1),
	}
	for name, m := range mutants {
		if m == md {
			t.Errorf("mutant %q changed nothing", name)
			continue
		}
		if len(takeTaskBranchProblems(m)) == 0 {
			t.Errorf("mutant %q passed the lint", name)
		}
	}
}

// No spec cuts a branch from the merge branch: every new LETS branch is cut from the
// base Go resolves (lets-qjt3h) - the "no seventh path" pin.
func TestPluginSpecs_NoCutFromMergeBranch(t *testing.T) {
	if hits := mergeBranchCuts(t, pluginDir(t)); len(hits) != 0 {
		t.Errorf("a spec cuts a branch from the merge branch:\n%s", strings.Join(hits, "\n"))
	}
	for _, cut := range []string{
		"- **Branch** -> `git checkout -b <branch> {LETS_MERGE_BRANCH}` (from LETS Config)",
		"git switch -c x {LETS_MERGE_BRANCH}",
		"git checkout --no-track -b b origin/{LETS_MERGE_BRANCH}",
		`git switch --create x "$LETS_MERGE_BRANCH"`,
	} {
		if !mergeBranchCut.MatchString(cut) {
			t.Errorf("the matcher misses a cut from the merge branch: %s", cut)
		}
	}
	for _, prose := range []string{
		"- **Stay on current branch** -> skip `git checkout -b`; stay on the current branch (could be `$LETS_MERGE_BRANCH` or any pre-existing branch). Print one line:",
		"`ok=true` -> `git checkout --no-track -b <branch> <base_resolution.sha>`",
		"never fall back to the local `{LETS_MERGE_BRANCH}`.",
	} {
		if mergeBranchCut.MatchString(prose) {
			t.Errorf("the matcher flags a line that is not a cut from the merge branch: %s", prose)
		}
	}
}
