//go:build unix

package peerscmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// capability says whether a command a remediation names can act on a refusal, for
// its reason, detail and repo scope. A command absent here may not be named at all
// (lets-th8ti: a refusal pointed at /lets:hub, which cannot see a live orchestrator
// in another repo).
var capability = map[string]func(reason, detail string, sibling bool) bool{
	// wakes a STOPPED orchestrator
	"/lets:hub": func(r, _ string, _ bool) bool { return r == "target_not_alive" },
	// rebinds this branch, or re-registers an orchestrator
	"/lets:start": func(r, _ string, _ bool) bool {
		return r == "orchestrator_not_registered" || r == "target_not_alive" || r == "target_in_other_repo" || r == "bound_ambiguous" || r == "orchestrator_needs_name"
	},
	// sets LETS_LAUNCHER - the missing switch, not a missing repo or a silent Orca
	"/lets:init": func(r, d string, _ bool) bool {
		return r == "target_in_other_repo" && (d == "" || d == "orca_not_selected" || d == "its cwd is outside this repo")
	},
	// `who` lists THIS repo only
	"/lets:orc": func(r, _ string, sib bool) bool { return r == "target_unsendable" && !sib },
	// lists the sibling repos' rows with their send reason
	"lets peers who --orca-repos": func(r, _ string, sib bool) bool { return r == "target_unsendable" && sib },
	"/rename": func(r, _ string, _ bool) bool {
		return r == "target_unsendable" || r == "orchestrator_needs_name" || r == "bound_ambiguous"
	},
}

var commandRe = regexp.MustCompile(`/lets:[a-z-]+|/rename\b|lets peers who --orca-repos`)

func TestRemedy_NamesOnlyCommandsThatCanAct(t *testing.T) {
	for reason, details := range refusalDetails {
		for _, d := range details {
			for _, orca := range []bool{false, true} {
				for _, sib := range []bool{false, true} {
					line := remedy(reason, d, "X", orca, sib)
					if line == "" {
						t.Errorf("remedy(%s, %q, orca=%v, sibling=%v) is empty", reason, d, orca, sib)
					}
					for _, m := range commandRe.FindAllString(line, -1) {
						can, known := capability[m]
						if !known || !can(reason, d, sib) {
							t.Errorf("remedy(%s, %q, orca=%v, sibling=%v) names %s, which cannot act on it: %q", reason, d, orca, sib, m, line)
						}
					}
				}
			}
		}
	}
}

// The vocabulary cannot fall behind the code: every literal a refusal is built from
// in the resolver's files must be in refusalDetails.
func TestRemedy_VocabularyCoversSource(t *testing.T) {
	known := map[string]bool{}
	for r, ds := range refusalDetails {
		known[r] = true
		for _, d := range ds {
			known[d] = true
		}
	}
	// Only the places a refusal is built: a Refused{} literal, an assignment to
	// ref.Reason / ref.Detail / res.Reason, and who.go's peer send reasons (p.Reason -
	// the target_unsendable details). Degraded{} lines are telemetry, not refusals.
	// A literal followed by `+` is a prefix: "x" + name -> "x*".
	site := regexp.MustCompile(`Refused\{|\bref\.(?:Reason|Detail)\b|\bres\.Reason\s*(?:=|,)|\bp\.Reason\s*=|\bp\.Send, p\.Reason\s*=`)
	val := regexp.MustCompile(`(?:Reason|Detail):\s*"([^"]*)"(\s*\+)?` +
		`|(?:ref\.Reason|ref\.Detail|res\.Reason|p\.Reason)(?:, (?:ref\.Detail|res\.Refused))?\s*=\s*"([^"]*)"(\s*\+)?(?:, "([^"]*)")?` +
		`|p\.Send, p\.Reason = "none", (?:nonEmpty\(p\.Reason, )?"([^"]*)"`)
	star := func(plus string) string {
		if plus != "" {
			return "*"
		}
		return ""
	}
	seen := 0
	var missing []string
	for _, f := range []string{"binding.go", "sibling.go", "who.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "Degraded{") || !site.MatchString(line) {
				continue
			}
			for _, m := range val.FindAllStringSubmatch(line, -1) {
				for _, v := range []string{m[1] + star(m[2]), m[3] + star(m[4]), m[5], m[6]} {
					if v == "" || v == "*" {
						continue
					}
					seen++
					if !known[v] {
						missing = append(missing, fmt.Sprintf("%s:%d %q", f, n+1, v))
					}
				}
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("refusal literals missing from refusalDetails: %v", missing)
	}
	if seen < 15 { // the scan must actually be reading refusals, not matching nothing
		t.Errorf("the source scan found only %d refusal literals - it no longer reads the code", seen)
	}
}

// hub.md must still claim the capability the table grants it - the two cannot drift.
func TestRemedy_HubCapabilityMatchesHubSpec(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "commands", "hub.md"))
	if err != nil {
		t.Fatal(err)
	}
	hub := string(data)
	for _, want := range []string{"lets orca wake", "stopped"} {
		if !strings.Contains(hub, want) {
			t.Errorf("hub.md no longer says %q, but capability still lets remediations send users there", want)
		}
	}
	for reason, details := range refusalDetails {
		for _, d := range details {
			if capability["/lets:hub"](reason, d, false) != (reason == "target_not_alive") {
				t.Errorf("/lets:hub may only be named for a stopped orchestrator, not %s/%q", reason, d)
			}
		}
	}
}
