package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// handoffLaunchGuard is the 7.3 line that stops --send without LETS_LAUNCHER=orca.
const handoffLaunchGuard = "Only when `{LETS_LAUNCHER}` is `orca`"

// lintHandoff checks the /lets:handoff command and its alias: Orca is reached only
// through `lets handoff` and only after the launcher guard, Codex only through
// `lets handoff codex`, nothing interrupts an agent, no result file is read, and
// the alias delegates without calling Go itself.
func lintHandoff(handoff, alias string) []string {
	var bad []string
	guard := strings.Index(handoff, handoffLaunchGuard)
	for _, call := range []string{"lets handoff targets", "lets handoff send"} {
		if i := strings.Index(handoff, call); i >= 0 && (guard < 0 || i < guard) {
			bad = append(bad, "handoff.md: "+call+" before the LETS_LAUNCHER=orca guard")
		}
	}
	for name, body := range map[string]string{"handoff.md": handoff, "review-handoff.md": alias} {
		for _, lit := range []string{"codex exec", "Orca.app", "orca terminal", "--interrupt"} {
			if strings.Contains(body, lit) {
				bad = append(bad, name+": "+lit+" (Go owns Codex and Orca; there is no interrupt)")
			}
		}
	}
	if strings.Count(alias, `Skill(skill: "lets:handoff"`) != 1 || strings.Contains(alias, "lets handoff ") {
		bad = append(bad, "review-handoff.md: the alias delegates once to lets:handoff and calls nothing itself")
	}
	if strings.Contains(handoff, "-result.json") {
		bad = append(bad, "handoff.md: the result is the background run's own output, never a -result.json")
	}
	return bad
}

// TestHandoffLint pins the delivery boundaries of /lets:handoff (lets-w5tm5).
func TestHandoffLint(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "plugins", "lets", "commands")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	handoff, alias := read("handoff.md"), read("review-handoff.md")
	if !strings.Contains(handoff, handoffLaunchGuard) {
		t.Fatal("handoff.md: the 7.3 launcher guard line is missing")
	}
	for _, v := range lintHandoff(handoff, alias) {
		t.Error(v)
	}
	// mutation self-check: each assertion can fail
	for name, m := range map[string][2]string{
		"send before guard":  {"lets handoff send --brief x\n" + handoff, alias},
		"codex exec":         {handoff + "\ncodex exec -", alias},
		"orca terminal":      {handoff, alias + "\norca terminal send"},
		"interrupt":          {handoff + "\n--interrupt", alias},
		"alias calls Go":     {handoff, alias + "\nlets handoff codex --brief x"},
		"alias delegates 2x": {handoff, alias + "\n" + `Skill(skill: "lets:handoff", args: "x")`},
		"result file":        {handoff + "\nRead <base>-result.json", alias},
	} {
		if len(lintHandoff(m[0], m[1])) == 0 {
			t.Errorf("mutation %q must fail the lint", name)
		}
	}
}
