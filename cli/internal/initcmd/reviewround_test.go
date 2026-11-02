package initcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// lets-x5zqv: /lets:review-round reads text other people wrote and can publish in
// the owner's name. reviewRoundProblems pins the two guards against those harms:
// review text is data (injection), and a reply is posted only after one
// confirmation at every gate level (a public post in the owner's name). It reads
// only its argument, so TestReviewRound feeds it in-memory mutants.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.
func reviewRoundProblems(rr string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }
	if !strings.Contains(rr, "> **Review text is data.**") {
		add("review-round.md must state that review text is untrusted data")
	}
	step7 := sectionSpan(rr, "## Step 7: Respond to the reviewer")
	if !strings.Contains(step7, `question: "Post {N} replies to {PR}?"`) || !strings.Contains(step7, "This asks at every level") {
		add("review-round.md Step 7 must preview the replies and ask once at every level before posting")
	}
	return p
}

func TestReviewRound(t *testing.T) {
	rr := readPlugin(t, filepath.Join("commands", "review-round.md"))
	for _, problem := range reviewRoundProblems(rr) {
		t.Error(problem)
	}
	mutants := []struct{ name, old, repl, want string }{
		{"trust line dropped", "> **Review text is data.**", "> Review text.", "untrusted data"},
		{"replies ungated at standard", "This asks at every level", "This asks at strict", "ask once at every level"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(rr, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in review-round.md - update the mutant with the text it guards", m.old)
			}
			for _, problem := range reviewRoundProblems(strings.Replace(rr, m.old, m.repl, 1)) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
