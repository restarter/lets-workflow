package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// lets-0g49b: /lets:done asked GitHub only for a MERGED PR by branch name, so an
// open PR was missed (Step 8 opened a second one) and a branch reused after its
// PR merged was closed unshipped; Bitbucket had no guard at all. The
// Existing-PR Guard asks either forge for every PR of the branch whose head is
// the repository `git push origin` updates, and decides by commits.
// donePRProblems pins what the guard DECIDES - each decision row's condition and
// action cells by its Outcome cell, every state, every page and the push-remote
// identity on both forges, STOP when a lookup fails or comes back incomplete or
// the outcome / PR number changed before the push, the ancestor test against the
// PR's own head, an open PR never re-created or force-pushed, the shipped close,
// the open PR reaching Step 9, no second PR, the trunk skip. Every mutant below
// is a regression of a real lets-0g49b symptom or acceptance criterion, and must
// be caught by its own check. region comes from roles_lint_test.go.

const (
	donePRGuard      = "Existing-PR Guard"
	doneGuardHead    = "\n## Existing-PR Guard\n"
	doneTrunkHead    = "\n## Trunk-mode Routing\n"
	doneStep3Head    = "\n## Step 3: "
	doneShippedHead  = "\n### If the Existing-PR Guard's outcome is `shipped`:"
	doneGitHubFinish = "\n### If $LETS_PR_FLOW == github (PR flow):"
	doneBbFinish     = "\n### If $LETS_PR_FLOW == bitbucket (PR flow):"
	doneStep9Head    = "\n## Step 9: Output\n"
	doneRulesHead    = "\n## Rules\n"
	doneTableHead    = "| PRs of this branch | Outcome | Then |"
	doneRecheck      = "Existing-PR Guard's lookup"
	doneStop         = "STOP before any push"
)

var (
	doneOldGuardNames = []string{"Already-Merged Guard", "merged-PR shortcut"}
	doneAncestorSpan  = regexp.MustCompile("`([^`\n]*--is-ancestor[^`\n]*)`")
	doneNeverCreate   = regexp.MustCompile(`never[^.\n]*create`)
	doneNeverForce    = regexp.MustCompile(`never[^.\n]*--force`)
	// each decision row, keyed by its Outcome cell: what its condition cell must say and
	// must not, and what its Then cell must do
	doneRowNeeds = []struct {
		outcome                     string
		condMust, condNot, thenMust []string
	}{
		{"`open`", []string{"Exactly one open PR", "`{LETS_MERGE_BRANCH}`"}, nil, []string{"never opens a second"}},
		{"stop", []string{"More than one open PR", "another base"}, nil, []string{"Never open a second PR", "never force-push"}},
		{"`shipped`", []string{"merged", "holds HEAD", "`{LETS_MERGE_BRANCH}`"}, []string{"declined", "cannot", "does not hold", "another base"}, nil},
		{"`fresh`", []string{"declined", "another base", "does not hold HEAD", "cannot be resolved"}, nil, nil},
	}
	// each forge's lookup bullet must enumerate completely, in its own terms
	doneComplete = map[string]string{"github": "equal to the limit", "bitbucket": "every page"}
)

// doneRow is one decision row split into its condition and Then cells.
type doneRow struct{ cond, then string }

// doneDecisionRows returns the guard's decision rows - condition (c[1]) and Then
// (c[3]) cells - keyed by the first word of their Outcome cell (c[2]), and those
// keys in table order.
func doneDecisionRows(guard string) (map[string]doneRow, []string) {
	rows, order := map[string]doneRow{}, []string{}
	for _, l := range strings.Split(region(guard, doneTableHead, "\n\n"), "\n") {
		c := strings.Split(l, "|")
		if len(c) < 5 || strings.HasPrefix(strings.TrimSpace(c[2]), "---") {
			continue
		}
		if o := strings.Fields(strings.TrimSpace(c[2])); len(o) > 0 {
			rows[o[0]] = doneRow{cond: c[1], then: c[3]}
			order = append(order, o[0])
		}
	}
	return rows, order
}

// doneParagraph returns the first blank-line-separated paragraph of s holding every needle ("" when none).
func doneParagraph(s string, needles ...string) string {
	for _, p := range strings.Split(s, "\n\n") {
		all := true
		for _, n := range needles {
			if !strings.Contains(p, n) {
				all = false
				break
			}
		}
		if all {
			return p
		}
	}
	return ""
}

// doneLines returns the lines of s that start with prefix (after indentation) and contain sub.
func doneLines(s, prefix, sub string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) && strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return out
}

func doneIndexOf(order []string, s string) int {
	for i, o := range order {
		if o == s {
			return i
		}
	}
	return -1
}

func donePRProblems(done string) []string {
	const f = "commands/done.md: "
	var bad []string
	guard := region(done, doneGuardHead, "\n## ")
	if guard == "" {
		return []string{f + "no `## " + donePRGuard + "` section"}
	}
	// before Step 3 - so before every gate and every push
	if g, s := strings.Index(done, doneGuardHead), strings.Index(done, doneStep3Head); s < 0 || g > s {
		bad = append(bad, f+"the guard must come before Step 3")
	}
	// (vi) trunk-mode skips it, and the routing table says so under the guard's name
	if first := strings.SplitN(strings.TrimSpace(guard), "\n\n", 2)[0]; !strings.Contains(first, "Skip this entire guard") || !strings.Contains(first, "`$LETS_MERGE_BRANCH`") {
		bad = append(bad, f+"trunk skip: the guard's first paragraph must skip it when HEAD == `$LETS_MERGE_BRANCH`")
	}
	trunk := doneLines(region(done, doneTrunkHead, "\n## "), "-", donePRGuard)
	if len(trunk) == 0 {
		bad = append(bad, f+"trunk skip: Trunk-mode Routing must name the "+donePRGuard)
	}
	for _, l := range trunk {
		if !strings.Contains(l, "**skip**") {
			bad = append(bad, f+"trunk skip: Trunk-mode Routing must skip the guard")
		}
	}
	// (ii) each forge is asked for every state, every page, and only for PRs whose head
	// is the repository `git push origin` updates - a merged-only or truncated lookup
	// misses the open PR; a same-repository flag is relative to the forge's default
	// repository, which can differ from origin
	for _, forge := range []string{"github", "bitbucket"} {
		lines := doneLines(guard, "- `"+forge+"`", "")
		if len(lines) == 0 {
			bad = append(bad, f+"all states: the guard has no "+forge+" lookup")
		}
		for _, l := range lines {
			narrowed := false
			for _, n := range []string{"--state merged", "--state open", "merged only", "OPEN only"} {
				narrowed = narrowed || strings.Contains(l, n)
			}
			asksAll := strings.Contains(l, "all states") || strings.Contains(l, "every state")
			if narrowed || !asksAll {
				bad = append(bad, f+"all states: the "+forge+" lookup must ask for every PR state")
			}
			if !strings.Contains(l, "git push origin") {
				bad = append(bad, f+"same repository: the "+forge+" lookup must keep only PRs whose head is the repository git push origin updates")
			}
			if !strings.Contains(l, "get-url --push origin") {
				bad = append(bad, f+"push url: the "+forge+" lookup must take origin's owner/name from its push URL (git remote get-url --push origin), not the fetch URL")
			}
			if strings.Contains(l, "isCrossRepository") {
				bad = append(bad, f+"push identity: the "+forge+" lookup must bind the head repository to origin, not to the forge's default repository (isCrossRepository)")
			}
			if !strings.Contains(l, doneComplete[forge]) {
				bad = append(bad, f+"complete: the "+forge+" lookup must enumerate every PR ("+doneComplete[forge]+")")
			}
		}
	}
	// (iii) a lookup that ran and failed, came back incomplete or cannot tell a PR's
	// source repository - or origin has more than one push URL - stops; it never reads
	// as "no PR"
	errRows := doneLines(guard, "|", "ran and failed")
	if len(errRows) == 0 {
		bad = append(bad, f+"lookup error: the forge table has no failed-lookup row")
	}
	for _, r := range errRows {
		if !strings.Contains(r, doneStop) || strings.Contains(r, "continue") || !strings.Contains(r, "incomplete") || !strings.Contains(r, "source repository") || !strings.Contains(r, "more than one push URL") {
			bad = append(bad, f+"lookup error: a failed, incomplete or source-unknown lookup, or origin with more than one push URL, must "+doneStop+" and never continue")
		}
	}
	// (iv) commits, not names: holds HEAD is HEAD into the PR head, never the merge-branch
	if doneParagraph(guard, "**holds HEAD**", "--is-ancestor HEAD <pr-head>") == "" {
		bad = append(bad, f+"holds HEAD: its definition must be `--is-ancestor HEAD <pr-head>`")
	}
	for _, m := range doneAncestorSpan.FindAllStringSubmatch(guard, -1) {
		if strings.Contains(m[1], "MERGE_BRANCH") || strings.Contains(m[1], "origin/") {
			bad = append(bad, f+"merge-branch: an ancestor test names the merge branch: "+m[1])
		}
	}
	// (i) each decision row decides what its outcome says; open wins by row order
	rows, order := doneDecisionRows(guard)
	for _, n := range doneRowNeeds {
		row, ok := rows[n.outcome]
		if !ok {
			bad = append(bad, f+"decision: no "+n.outcome+" row")
			continue
		}
		for _, s := range n.condMust {
			if !strings.Contains(row.cond, s) {
				bad = append(bad, f+"decision: the "+n.outcome+" condition must say "+s)
			}
		}
		for _, s := range n.condNot {
			if strings.Contains(row.cond, s) {
				bad = append(bad, f+"decision: the "+n.outcome+" condition must not cover "+s)
			}
		}
		for _, s := range n.thenMust {
			if !strings.Contains(row.then, s) {
				bad = append(bad, f+"decision: the "+n.outcome+" action must say "+s)
			}
		}
	}
	if o, s := doneIndexOf(order, "`open`"), doneIndexOf(order, "`shipped`"); o < 0 || s < 0 || o > s {
		bad = append(bad, f+"decision: the `open` row must come before `shipped` - an open PR wins")
	}
	// (v) Step 8 on both forges: re-check before the first push - same outcome AND same PR
	// number, else STOP; open never re-created or forced
	for _, s := range []struct{ name, head string }{{"github", doneGitHubFinish}, {"bitbucket", doneBbFinish}} {
		sec := region(done, s.head, "\n### ")
		if sec == "" {
			bad = append(bad, f+"Step 8 has no "+s.name+" PR flow")
			continue
		}
		open := doneParagraph(sec, "Outcome `open`")
		if !doneNeverCreate.MatchString(open) {
			bad = append(bad, f+"open creates: Step 8 "+s.name+" must never create a PR on the `open` outcome")
		}
		if !doneNeverForce.MatchString(open) {
			bad = append(bad, f+"force: Step 8 "+s.name+" must never --force the push to an open PR")
		}
		r, p := strings.Index(sec, doneRecheck), strings.Index(sec, "git push")
		if r < 0 || p < 0 || r > p || !strings.Contains(doneParagraph(sec, doneRecheck), doneStop) {
			bad = append(bad, f+"re-check: Step 8 "+s.name+" must repeat the lookup before the first push and "+doneStop+" on a change")
		}
		if !strings.Contains(doneParagraph(sec, doneRecheck), "same PR number") {
			bad = append(bad, f+"re-check identity: Step 8 "+s.name+" must compare the PR number, not only the outcome")
		}
		// a fresh github PR names its base: without --base gh falls back to the repository
		// default, and the guard's next run stops on its own PR as "another base"
		if s.name == "github" {
			creates := doneLines(sec, "gh pr create", "")
			if len(creates) == 0 {
				bad = append(bad, f+"fresh base: Step 8 github has no gh pr create")
			}
			for _, l := range creates {
				if !strings.Contains(l, `--base "{LETS_MERGE_BRANCH}"`) {
					bad = append(bad, f+"fresh base: Step 8 github's gh pr create must name --base \"{LETS_MERGE_BRANCH}\"")
				}
			}
		}
	}
	// (vii) the shipped outcome closes the task in Step 8
	if !strings.Contains(region(done, doneShippedHead, "\n### "), "close task=") {
		bad = append(bad, f+"shipped close: Step 8 must close the task on the `shipped` outcome")
	}
	// (viii) Step 9 acts on the open PR the guard found
	if doneParagraph(region(done, doneStep9Head, "\n## "), "**Which PR.**", donePRGuard, "`open`") == "" {
		bad = append(bad, f+"which PR: Step 9 must say {number} is the open PR the guard found")
	}
	// (ix) no second PR, as a rule
	if len(doneLines(region(done, doneRulesHead, "\n## "), "- **NEVER", "second PR")) == 0 {
		bad = append(bad, f+"rules: Rules must carry a NEVER line against a second PR")
	}
	// (x) one name: the guard's name says its scope
	for _, n := range doneOldGuardNames {
		if strings.Contains(done, n) {
			bad = append(bad, f+"old name: still names "+n)
		}
	}
	return bad
}

func TestDoneExistingPRLint(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "commands", "done.md"))
	if err != nil {
		t.Fatal(err)
	}
	done := string(b)
	for _, p := range donePRProblems(done) {
		t.Error(p)
	}

	// in returns done with edit applied to [from, the next `to` after it) - the whole file when from is "".
	in := func(from, to string, edit func(string) string) string {
		start, end := 0, len(done)
		if from != "" {
			if start = strings.Index(done, from); start < 0 {
				t.Fatalf("mutant region %q is gone - renamed? update this test", from)
			}
			if j := strings.Index(done[start+len(from):], to); j >= 0 {
				end = start + len(from) + j
			}
		}
		seg := done[start:end]
		out := edit(seg)
		if out == seg {
			t.Fatalf("mutant anchor not found in region %q", from)
		}
		return done[:start] + out + done[end:]
	}
	repl := func(old, new string) func(string) string {
		return func(s string) string { return strings.ReplaceAll(s, old, new) }
	}
	dropLine := func(prefix string) func(string) string {
		return func(s string) string {
			var keep []string
			for _, l := range strings.Split(s, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(l), prefix) {
					keep = append(keep, l)
				}
			}
			return strings.Join(keep, "\n")
		}
	}
	// moveAfter moves the line starting with line to just after the line starting with after.
	moveAfter := func(line, after string) func(string) string {
		return func(s string) string {
			var moved string
			var keep []string
			for _, l := range strings.Split(s, "\n") {
				if strings.HasPrefix(l, line) {
					moved = l
					continue
				}
				keep = append(keep, l)
			}
			var out []string
			for _, l := range keep {
				out = append(out, l)
				if strings.HasPrefix(l, after) && moved != "" {
					out = append(out, moved)
				}
			}
			return strings.Join(out, "\n")
		}
	}
	// paraBefore moves the paragraph starting with para to just before the one starting with before.
	paraBefore := func(para, before string) func(string) string {
		return func(s string) string {
			var moved string
			var keep []string
			for _, p := range strings.Split(s, "\n\n") {
				if strings.HasPrefix(p, para) {
					moved = p
					continue
				}
				keep = append(keep, p)
			}
			var out []string
			for _, p := range keep {
				if strings.HasPrefix(p, before) && moved != "" {
					out = append(out, moved)
				}
				out = append(out, p)
			}
			return strings.Join(out, "\n\n")
		}
	}
	const h2, h3 = "\n## ", "\n### "
	shippedRow := "| No open PR; a merged PR into `{LETS_MERGE_BRANCH}` holds HEAD |"
	for _, m := range []struct{ name, src, want string }{
		// (i) the decision table - symptom (b), closing unshipped work, and the second PR
		{"reused branch closed as shipped", in(doneGuardHead, h2, repl(shippedRow, "| No open PR; a merged PR into `{LETS_MERGE_BRANCH}` holds HEAD, or one that does not hold HEAD (new work on a reused branch) |")), "decision: the `shipped` condition must not cover does not hold"},
		{"cannot-tell closed as shipped", in(doneGuardHead, h2, repl(shippedRow, "| No open PR; a merged PR into `{LETS_MERGE_BRANCH}` holds HEAD or its head cannot be resolved |")), "decision: the `shipped` condition must not cover cannot"},
		{"declined closed as shipped", in(doneGuardHead, h2, repl(shippedRow, "| No open PR; a merged or declined PR into `{LETS_MERGE_BRANCH}` holds HEAD |")), "decision: the `shipped` condition must not cover declined"},
		{"shipped accepts any base", in(doneGuardHead, h2, repl(shippedRow, "| No open PR; a merged PR holds HEAD |")), "decision: the `shipped` condition must say `{LETS_MERGE_BRANCH}`"},
		{"shipped by branch name, holds HEAD moved to Then", in(doneGuardHead, h2, repl(shippedRow+" `shipped` #N | Every commit already shipped.", "| No open PR; a merged PR exists | `shipped` #N | Skip the holds HEAD check. Every commit already shipped.")), "decision: the `shipped` condition must say holds HEAD"},
		{"stop row deleted", in(doneGuardHead, h2, dropLine("| More than one open PR")), "decision: no stop row"},
		{"open no longer wins", in(doneGuardHead, h2, moveAfter("| Exactly one open PR", shippedRow)), "an open PR wins"},
		// (ii) symptom (a): the lookup sees only some states
		{"github lookup back to merged-only", in(doneGuardHead, h2, repl("--state all", "--state merged")), "all states: the github lookup"},
		{"bitbucket lookup OPEN-only", in(doneGuardHead, h2, repl("asking for every state (Bitbucket lists only OPEN otherwise)", "asking for OPEN PRs only")), "all states: the bitbucket lookup"},
		{"fork PR counted", in(doneGuardHead, h2, repl(", keeping only those whose head repository is the one `git push origin` updates (`headRepository` `nameWithOwner` equal to the owner/name parsed from `git remote get-url --push origin` - gh may be set to query another repository by default, so never trust a same-repository flag)", "")), "same repository: the github lookup"},
		{"identity bound to the forge default, not origin", in(doneGuardHead, h2, repl("`headRepository` `nameWithOwner` equal to the owner/name parsed from `git remote get-url --push origin`", "`isCrossRepository` false")), "push identity: the github lookup"},
		{"identity from the fetch URL", in(doneGuardHead, h2, repl("owner/name parsed from `git remote get-url --push origin` - gh may be set", "owner/name parsed from `git remote get-url origin` - gh may be set")), "push url: the github lookup"},
		{"github first page only", in(doneGuardHead, h2, repl(" All of them: pass a `--limit` well above the count you expect; a result count equal to the limit may be incomplete.", "")), "complete: the github lookup"},
		{"bitbucket first page only", in(doneGuardHead, h2, repl("following every page (the response's `next` link) until there is none", "reading the first page")), "complete: the bitbucket lookup"},
		// (iii) a failed lookup read as "no PR" - the duplicate path
		{"several push URLs not stopped", in(doneGuardHead, h2, repl(", or origin has more than one push URL |", " |")), "lookup error:"},
		{"failed lookup reads as no PR", in(doneGuardHead, h2, repl(`**STOP before any push:** "Could not get`, `Treat it as no PR and continue: "Could not get`)), "lookup error:"},
		// (iv) commits, not names
		{"holds-HEAD definition reversed", in(doneGuardHead, h2, repl("--is-ancestor HEAD <pr-head>", "--is-ancestor <pr-head> HEAD")), "holds HEAD:"},
		{"compared with the merge branch", in(doneGuardHead, h2, repl("Step 8's plain push decides.", "Step 8's plain push decides. A merged PR also counts when `git merge-base --is-ancestor HEAD origin/{LETS_MERGE_BRANCH}`.")), "merge-branch:"},
		// (v) Step 8: the duplicate PR, a forced push, a stale state
		{"github open path creates", in(doneGitHubFinish, h3, repl("never `gh pr create`", "then `gh pr create` as below")), "open creates: Step 8 github"},
		{"bitbucket open path creates", in(doneBbFinish, h3, repl("never create a PR", "then create a PR as below")), "open creates: Step 8 bitbucket"},
		{"fresh PR into a fixed base", in(doneGitHubFinish, h3, repl(`--base "{LETS_MERGE_BRANCH}"`, "--base main")), "fresh base: Step 8 github"},
		{"fresh PR without a base", in(doneGitHubFinish, h3, repl(`gh pr create --base "{LETS_MERGE_BRANCH}" `, "gh pr create ")), "fresh base: Step 8 github"},
		{"open path force-pushes", in(doneGitHubFinish, h3, repl("never `--force`", "with `--force` when rejected")), "force: Step 8 github"},
		{"re-check after the push", in(doneGitHubFinish, h3, paraBefore("**Re-check before any push.**", "**Outcome `fresh`:**")), "re-check: Step 8 github"},
		{"re-check compares category only", in(doneGitHubFinish, h3, repl("The same outcome and the same PR number as Step 6 confirmed", "The same outcome as Step 6 confirmed")), "re-check identity: Step 8 github"},
		// (vi) trunk-mode
		{"trunk skip dropped", in(doneGuardHead, h2, repl("Skip this entire guard", "Run this guard")), "trunk skip:"},
		// (vii) a shipped branch is never closed
		{"shipped never closes", in(doneShippedHead, h3, repl(`close task=<task-id> reason="Shipped in merged PR #{number}"`, `comment-add task=<task-id> body="Shipped in merged PR #{number}"`)), "shipped close:"},
		// (viii) the open PR's number never reaches Step 9
		{"Step 9 forgets the open PR", in(doneStep9Head, h2, dropLine("**Which PR.**")), "which PR:"},
		// (ix) the rule against a second PR
		{"Rules lost the no-second-PR line", in(doneRulesHead, h2, dropLine("- **NEVER open a second PR")), "rules:"},
		// (x) name and scope disagree again
		{"old name back", in(doneTrunkHead, h2, repl("- "+donePRGuard+": **skip**", "- Already-Merged Guard: **skip**")), "old name:"},
	} {
		got := donePRProblems(m.src)
		caught := false
		for _, p := range got {
			caught = caught || strings.Contains(p, m.want)
		}
		if !caught {
			t.Errorf("mutant %q was not caught by its check %q (got %q)", m.name, m.want, got)
		}
	}
}
