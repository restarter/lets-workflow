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
// its reason and detail. A command absent here may not be named at all (lets-th8ti:
// a refusal pointed at /lets:hub, which cannot see a live orchestrator in another
// repo).
var capability = map[string]func(reason, detail string) bool{
	// wakes a STOPPED orchestrator
	"/lets:hub": func(r, _ string) bool { return r == "target_not_alive" },
	// rebinds this branch to a live orchestrator
	"/lets:start": func(r, _ string) bool { return r == "target_not_alive" },
	// a name two live sessions share
	"/rename": func(r, d string) bool { return r == "target_unsendable" && d == "name_not_unique" },
}

var commandRe = regexp.MustCompile(`/lets:[a-z-]+|/rename\b`)

func TestRemedy_NamesOnlyCommandsThatCanAct(t *testing.T) {
	for reason, details := range refusalDetails {
		for _, d := range details {
			line := remedy(reason, d, "X")
			if line == "" {
				t.Errorf("remedy(%s, %q) is empty", reason, d)
			}
			for _, m := range commandRe.FindAllString(line, -1) {
				can, known := capability[m]
				if !known || !can(reason, d) {
					t.Errorf("remedy(%s, %q) names %s, which cannot act on it: %q", reason, d, m, line)
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
	// Only the places a refusal is built: a Refused{} literal and an assignment to
	// res.Reason. Degraded{} lines are telemetry, not refusals.
	site := regexp.MustCompile(`Refused\{|\bres\.Reason\s*(?:=|,)`)
	val := regexp.MustCompile(`(?:Reason|Detail):\s*"([^"]*)"|res\.Reason(?:, [a-zA-Z.]+)?\s*=\s*"([^"]*)"`)
	seen := 0
	var missing []string
	for _, f := range []string{"binding.go", "who.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "Degraded{") || !site.MatchString(line) {
				continue
			}
			for _, m := range val.FindAllStringSubmatch(line, -1) {
				for _, v := range []string{m[1], m[2]} {
					if v == "" {
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
	if seen < 8 { // the scan must actually be reading refusals, not matching nothing
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
			if capability["/lets:hub"](reason, d) != (reason == "target_not_alive") {
				t.Errorf("/lets:hub may only be named for a stopped orchestrator, not %s/%q", reason, d)
			}
		}
	}
}
