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

// harnessRefusedBasename is Claude Code's Write-tool rule (2.1.x validateInput,
// telemetry tengu_subagent_md_report_blocked): a SUBAGENT may not Write a .md whose
// basename starts with report / summary / findings / analysis, in any case. A
// REPORT_FILE named that way is refused and the report is lost - only the lossy
// final text is left (lets-jyjxc).
var harnessRefusedBasename = regexp.MustCompile(`(?i)^(report|summary|findings|analysis)`)

// retrySlot is agent-report's retry suffix (Step 3 re-dispatches as {name}-r2): no
// caller's name may end in it.
var retrySlot = regexp.MustCompile(`-r([0-9]+|\{[^}]*\})$`)

var (
	reportPathRe  = regexp.MustCompile(`\.lets/[A-Za-z0-9_./{}+-]*\.md`)
	reportNamesRe = regexp.MustCompile("names=([^\\s\"`]+)")
	// freshNameRe is the prose form a repeat dispatch names its fresh report name in:
	// "a fresh name (`explorer-n{n}`" / "fresh names: `{agent}-challenge-2`".
	freshNameRe = regexp.MustCompile("fresh names?:? \\(?`([^`]+)`")
)

// reportNameProblems checks the report path and name templates of one plugin file.
// A report path is any .lets/...md on a line that hands out a report file
// (REPORT_FILE: / report-file=); a report name is any names= entry of an
// agent-report call, and the fresh name a repeat dispatch states in prose. Both
// get both checks: a path's basename without .md is a name.
func reportNameProblems(rel, text string) []string {
	var p []string
	check := func(at, what, name string) {
		if harnessRefusedBasename.MatchString(name) {
			p = append(p, at+": report "+what+" "+name+" starts with a word Claude Code refuses for a subagent's .md")
		}
		if rel != "skills/agent-report/SKILL.md" && retrySlot.MatchString(name) {
			p = append(p, at+": report "+what+" "+name+" ends in agent-report's reserved retry suffix -r<digits>")
		}
	}
	for i, line := range strings.Split(text, "\n") {
		at := rel + ":" + strconv.Itoa(i+1)
		if strings.Contains(line, "REPORT_FILE:") || strings.Contains(line, "report-file=") {
			for _, path := range reportPathRe.FindAllString(line, -1) {
				base := strings.TrimSuffix(filepath.Base(path), ".md")
				check(at, "file", base)
				if strings.HasPrefix(path, ".lets/cache/") && strings.HasPrefix(base, "{") {
					p = append(p, at+": report file "+path+" starts with a placeholder - its value could start with a refused word; lead with a literal")
				}
			}
		}
		var names []string
		if strings.Contains(line, "lets:agent-report") {
			for _, m := range reportNamesRe.FindAllStringSubmatch(line, -1) {
				names = append(names, strings.Split(m[1], ",")...)
			}
		}
		for _, m := range freshNameRe.FindAllStringSubmatch(line, -1) {
			names = append(names, m[1])
		}
		for _, name := range names {
			name = strings.TrimRight(name, "\")`")
			if name == "" || strings.HasPrefix(name, "<") || strings.Count(name, "{") != strings.Count(name, "}") {
				continue // a prose placeholder: <every name to spawn>, {comma-separated agent short names}
			}
			check(at, "name", name)
		}
	}
	return p
}

// TestReportFileNames walks every plugin markdown file: no report path or
// agent-report name may produce a basename Claude Code refuses for a subagent, and
// no caller's name may take agent-report's retry slot.
func TestReportFileNames(t *testing.T) {
	root := pluginDir(t)
	var problems []string
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		scanned++
		problems = append(problems, reportNameProblems(filepath.ToSlash(rel), string(b))...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("no plugin markdown scanned")
	}
	for _, p := range problems {
		t.Error(p)
	}

	// Lines that must stay clean: an argument label is not a name, and the renamed
	// shapes pass.
	for _, ok := range []string{
		"- `report-file` - an absolute path the CALLER named (execute: its pinned `.lets/cache/run-...` path; team: `agent-report op=open` / `op=add`).",
		"`Skill(skill: \"lets:agent-report\", args: \"op=add dir=<dir> names=member-{name}-n{k}\")`",
		"(`report-file=` `.lets/cache/run-{TASK_ID}-{RUN}-{chunk}-{member}-n{round}.md`, then a nudge `.lets/cache/nudge-{TASK_ID}-{RUN}-{chunk}-{member}-n{round}.md`)",
	} {
		if got := reportNameProblems("commands/clean.md", ok); len(got) > 0 {
			t.Errorf("a clean line was flagged: %q -> %v", ok, got)
		}
	}

	for _, m := range []struct{ name, text, want string }{
		{"execute regains the report- prefix", "REPORT_FILE: {absolute path of .lets/cache/report-{TASK_ID}-{RUN}-{chunk}-g{generation}-n{round}.md}", "Claude Code refuses"},
		{"a summary report path", "the member-run call passes `report-file=` `.lets/cache/Summary-{TASK_ID}.md`", "Claude Code refuses"},
		{"a cache report leads with a placeholder", "REPORT_FILE: {absolute path of .lets/cache/{TASK_ID}-{RUN}-{chunk}-n{round}.md}", "placeholder"},
		{"an agent-report name takes the retry slot", "`Skill(skill: \"lets:agent-report\", args: \"op=peek dir={abs .lets/cache} names=run-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round}\")`", "reserved retry suffix"},
		{"a re-explore takes the retry slot", "- **Re-explore** -> `op=add` a fresh name (`explorer-r{n}`; then op=collect", "reserved retry suffix"},
		{"a path-only template takes the retry slot", "with a `REPORT_FILE:` line (`report-file=` the absolute `.lets/cache/run-{TASK_ID}-{RUN}-{chunk}-architect-r{round}.md`", "reserved retry suffix"},
		{"an agent-report name starts with findings", "`Skill(skill: \"lets:agent-report\", args: \"op=add dir={REPORT_DIR} names=findings-a,architect\")`", "Claude Code refuses"},
	} {
		t.Run(m.name, func(t *testing.T) {
			got := strings.Join(reportNameProblems("commands/mutant.md", m.text), "\n")
			if !strings.Contains(got, m.want) {
				t.Errorf("mutant produced no problem containing %q - that guard cannot fail:\n%s", m.want, got)
			}
		})
	}
}
