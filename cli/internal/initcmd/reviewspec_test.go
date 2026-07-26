package initcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The SPEC block is copied into four prompt templates - review.md's reviewer and
// skeptic templates, check.md's inline lens, and review.workflow.js's specBlock
// and specBlockSkeptic - because the Dynamic Workflow runtime forbids
// import/filesystem and the text cannot be shared.
//
// This file pins the STRUCTURAL invariants only: things prose cannot express and
// a reword cannot break. Sentence-level needles were tried in two earlier
// revisions and were worse than useless - they went green on real regressions
// (a string appearing twice in a file only proves one copy survived) while
// failing on harmless reflows.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/, so a plugin-markdown edit alone serves a stale PASS.

// squash collapses whitespace runs so a needle survives a hard wrap or a reflow.
var wsRun = regexp.MustCompile(`\s+`)

func squash(s string) string { return wsRun.ReplaceAllString(s, " ") }

func readPlugin(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(pluginDir(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// region returns src between the first `start` and the following `end`, and
// fails loudly on a stale anchor rather than silently widening to the whole file.
func region(t *testing.T, src, what, start, end string) string {
	t.Helper()
	i := strings.Index(src, start)
	if i < 0 {
		t.Fatalf("%s: start anchor %q not found - renamed? update this guard", what, start)
	}
	rest := src[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("%s: end anchor %q not found after %q", what, end, start)
	}
	return rest[:j]
}

// TestSkepticSpecBlockIsNarrower is the one that earns its keep. A skeptic
// returns {real, confidence, reason} and cannot set a tier, so a reviewer-style
// "cap it at SUGGESTION" would be executed with its only lever - real=false -
// which the drop rule turns into a silent DELETE of the finding. Both the
// markdown and the JS skeptic prompt must carry the narrow wording and must NOT
// carry the reviewer's cap.
func TestSkepticSpecBlockIsNarrower(t *testing.T) {
	for _, c := range []struct{ file, what, start, end string }{
		{filepath.Join("commands", "review.md"), "review.md skeptic template",
			"**Skeptic prompt template.**", "**Asymmetric drop rule"},
		{filepath.Join("skills", "review-workflow", "review.workflow.js"), "js specBlockSkeptic",
			"const specBlockSkeptic =", "const treeBlock ="},
	} {
		body := squash(region(t, readPlugin(t, c.file), c.what, c.start, c.end))
		if !strings.Contains(body, "NEVER use the SPEC as grounds to refute a correctness") {
			t.Errorf("%s: lost the narrow wording - a skeptic must not refute a real bug on spec grounds", c.what)
		}
		for _, banned := range []string{"cap it at", "SCOPE vs SPEC"} {
			if strings.Contains(body, banned) {
				t.Errorf("%s: carries the REVIEWER's %q - a skeptic executes a tier cap as real=false, i.e. a drop", c.what, banned)
			}
		}
	}
}

// funcBody returns a JS function's source so an interpolation check is scoped to
// ONE prompt builder.
func funcBody(t *testing.T, src, marker string) string {
	t.Helper()
	i := strings.Index(src, marker)
	if i < 0 {
		t.Fatalf("review.workflow.js: %q not found - renamed? update this guard", marker)
	}
	rest := src[i:]
	if j := strings.Index(rest, "\n}"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// TestWorkflowPromptsAreWired catches a block that is declared but never spliced
// in, and the reverse - the skeptic holding the reviewer's block. Also pins the
// args keys, which span three files: rename one and every --workflow review
// silently runs with an empty spec, with no other signal.
func TestWorkflowPromptsAreWired(t *testing.T) {
	js := readPlugin(t, filepath.Join("skills", "review-workflow", "review.workflow.js"))

	review := funcBody(t, js, "function reviewPrompt")
	skeptic := funcBody(t, js, "function skepticPrompt")
	for _, c := range []struct{ what, body, need string }{
		{"reviewPrompt", review, "${specBlock}"},
		{"reviewPrompt", review, "${treeBlock}"},
		{"skepticPrompt", skeptic, "${specBlockSkeptic}"},
		{"skepticPrompt", skeptic, "${treeBlock}"},
	} {
		if !strings.Contains(c.body, c.need) {
			t.Errorf("review.workflow.js: %s does not interpolate %s", c.what, c.need)
		}
	}
	if strings.Contains(skeptic, "${specBlock}") {
		t.Error("review.workflow.js: skepticPrompt interpolates the REVIEWER's specBlock")
	}

	destructure := regexp.MustCompile(`(?m)^const \{[^}]*\} = input`).FindString(js)
	if destructure == "" {
		t.Fatal("review.workflow.js: no `const { ... } = input` destructure found")
	}
	reviewMd := readPlugin(t, filepath.Join("commands", "review.md"))
	skillMd := readPlugin(t, filepath.Join("skills", "review-workflow", "SKILL.md"))
	for _, k := range []string{"spec", "prTree"} {
		if !regexp.MustCompile(`\b` + k + `\b`).MatchString(destructure) {
			t.Errorf("review.workflow.js: %q is not destructured from input", k)
		}
		if !strings.Contains(reviewMd, k+":") {
			t.Errorf("commands/review.md: W2 args block does not pass %q", k)
		}
		if !strings.Contains(skillMd, "`"+k+"`") {
			t.Errorf("skills/review-workflow/SKILL.md: args table does not document %q", k)
		}
	}
}

var bashFenceOpen = regexp.MustCompile("^```(?:bash|sh|shell|zsh|console)\\b")

// bashFences returns the body of every bash-family fence in src.
func bashFences(src string) []string {
	var out []string
	var cur strings.Builder
	in := false
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			} else if bashFenceOpen.MatchString(strings.TrimSpace(line)) {
				in = true
			}
			continue
		}
		if in {
			cur.WriteString(line + "\n")
		}
	}
	return out
}

// TestReviewSwitchIsOneShell pins the shape that cost three rounds of the same
// bug: recording the restore ref, stashing, checking out and unwinding must live
// in ONE bash fence. Each Bash tool call is a fresh shell (CLAUDE.md, "Surface
// forms"), so splitting them drops $F and $SH and the user's stash is stranded.
func TestReviewSwitchIsOneShell(t *testing.T) {
	var found bool
	for _, f := range bashFences(readPlugin(t, filepath.Join("commands", "review.md"))) {
		// The SWITCH block WRITES the state file; Step 6.7's block only reads it.
		if !strings.Contains(f, ".review-restore-") || !strings.Contains(f, `mv -f "$tmp"`) {
			continue
		}
		found = true
		for _, need := range []string{"git stash push", "gh pr checkout", `[ "$AFTER" != "$BEFORE" ]`, `rm -f "$F"`} {
			if !strings.Contains(f, need) {
				t.Errorf("review.md: %q is in a DIFFERENT bash fence than the restore-state write - a fresh shell loses $F/$SH and the stash is stranded", need)
			}
		}
	}
	if !found {
		t.Fatal("review.md: no bash fence writes the .review-restore- state file")
	}
}

var worktreeMention = regexp.MustCompile(`(?i)(git worktree|lets worktree|worktree add|worktree create)`)

// TestReviewNeverCreatesWorktree pins the design boundary: /lets:review reviews
// where it was launched. An earlier revision materialized the PR in a throwaway
// worktree. Scanned in bash fences only - policing prose failed on benign edits.
func TestReviewNeverCreatesWorktree(t *testing.T) {
	body := readPlugin(t, filepath.Join("commands", "review.md"))
	var scanned int
	for _, f := range bashFences(body) {
		for i, line := range strings.Split(f, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			scanned++
			if worktreeMention.MatchString(line) {
				t.Errorf("commands/review.md: bash line %d creates a worktree - review must never create one: %s", i+1, trimmed)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("commands/review.md: scanned 0 bash lines - fence detection broken")
	}
	if n := strings.Count(body, "NEVER create a git worktree here"); n != 1 {
		t.Errorf("commands/review.md: expected exactly 1 worktree prohibition, found %d", n)
	}
}
