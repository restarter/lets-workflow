package initcmd

import (
	"os"
	"os/exec"
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
	for _, c := range []struct{ file, what, start, end, trustGuard string }{
		{filepath.Join("commands", "review.md"), "review.md skeptic template",
			"**Skeptic prompt template.**", "**Asymmetric drop rule", "spec_trusted"},
		// Needle is the EXECUTABLE form, not the bare key: `specTrusted` also appears in the
		// comment above the expression, so deleting the guard would leave a bare-key needle green.
		{filepath.Join("skills", "review-workflow", "review.workflow.js"), "js specBlockSkeptic",
			"const specBlockSkeptic =", "const treeBlock =", "specTrusted === true"},
	} {
		body := squash(region(t, readPlugin(t, c.file), c.what, c.start, c.end))
		if !strings.Contains(body, "NEVER use the SPEC as grounds to refute a correctness") {
			t.Errorf("%s: lost the narrow wording - a skeptic must not refute a real bug on spec grounds", c.what)
		}
		if !strings.Contains(body, c.trustGuard) {
			t.Errorf("%s: lost %q - a PR-body spec is written by the author of the code being judged, and a skeptic's real=false deletes findings", c.what, c.trustGuard)
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
	// Both lookups are SCOPED, and the SKILL.md one matches the table ROW. A whole-file search for
	// `spec` went green after the `spec` row was deleted, because the word also appears in the
	// `specTrusted` row's prose - the exact trap this file's header says was designed out. Two of
	// the three keys were unique only by luck.
	w2 := region(t, readPlugin(t, filepath.Join("commands", "review.md")), "review.md W2 args",
		"### W2: Build args", "### W3:")
	skillMd := readPlugin(t, filepath.Join("skills", "review-workflow", "SKILL.md"))
	for _, k := range []string{"spec", "prTree", "specTrusted"} {
		if !regexp.MustCompile(`\b` + k + `\b`).MatchString(destructure) {
			t.Errorf("review.workflow.js: %q is not destructured from input", k)
		}
		if !strings.Contains(w2, k+":") {
			t.Errorf("commands/review.md: the W2 args block does not pass %q", k)
		}
		if !strings.Contains(skillMd, "| `"+k+"` |") {
			t.Errorf("skills/review-workflow/SKILL.md: the args table has no row for %q", k)
		}
	}
}

// TestReviewSpecBlockExists pins the branch's headline artifact in all THREE reviewer copies.
// Mutation-proved gap: deleting the BEGIN SPEC fence from review.md's Step 5, deleting the whole
// SCOPE vs SPEC paragraph, or setting `const specBlock = ”` each left the previous suite 4/4
// green - and check.md was read by no test at all, despite being one of the four copies.
func TestReviewSpecBlockExists(t *testing.T) {
	for _, c := range []struct{ file, what, start, end, payload string }{
		{filepath.Join("commands", "review.md"), "review.md Step 5 reviewer template",
			"### Task Prompt Template", "## Workflow Mode", "{spec}"},
		{filepath.Join("commands", "check.md"), "check.md Spec Alignment",
			"### Spec Alignment", "### Review Focus", "{spec}"},
		{filepath.Join("skills", "review-workflow", "review.workflow.js"), "js specBlock",
			"const specBlock = SPEC", "// The skeptic returns", "${SPEC}"},
	} {
		body := squash(region(t, readPlugin(t, c.file), c.what, c.start, c.end))
		for _, need := range []string{"BEGIN SPEC", "END SPEC", "SCOPE vs SPEC", c.payload} {
			if !strings.Contains(body, need) {
				t.Errorf("%s: lost %q - reviewers would run blind, which is the defect this branch fixes", c.what, need)
			}
		}
	}
	// The skeptic copies carry the payload too: TestSkepticSpecBlockIsNarrower pins their WORDING,
	// which stayed green in mutation while the {spec}/${SPEC} slot itself was deleted - leaving a
	// prompt that says "Use the SPEC ONLY when..." with no SPEC in it.
	for _, c := range []struct{ file, what, start, end, payload string }{
		{filepath.Join("commands", "review.md"), "review.md skeptic template",
			"**Skeptic prompt template.**", "**Asymmetric drop rule", "{spec}"},
		{filepath.Join("skills", "review-workflow", "review.workflow.js"), "js specBlockSkeptic",
			"const specBlockSkeptic =", "const treeBlock =", "${SPEC}"},
	} {
		if body := region(t, readPlugin(t, c.file), c.what, c.start, c.end); !strings.Contains(body, c.payload) {
			t.Errorf("%s: carries the SPEC instructions but not the %s slot they refer to", c.what, c.payload)
		}
	}
}

// TestReviewRestoreFenceIsPinned covers the half of the PR gate that gives the user their branch
// back. Its sibling (the half that TAKES the branch) has been pinned since the one-shell bug;
// this one was pinned by nothing, so deleting the whole step, gutting the ref, or replacing the
// by-sha pop with a bare `git stash pop` all passed. The bare form is the dangerous one:
// refs/stash is repo-global, so stash@{0} can be a parallel worktree's entry.
func TestReviewRestoreFenceIsPinned(t *testing.T) {
	var found bool
	for _, f := range bashFences(readPlugin(t, filepath.Join("commands", "review.md"))) {
		// Three fences touch `.review-restore-`: the SWITCH writes it (`mv -f "$tmp"`), the
		// stray-scan globs it, and this one loads it into $BR to act on. Select on the assignment.
		if !strings.Contains(f, `BR=$(sed`) {
			continue
		}
		found = true
		for _, need := range []string{`git checkout "$BR"`, `git stash pop "$IDX"`} {
			if !strings.Contains(f, need) {
				t.Errorf("review.md restore fence: lost %q", need)
			}
		}
		if regexp.MustCompile(`git stash pop\s*(\n|$|;|&&)`).MatchString(f) {
			t.Error("review.md restore fence: bare `git stash pop` - refs/stash is repo-global, so stash@{0} may belong to a parallel worktree")
		}
	}
	if !found {
		t.Fatal("review.md: no bash fence READS the .review-restore- state file - Step 6.7 gone?")
	}
}

// switchFence returns the Step 2.5 switch block with its two orchestrator placeholders resolved,
// ready to execute.
func switchFence(t *testing.T, stash, pr string) string {
	t.Helper()
	for _, f := range bashFences(readPlugin(t, filepath.Join("commands", "review.md"))) {
		if !strings.Contains(f, ".review-restore-") || !strings.Contains(f, `mv -f "$tmp"`) {
			continue
		}
		f = regexp.MustCompile(`(?m)^STASH=\{[^}]*\}$`).ReplaceAllString(f, "STASH="+stash)
		return strings.ReplaceAll(f, "{number}", pr)
	}
	t.Fatal("review.md: switch fence not found")
	return ""
}

// smokeRepo builds a throwaway repo with two PR branches, a stubbed `gh` that detaches onto
// prbranch<N>, and .lets/sessions/. Returns the repo path and the PATH prefix carrying the stub.
func smokeRepo(t *testing.T, dirty string) (repo, stub string) {
	t.Helper()
	repo = t.TempDir()
	stub = filepath.Join(repo, "stub")
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = repo, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "init", "-q", "-b", "main", ".")
	run("git", "config", "user.email", "t@example.com")
	run("git", "config", "user.name", "t")
	write("a.txt", "base\n", 0o644)
	run("git", "add", "-A")
	run("git", "commit", "-qm", "base")
	for _, br := range []string{"prbranch42", "prbranch43"} {
		run("git", "checkout", "-q", "-b", br, "main")
		write(br+".txt", br+"\n", 0o644)
		run("git", "add", "-A")
		run("git", "commit", "-qm", br)
	}
	run("git", "checkout", "-q", "main")
	if err := os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	switch dirty {
	case "tracked":
		write("a.txt", "base\nmine\n", 0o644)
	case "untracked":
		write("u.txt", "mine\n", 0o644)
	}
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	// `gh pr checkout --detach <N>` -> detach onto prbranch<N>. Last arg is the number.
	write(filepath.Join("stub", "gh"),
		"#!/bin/sh\nfor a in \"$@\"; do n=\"$a\"; done\ngit checkout --detach -q \"prbranch$n\"\n", 0o755)
	return repo, stub
}

// runFence executes the switch block in repo and returns its combined output.
func runFence(t *testing.T, repo, stub, stash, pr string) string {
	t.Helper()
	// `sh -c` is the point: the artifact under test IS a shell block, extracted from
	// committed markdown in this repo. No external input reaches it.
	cmd := exec.Command("sh", "-c", switchFence(t, stash, pr))
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"CLAUDE_CODE_SESSION_ID=testsession")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("switch fence exited %v\n%s", err, out)
	}
	return string(out)
}

// TestReviewSwitchFenceWritesRestoreState EXECUTES the switch block instead of grepping it.
//
// Every other guard here is text co-location, and text co-location is exactly what missed the bug
// that shipped: a `{...}` group whose last command was conditional exits 1 whenever nothing was
// stashed, so `&& mv` never ran and the restore record existed ONLY in the stash path - on a clean
// tree the review ended with HEAD left on the PR branch. TestReviewSwitchIsOneShell passed
// throughout. Running the block in three tree states catches that class; reading it did not.
func TestReviewSwitchFenceWritesRestoreState(t *testing.T) {
	for _, bin := range []string{"git", "sh"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	for _, c := range []struct{ name, stash, dirty string }{
		{"clean-tree", "no", ""},               // the DEFAULT path, and the one that was broken
		{"stashed", "yes", "tracked"},          //
		{"untracked-only", "yes", "untracked"}, // `git stash push` saves nothing yet exits 0
		{"commit-first", "no", ""},             // user picked "Commit first" -> STASH=no
	} {
		t.Run(c.name, func(t *testing.T) {
			repo, stub := smokeRepo(t, c.dirty)
			out := runFence(t, repo, stub, c.stash, "42")
			if !strings.Contains(out, "SWITCHED") {
				t.Fatalf("expected SWITCHED, got:\n%s", out)
			}
			state := filepath.Join(repo, ".lets", "sessions", ".review-restore-testsession-pr42")
			body, err := os.ReadFile(state)
			if err != nil {
				t.Fatalf("restore record missing - Step 6.7 would print \"nothing to restore\" and leave the user on the PR branch: %v\n%s", err, out)
			}
			for _, need := range []string{"ref: main", "pr: 42"} {
				if !strings.Contains(string(body), need) {
					t.Errorf("restore record lacks %q; got:\n%s", need, body)
				}
			}
			leftovers, _ := filepath.Glob(state + ".*")
			if len(leftovers) > 0 {
				t.Errorf("mktemp leftovers in .lets/sessions: %v", leftovers)
			}
		})
	}
}

// TestReviewSwitchKeepsAnEarlierRestoreRecord covers two PR reviews in ONE session, where the
// first one's restore did not finish (Step 2.5 keeps the record on any non-clean outcome, and a
// crash or an abort skips Step 6.7 entirely). With a session-only key the second review's
// `mv -f` overwrote that record, and since HEAD was by then detached at the first PR, `ref:`
// was rewritten to that detached sha - destroying the only route back to the user's branch.
func TestReviewSwitchKeepsAnEarlierRestoreRecord(t *testing.T) {
	for _, bin := range []string{"git", "sh"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	repo, stub := smokeRepo(t, "")
	sessions := filepath.Join(repo, ".lets", "sessions")

	runFence(t, repo, stub, "no", "42") // leaves HEAD detached at prbranch42, record kept
	runFence(t, repo, stub, "no", "43") // second review, same session

	first, err := os.ReadFile(filepath.Join(sessions, ".review-restore-testsession-pr42"))
	if err != nil {
		t.Fatalf("the first review's record was destroyed by the second: %v", err)
	}
	if !strings.Contains(string(first), "ref: main") {
		t.Errorf("the first record no longer points at the user's branch; got:\n%s", first)
	}
	second, err := os.ReadFile(filepath.Join(sessions, ".review-restore-testsession-pr43"))
	if err != nil {
		t.Fatalf("the second review wrote no record: %v", err)
	}
	if !strings.Contains(string(second), "pr: 43") {
		t.Errorf("the second record is not for PR 43; got:\n%s", second)
	}
}

// TestReviewWorkflowScriptParses runs the syntax check SKILL.md documents. A bare `node --check`
// on this file is a NO-OP - line 2 is `export`, which flips node to ESM detection and exits 0 even
// on an unterminated template literal, i.e. precisely the failure mode of its long backticked
// prompt strings. Nothing else in the repo runs node, so a syntax error here would surface only as
// every `--workflow` review dying at runtime.
func TestReviewWorkflowScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	src := readPlugin(t, filepath.Join("skills", "review-workflow", "review.workflow.js"))
	wrapped := "async function __w(){\n" +
		strings.ReplaceAll("\n"+src, "\nexport ", "\n") + "\n}\n"
	path := filepath.Join(t.TempDir(), "wrapped.js")
	if err := os.WriteFile(path, []byte(wrapped), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--check", path)
	cmd.Env = append(os.Environ(), "NODE_OPTIONS=") // an inherited --require preload breaks --check
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("review.workflow.js does not parse: %v\n%s", err, out)
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
