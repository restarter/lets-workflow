package initcmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pluginTexts reads every regular file under plugins/lets/, keyed by its path
// relative to the plugin root. A walk error fails the test, and so does a walk that
// found no markdown at all (a wrong root would otherwise pass every check).
func pluginTexts(t *testing.T) map[string]string {
	t.Helper()
	root := pluginDir(t)
	files := map[string]string{}
	md := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		files[filepath.ToSlash(rel)] = string(b)
		if strings.HasSuffix(path, ".md") {
			md++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if md == 0 {
		t.Fatalf("no .md file under %s - wrong plugin root", root)
	}
	return files
}

// teamD6Legacy is the one line allowed to name `lets members`: team.md D6 keeps the
// retired registry's file as history.
const teamD6Legacy = "any legacy `.lets/execution/members-<c>.json` the retired `lets members` wrote, never read"

// noLetsMembersProblems lists every plugin line that still names `lets members`
// (retired in lets-gbfpd), except team.md's D6 legacy-file line.
func noLetsMembersProblems(files map[string]string) []string {
	var p []string
	for path, text := range files {
		for i, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, "lets members") {
				continue
			}
			if path == "commands/team.md" && strings.Contains(line, teamD6Legacy) {
				continue
			}
			p = append(p, path+":"+strconv.Itoa(i+1)+": names the retired `lets members`")
		}
	}
	return p
}

// noSessionProofProblems lists every plugin file that names session_proof - it is
// written only by Go; protocol/worktrees.md names it once, as Go-written.
func noSessionProofProblems(files map[string]string) []string {
	var p []string
	for path, text := range files {
		if !strings.Contains(text, "session_proof") {
			continue
		}
		if path == "protocol/worktrees.md" {
			if !strings.Contains(text, "written only by Go") || !strings.Contains(text, "never by markdown") {
				p = append(p, path+": session_proof must be named as written only by Go, never by markdown")
			}
			continue
		}
		p = append(p, path+": a plugin file names session_proof - only Go writes it")
	}
	return p
}

func TestNoLetsMembersInPlugin(t *testing.T) {
	files := pluginTexts(t)
	for _, problem := range noLetsMembersProblems(files) {
		t.Error(problem)
	}
	if !strings.Contains(files["commands/team.md"], teamD6Legacy) {
		t.Error("team.md D6 must keep the legacy members file as history")
	}
	mutated := map[string]string{}
	for k, v := range files {
		mutated[k] = v
	}
	mutated["skills/member-run/SKILL.md"] += "\nlets members status --scope x --json\n"
	if len(noLetsMembersProblems(mutated)) == 0 {
		t.Error("an injected `lets members` call must fail the check")
	}
}

func TestNoSessionProofInPlugin(t *testing.T) {
	files := pluginTexts(t)
	for _, problem := range noSessionProofProblems(files) {
		t.Error(problem)
	}
	if !strings.Contains(files["protocol/worktrees.md"], "session_proof") {
		t.Error("protocol/worktrees.md must list session_proof in the task-state fields")
	}
	mutated := map[string]string{}
	for k, v := range files {
		mutated[k] = v
	}
	mutated["skills/take-task/SKILL.md"] += "\necho \"session_proof: $SID 1 1790797661911\" >> \"$TASK_FILE\"\n"
	if len(noSessionProofProblems(mutated)) == 0 {
		t.Error("a markdown write of session_proof must fail the check")
	}
}

// TestWorktreesProtocol_TaskStateFields pins the task-state table of
// protocol/worktrees.md to the keys Go writes: orchestrator: (lets-0xsw2, merge-branch
// only, repeatable) and session_proof: (lets-gbfpd) beside the single-valued keys.
func TestWorktreesProtocol_TaskStateFields(t *testing.T) {
	wt := pluginTexts(t)["protocol/worktrees.md"]
	if p := taskStateFieldProblems(wt); len(p) > 0 {
		t.Errorf("protocol/worktrees.md: %v", p)
	}
	row := "| `orchestrator: <name>[ scope=<part>][ standing=on\\|off]` | merge-branch file only, repeatable"
	if !strings.Contains(wt, row) {
		t.Fatal("mutant cannot apply: the orchestrator: row is missing")
	}
	if len(taskStateFieldProblems(strings.Replace(wt, row, "| `orc2:` | x", 1))) == 0 {
		t.Error("mutation: a table without the orchestrator: row must fail the check")
	}
}

func taskStateFieldProblems(wt string) []string {
	var bad []string
	for _, key := range []string{"| `task:` |", "| `start:` |", "| `session: <sha> <sid>` |", "| `origin:` |", "| `orc:` |", "| `orchestrator: <name>[ scope=<part>][ standing=on\\|off]` | merge-branch file only, repeatable", "| `session_proof: <sid> <pid> <started-ms>` |"} {
		if !strings.Contains(wt, key) {
			bad = append(bad, "missing row "+key)
		}
	}
	return bad
}
