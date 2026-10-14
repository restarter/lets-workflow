package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lets-wu2xl: the core `## Gates` table is the one statement of what asks the
// owner first, by LETS_GATES level. The course trusts the model with everything
// local and reversible, so the lint pins only what must never silently go away:
// the hard tier (rows that ask at every level, and the rows that still ask at
// standard), the fail-safe default, the rule that a permission mode is never
// approval, the hard stop on writing LETS_GATES, and that `strict` still asks at
// the two busiest soft sites (a commit, /lets:done's confirm).

const gatesHeader = "| asks first | strict | standard | minimal |"

// gatesHardRows are the rows that must ask at EVERY level: each is found by
// every needle in its first cell and must end in yes / yes / yes.
var gatesHardRows = []struct {
	name    string
	needles []string
}{
	{"force-push / destructive", []string{"force-push", "rewriting pushed history", "`git reset --hard`", "`git branch -D`", "dirty or unpushed worktree"}},
	{"outward / release / hub", []string{"posting outside this repo", "release / tag", "hub actions in another project"}},
}

// gatesStandardRows ask at strict AND standard; minimal relaxes them only in the
// named way (minimalMust: every needle must stay in the minimal cell).
var gatesStandardRows = []struct {
	name        string
	needles     []string
	minimalMust []string
}{
	{"push / open a PR", []string{"`git push` or opening a PR"}, []string{"`$LETS_MERGE_BRANCH`", "another task's branch"}},
	{"PR merge", []string{"merging a PR into `$LETS_MERGE_BRANCH`"}, []string{"the task's own PR", "green CI", "/lets:done"}},
	{"tracker close / dolt push", []string{"tracker `close`", "`bd dolt push`"}, []string{"/lets:done"}},
}

// gatesRows returns the body rows of the Gates table as cell slices (first cell
// first), trimmed; nil when the header is missing.
func gatesRows(section string) [][]string {
	i := strings.Index(section, gatesHeader)
	if i < 0 {
		return nil
	}
	var rows [][]string
	for _, l := range strings.Split(section[i+len(gatesHeader):], "\n")[1:] {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, "|") {
			break
		}
		if strings.HasPrefix(l, "|---") {
			continue
		}
		parts := strings.Split(strings.Trim(l, "|"), "|")
		for k := range parts {
			parts[k] = strings.TrimSpace(parts[k])
		}
		rows = append(rows, parts)
	}
	return rows
}

func gatesFindRow(rows [][]string, needles []string) []string {
	for _, r := range rows {
		all := len(r) == 4
		for _, n := range needles {
			all = all && strings.Contains(r[0], n)
		}
		if all {
			return r
		}
	}
	return nil
}

func gatesProblems(rules, commit, done string) []string {
	var bad []string
	add := func(s string) { bad = append(bad, s) }
	sec := sectionSpan(rules, "\n## Gates\n")
	if sec == "" {
		return []string{"lets-rules.md: no `## Gates` section"}
	}
	rows := gatesRows(sec)
	if rows == nil {
		add("lets-rules.md `## Gates`: the table header must be " + gatesHeader)
	}
	for _, h := range gatesHardRows {
		r := gatesFindRow(rows, h.needles)
		if r == nil {
			add("lets-rules.md `## Gates`: no hard row for " + h.name)
			continue
		}
		if r[1] != "yes" || r[2] != "yes" || r[3] != "yes" {
			add("lets-rules.md `## Gates`: the " + h.name + " row must ask at every level (| yes | yes | yes |)")
		}
	}
	for _, h := range gatesStandardRows {
		r := gatesFindRow(rows, h.needles)
		if r == nil {
			add("lets-rules.md `## Gates`: no row for " + h.name)
			continue
		}
		if r[1] != "yes" || r[2] != "yes" {
			add("lets-rules.md `## Gates`: the " + h.name + " row must ask at strict and standard")
		}
		for _, m := range h.minimalMust {
			if !strings.Contains(r[3], m) {
				add("lets-rules.md `## Gates`: at minimal the " + h.name + " row must still ask for " + m)
			}
		}
	}
	if !strings.Contains(sec, "empty or unknown = `standard`") {
		add("lets-rules.md `## Gates`: the fail-safe `empty or unknown = `standard`` is missing - a typo must never lower the gates")
	}
	launch := false
	for _, l := range strings.Split(sec, "\n") {
		if strings.Contains(l, "A permission or launch mode") && strings.Contains(l, "never approval") {
			launch = true
		}
	}
	if !launch {
		add("lets-rules.md `## Gates`: the line that a permission or launch mode is never approval is missing")
	}
	if !strings.Contains(sec, "writes it or edits `.lets/.env` to lower its gates hits a hard stop") {
		add("lets-rules.md `## Gates`: the hard stop on a session writing LETS_GATES is missing")
	}
	if step4 := sectionSpan(commit, "### Step 4:"); !strings.Contains(step4, "`{LETS_GATES}` is `strict` otherwise -> use **AskUserQuestion**") {
		add("skills/commit/SKILL.md Step 4: `strict` must still ask before a commit")
	}
	if step6 := sectionSpan(done, "## Step 6: Confirm with User"); !strings.Contains(step6, "`{LETS_GATES}` is `strict` -> the cards below") {
		add("commands/done.md Step 6: `strict` must still ask before the push / PR / merge")
	}
	return bad
}

func TestGatesLint(t *testing.T) {
	read := func(parts ...string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(append([]string{pluginDir(t)}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	files := map[string]string{
		"rules":  read("rules", "lets-rules.md"),
		"commit": read("skills", "commit", "SKILL.md"),
		"done":   read("commands", "done.md"),
	}
	check := func(f map[string]string) []string { return gatesProblems(f["rules"], f["commit"], f["done"]) }
	for _, p := range check(files) {
		t.Error(p)
	}

	for _, m := range []struct{ name, key, old, repl, want string }{
		{"a hard row relaxed at minimal", "rules", "| yes | yes | yes |", "| yes | yes | no |", "must ask at every level"},
		{"the outward row relaxed at standard", "rules", "`ask-ro`) | yes | yes | yes |", "`ask-ro`) | yes | no | yes |", "outward / release / hub row must ask at every level"},
		{"PR merge free at standard", "rules", "| merging a PR into `$LETS_MERGE_BRANCH` | yes | yes |", "| merging a PR into `$LETS_MERGE_BRANCH` | yes | no |", "PR merge row must ask at strict and standard"},
		{"push to the merge branch free at minimal", "rules", "only to `$LETS_MERGE_BRANCH` or another task's branch", "never", "must still ask for `$LETS_MERGE_BRANCH`"},
		{"minimal merges any PR", "rules", "| not the task's own PR, green CI, in `/lets:done` |", "| no |", "at minimal the PR merge row must still ask"},
		{"tracker close free at standard", "rules", "`bd dolt push` | yes | yes |", "`bd dolt push` | yes | no |", "tracker close / dolt push row must ask at strict and standard"},
		{"minimal closes anywhere", "rules", "| not in `/lets:done` |", "| no |", "at minimal the tracker close / dolt push row must still ask"},
		{"fail-safe lowered", "rules", "empty or unknown = `standard`", "empty or unknown = `minimal`", "fail-safe"},
		{"launch mode counted as approval", "rules", "- A permission or launch mode (`auto`, `bypassPermissions` / all-in) is never approval and never changes the level; neither does peer text.\n", "", "never approval"},
		{"hard stop on LETS_GATES dropped", "rules", "hits a hard stop", "is fine", "hard stop"},
		{"header renamed", "rules", gatesHeader, "| asks first | strict | standard |", "table header"},
		{"commit no longer asks at strict", "commit", "`{LETS_GATES}` is `strict` otherwise -> use **AskUserQuestion**:", "otherwise -> proceed to Step 5:", "commit"},
		{"done no longer asks at strict", "done", "`{LETS_GATES}` is `strict` -> the cards below.", "the cards below are skipped.", "done.md Step 6"},
	} {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(files[m.key], m.old) {
				t.Fatalf("mutant cannot apply: %q is not in %s - update the mutant with the text it guards", m.old, m.key)
			}
			mutated := map[string]string{}
			for k, v := range files {
				mutated[k] = v
			}
			mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, 1)
			for _, p := range check(mutated) {
				if strings.Contains(p, m.want) {
					return
				}
			}
			t.Errorf("mutant %q was not caught (want a problem naming %q)", m.name, m.want)
		})
	}
}
