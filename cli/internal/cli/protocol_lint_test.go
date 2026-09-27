package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// lets-nobb5: the always-on rules keep only what applies on every turn; the
// procedures a command needs at one step live in a lazy protocol layer - the
// lets:protocol-tracker and lets:protocol-orchestrator-offer skills, and the
// plain files under plugins/lets/protocol/ - that a command loads right before
// the step. This lint keeps the two layers wired:
//
//  1. a file that runs a ```lets-tracker block loads lets:protocol-tracker
//     before its first block (the skill itself is exempt);
//  2. every ${CLAUDE_PLUGIN_ROOT}/protocol/<x>.md reference resolves, and every
//     protocol file is referenced at least once;
//  3. no pointer names a rules section that left the core;
//  4. every remaining lets-rules section pointer names a heading the rules file
//     still has - so the core rewrite cannot strand a pointer silently.
//
// The orchestrator-offer load is asserted in one place only: orc_lint_test.go.
// protocolProblems reads only its arguments, so the test feeds it mutants.

const (
	trackerProtocolSkill = "skills/protocol-tracker/SKILL.md"
	trackerLoad          = `Skill(skill: "lets:protocol-tracker")`
)

var (
	protocolRef = regexp.MustCompile(`\$\{CLAUDE_PLUGIN_ROOT\}/protocol/([A-Za-z0-9._-]+)`)
	// a lets-rules pointer followed by a backticked or quoted heading:
	// lets-rules `## X`, `.claude/rules/lets-rules.md` `### X`, lets-rules.md "## X"
	rulesHeadingPtr = regexp.MustCompile("lets-rules(?:\\.md)?`?\\s*[`\"](#{2,4} [^`\"]+)[`\"]")
	// sections that left the core for the protocol layer (lets-nobb5)
	movedSections = []string{"Orchestrator offer", "Tracker Adapters", "Handoff lane", "## Worktrees",
		"Carve-out - peers", "Carve-out - hub", "Carve-out - bound sibling sessions", "Carve-out for web data-gatherers"}
	// NOT "Carve-out (spawn entry claim)": the AUTO MODE entry-claim carve-out stays in core (inventory M3)
)

func firstTrackerFence(body string) int {
	off := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```lets-tracker") {
			return off
		}
		off += len(line)
	}
	return -1
}

func protocolProblems(files map[string]string, protocolFiles []string, rules string) []string {
	var bad []string
	referenced := map[string]bool{}
	have := map[string]bool{}
	for _, p := range protocolFiles {
		have[p] = true
	}
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		body := files[rel]
		// 1: tracker protocol before the first lets-tracker block
		if fence := firstTrackerFence(body); fence >= 0 && rel != trackerProtocolSkill {
			load := strings.Index(body, trackerLoad)
			if load < 0 || load > fence {
				bad = append(bad, rel+": runs a ```lets-tracker block but does not load "+trackerLoad+" before its first one")
			}
		}
		// 2: protocol file references resolve
		for _, m := range protocolRef.FindAllStringSubmatch(body, -1) {
			referenced[m[1]] = true
			if !have[m[1]] {
				bad = append(bad, rel+": ${CLAUDE_PLUGIN_ROOT}/protocol/"+m[1]+" does not exist")
			}
		}
		for _, line := range strings.Split(body, "\n") {
			if !strings.Contains(line, "lets-rules") {
				continue
			}
			// 3: no pointer to a section that left the core
			for _, s := range movedSections {
				if strings.Contains(line, s) {
					bad = append(bad, rel+": points at lets-rules "+s+", which moved to the protocol layer: "+strings.TrimSpace(line))
				}
			}
			// 4: a remaining section pointer names a heading the rules still have
			for _, m := range rulesHeadingPtr.FindAllStringSubmatch(line, -1) {
				if !strings.Contains(rules, "\n"+m[1]) {
					bad = append(bad, rel+": points at lets-rules "+m[1]+", which the rules file does not have")
				}
			}
		}
	}
	for _, p := range protocolFiles {
		if !referenced[p] {
			bad = append(bad, "protocol/"+p+" is referenced by no command or skill")
		}
	}
	return bad
}

func TestProtocolLint(t *testing.T) {
	pluginDir := filepath.Join("..", "..", "..", "plugins", "lets")
	files := orcLintFiles(t, pluginDir)
	paths, err := filepath.Glob(filepath.Join(pluginDir, "protocol", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("plugins/lets/protocol/*.md: no protocol files found")
	}
	var protocolFiles []string
	for _, p := range paths {
		protocolFiles = append(protocolFiles, filepath.Base(p))
	}
	rulesData, err := os.ReadFile(filepath.Join(pluginDir, "rules", "lets-rules.md"))
	if err != nil {
		t.Fatal(err)
	}
	rules := string(rulesData)
	if _, ok := files[trackerProtocolSkill]; !ok {
		t.Fatal(trackerProtocolSkill + " not scanned")
	}

	for _, p := range protocolProblems(files, protocolFiles, rules) {
		t.Error(p)
	}

	// mutants: each guard must be able to fail
	mutate := func(rel, old, new string) map[string]string {
		c := map[string]string{}
		for k, v := range files {
			c[k] = v
		}
		if old == "" {
			c[rel] += new
		} else {
			if !strings.Contains(c[rel], old) {
				t.Fatalf("mutant anchor %q not in %s", old, rel)
			}
			c[rel] = strings.Replace(c[rel], old, new, 1)
		}
		return c
	}
	cases := []struct {
		name  string
		files map[string]string
		proto []string
	}{
		{"tracker block without the load", mutate("commands/note.md", trackerLoad, "the tracker"), protocolFiles},
		{"tracker load after the first block", mutate("commands/note.md", "---\n", "```lets-tracker\nshow task=<id>\n```\n---\n"), protocolFiles},
		{"reference to a missing protocol file", mutate("commands/ask.md", "", "\nRead `${CLAUDE_PLUGIN_ROOT}/protocol/nope.md`.\n"), protocolFiles},
		{"unreferenced protocol file", files, append(append([]string{}, protocolFiles...), "orphan.md")},
		{"pointer to a moved section", mutate("commands/check.md", "", "\nper lets-rules `### Handoff lane`\n"), protocolFiles},
		{"pointer to a heading the rules lack", mutate("commands/status.md", "", "\nper lets-rules `## No Such Section`\n"), protocolFiles},
		{"pointer to a moved carve-out", mutate("commands/start.md", "", "\nper lets-rules **Carve-out - bound sibling sessions.**\n"), protocolFiles},
	}
	for _, c := range cases {
		if len(protocolProblems(c.files, c.proto, rules)) == 0 {
			t.Errorf("mutant %q passed - the guard cannot fail", c.name)
		}
	}
	// the carve-out that stays in core is not a moved-section pointer
	if p := protocolProblems(mutate("commands/execute.md", "", "\nper the lets-rules AUTO MODE entry-claim carve-out (Carve-out (spawn entry claim))\n"), protocolFiles, rules); len(p) != 0 {
		t.Errorf("a pointer to the AUTO MODE entry-claim carve-out (stays in core) must pass: %v", p)
	}
}
