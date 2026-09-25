package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	orcSkillCall = regexp.MustCompile(`Skill\(\s*skill:\s*"lets:orc"\s*,\s*args:\s*"((?:[^"\\]|\\.)*)"`)
	orcVerbArg   = regexp.MustCompile(`\bverb=([a-z-]+|<verb>)`)
	peerSendSM   = regexp.MustCompile(`SendMessage\(\s*\{?\s*to\s*[:=]\s*"([^"]*)"`)
	peerSendLits = []string{"lets peers tell", "terminal send", "notify_when_idle"}

	orcOfferLabel  = regexp.MustCompile(`label:\s*"([^"]*[Oo]rchestrator[^"]*)"`)
	orcOptionsOpen = regexp.MustCompile(`options:\s*\[`)
	// orcOfferExempt lists files where /lets:orc is reference text, not a touchpoint.
	// A new file that mentions /lets:orc fails rule 3 until it is a touchpoint or listed here.
	orcOfferExempt = map[string]bool{
		"skills/orc/SKILL.md":            true,
		"commands/peer.md":               true,
		"commands/start.md":              true,
		"commands/handoff.md":            true,
		"commands/install-deprecated.md": true,
	}
	// agentSendExempt maps each file that messages a member THIS session spawned to the ONE
	// placeholder it names that member by (execute.md's run record and member-run both call it
	// {agent} - in member-run, the Agent name the registry records) - never a peer session. Any
	// other recipient in these files, a generic {name} included, is still a peer send and fails.
	agentSendExempt = map[string]string{
		"commands/execute.md":        "{agent}",
		"skills/member-run/SKILL.md": "{agent}",
	}
)

// orcLintFiles returns commands/*.md and skills/*/SKILL.md, keyed by the path
// relative to the plugin root.
func orcLintFiles(t *testing.T, pluginDir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, glob := range []string{"commands/*.md", "skills/*/SKILL.md"} {
		m, _ := filepath.Glob(filepath.Join(pluginDir, glob))
		for _, p := range m {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			rel, _ := filepath.Rel(pluginDir, p)
			files[filepath.ToSlash(rel)] = string(b)
		}
	}
	return files
}

// lintOrcFiles applies the peer-messaging rules and returns every violation.
func lintOrcFiles(files map[string]string) []string {
	var bad []string
	for rel, body := range files {
		// 1 + 5: verbs and the single-footer rule
		for _, m := range orcSkillCall.FindAllStringSubmatch(body, -1) {
			v := orcVerbArg.FindStringSubmatch(m[1])
			aliasPlaceholder := rel == "commands/peer.md" && v != nil && v[1] == "<verb>" // the alias forwards the user's verb; the skill validates it
			if !aliasPlaceholder && (v == nil || !map[string]bool{"ask": true, "ping": true, "read": true, "tell": true, "who": true}[v[1]]) {
				bad = append(bad, rel+": Skill(lets:orc) with no valid verb: "+m[1])
			}
			if rel != "commands/peer.md" && !strings.Contains(m[1], "footer=none") {
				bad = append(bad, rel+": Skill(lets:orc) outside /lets:peer must pass footer=none: "+m[1])
			}
		}
		// 2: peer-send forms live only in the orc skill
		if rel != "skills/orc/SKILL.md" {
			for _, lit := range peerSendLits {
				if strings.Contains(body, lit) {
					bad = append(bad, rel+": peer-send form "+lit+" outside skills/orc/SKILL.md")
				}
			}
			for _, m := range peerSendSM.FindAllStringSubmatch(body, -1) {
				if own, ok := agentSendExempt[rel]; ok && m[1] == own {
					continue // the named implementer this run spawned, not a peer
				}
				bad = append(bad, rel+": peer SendMessage outside skills/orc/SKILL.md: "+m[1])
			}
		}
		// 3: touchpoints only OFFER /lets:orc (inside an AskUserQuestion fence, a LETS box, or on a handle line)
		if !orcOfferExempt[rel] {
			inFence, fence := false, []string{}
			flush := func() {
				text := strings.Join(fence, "\n")
				if strings.Contains(text, "/lets:orc") && !strings.Contains(text, "AskUserQuestion(") && !strings.Contains(text, "┌─ LETS") {
					bad = append(bad, rel+": /lets:orc in a fence that is neither an AskUserQuestion nor a LETS box")
				}
				fence = fence[:0]
			}
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "```") {
					if inFence {
						flush()
					}
					inFence = !inFence
					continue
				}
				if inFence {
					fence = append(fence, line)
					continue
				}
				if strings.Contains(line, "/lets:orc") && !strings.HasPrefix(strings.TrimSpace(line), "- **") {
					bad = append(bad, rel+": /lets:orc outside an offer: "+strings.TrimSpace(line))
				}
			}
		}
		// 4: the alias never sends itself
		if rel == "commands/peer.md" && (strings.Contains(body, "SendMessage") || strings.Contains(body, "lets peers")) {
			bad = append(bad, rel+": the alias delegates to the orc skill and sends nothing itself")
		}
		// 6: the SendMessage fallback is never silent (lets-rry3c): its Step 5 row is MANDATORY
		// and names the line to print
		if rel == "skills/orc/SKILL.md" {
			row := ""
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "| `reason=peer_not_ready`, `claude_fallback_allowed=true` |") {
					row = line
				}
			}
			if !strings.Contains(row, "**MANDATORY:**") || !strings.Contains(row, "delivered via SendMessage fallback") {
				bad = append(bad, rel+": the peer_not_ready fallback row must be **MANDATORY:** and print `delivered via SendMessage fallback`")
			}
		}
	}
	return append(bad, lintOrcOffers(files)...)
}

// optionCounts returns the number of labels in every multi-line `options: [` block.
func optionCounts(body string) []int {
	var counts []int
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		loc := orcOptionsOpen.FindStringIndex(lines[i])
		if loc == nil || strings.Contains(lines[i][loc[1]:], "]") { // one-line placeholder block
			continue
		}
		n := 0
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t, "]") {
				break
			}
			if strings.Contains(t, "label:") {
				n++
			}
			i = j
		}
		counts = append(counts, n)
	}
	return counts
}

// lintOrcOffers pins the orchestrator offer (lets-rules "Orchestrator offer"): one label,
// a pointer at the rule instead of a restated rule, an ask that goes through the orc skill,
// and no gate above four options. A ping is a notification and needs no pointer.
func lintOrcOffers(files map[string]string) []string {
	var bad []string
	for rel, body := range files {
		for _, n := range optionCounts(body) {
			if n > 4 {
				bad = append(bad, rel+": an AskUserQuestion options block has more than four labels")
			}
		}
		if orcOfferExempt[rel] {
			continue
		}
		for _, m := range orcOfferLabel.FindAllStringSubmatch(body, -1) {
			if m[1] != "Ask orchestrator" && m[1] != "Ping orchestrator" {
				bad = append(bad, rel+": orchestrator option label must be \"Ask orchestrator\" (or \"Ping orchestrator\" for a notification): "+m[1])
			}
		}
		labels := strings.Count(body, `label: "Ask orchestrator"`)
		if (labels > 0 || strings.Contains(body, "/lets:orc ask")) && !strings.Contains(body, "Orchestrator offer") {
			bad = append(bad, rel+": an orchestrator offer must point at lets-rules \"Orchestrator offer\"")
		}
		// one handler per offer: a file-level "some verb=ask exists" would let a gate lose its handler
		// while another gate in the same file still has one (execute.md carries two, plan.md three)
		asks := 0
		for _, m := range orcSkillCall.FindAllStringSubmatch(body, -1) {
			if v := orcVerbArg.FindStringSubmatch(m[1]); v != nil && v[1] == "ask" {
				asks++
			}
		}
		if asks < labels {
			bad = append(bad, fmt.Sprintf("%s: %d \"Ask orchestrator\" offers but %d Skill(lets:orc) verb=ask handlers", rel, labels, asks))
		}
	}
	return bad
}

// TestOrcLint pins where peer messages may be sent from: only the orc skill sends,
// touchpoints only offer, and a delegated orc run never adds a second footer. An offer
// points at the rule, uses one label, and no gate exceeds four options.
func TestOrcLint(t *testing.T) {
	pluginDir := filepath.Join("..", "..", "..", "plugins", "lets")
	files := orcLintFiles(t, pluginDir)
	if _, ok := files["commands/team.md"]; !ok {
		t.Fatal("commands/team.md not scanned - its Agent Teams SendMessage shape must be covered (and pass)")
	}
	for _, v := range lintOrcFiles(files) {
		t.Error(v)
	}
	// the hub is an Orca addon: its launcher guard runs before its first `lets` call
	if hub, ok := files["commands/hub.md"]; ok {
		guard := strings.Index(hub, `[ "{LETS_LAUNCHER}" = "orca" ]`)
		first := regexp.MustCompile("(?m)^lets |`lets ").FindStringIndex(hub)
		if guard < 0 || first == nil || guard > first[0] {
			t.Errorf("commands/hub.md: the LETS_LAUNCHER=orca guard must precede the first lets call (guard=%d first=%v)", guard, first)
		}
	}

	// a repo_index reaches the orc skill only from Go output (lets-th8ti): the skill
	// names both Go sources, and the start pointer forwards the resolved one
	if orc := files["skills/orc/SKILL.md"]; !strings.Contains(orc, "target.repo_index") || !strings.Contains(orc, "row.repo_index") || strings.Contains(orc, "only `/lets:hub` passes them") {
		t.Error("skills/orc/SKILL.md: a repo_index must come from a resolved target or a who --orc row, and not only from /lets:hub")
	}
	if !strings.Contains(files["commands/start.md"], "--repo-index <target.repo_index>") {
		t.Error("commands/start.md: the orchestrator pointer must forward a resolved target's repo_index")
	}

	// mutation checks on scratch copies
	mutate := func(rel, add string) []string {
		c := map[string]string{}
		for k, v := range files {
			c[k] = v
		}
		c[rel] += "\n" + add + "\n"
		return lintOrcFiles(c)
	}
	silent := map[string]string{}
	for k, v := range files {
		silent[k] = v
	}
	silent["skills/orc/SKILL.md"] = strings.Replace(files["skills/orc/SKILL.md"], "delivered via SendMessage fallback", "sent", 1)
	if len(lintOrcFiles(silent)) == 0 {
		t.Error("mutation: a fallback row without its printed line must fail the lint")
	}
	if len(mutate("commands/done.md", "lets peers tell x")) == 0 {
		t.Error("mutation: a peer send in done.md must fail the lint")
	}
	if len(mutate("commands/team.md", `SendMessage({to: "x"})`)) == 0 {
		t.Error("mutation: a peer SendMessage in team.md must fail the lint")
	}
	if len(mutate("commands/execute.md", `SendMessage({to: "orc-main"})`)) == 0 {
		t.Error("mutation: a peer SendMessage in execute.md must fail the lint even though it may address its own implementer")
	}
	if len(mutate("commands/execute.md", `SendMessage({to: "{name}"})`)) == 0 {
		t.Error("mutation: a generic {name} placeholder in execute.md must fail the lint - only its run-record {agent} is exempt")
	}
	if len(mutate("commands/done.md", "```\nAskUserQuestion(\n```\n- **Ping** -> `Skill(skill: \"lets:orc\", args: \"verb=ping text=x\")`")) == 0 {
		t.Error("mutation: an orc call without footer=none must fail the lint")
	}
	if len(mutate("commands/note.md", "```\nAskUserQuestion(\n    options: [\n      { label: \"Ask orchestrator\", description: \"x\" }\n    ]\n```")) == 0 {
		t.Error("mutation: an Ask orchestrator option with no rule pointer and no handler must fail the lint")
	}
	// execute.md already points at the rule, so only the one-handler-per-offer count can catch this
	if len(mutate("commands/execute.md", "```\nAskUserQuestion(\n    options: [\n      { label: \"Ask orchestrator\", description: \"x\" }\n    ]\n```")) == 0 {
		t.Error("mutation: an Ask orchestrator offer without its own verb=ask handler must fail the lint")
	}
	if len(mutate("commands/note.md", "```\nAskUserQuestion(\n    options: [\n      { label: \"Consult orchestrator\", description: \"x\" }\n    ]\n```")) == 0 {
		t.Error("mutation: a second orchestrator label must fail the lint")
	}
	if len(mutate("commands/note.md", "```\nAskUserQuestion(\n    options: [\n      { label: \"a\" },\n      { label: \"b\" },\n      { label: \"c\" },\n      { label: \"d\" },\n      { label: \"e\" }\n    ]\n```")) == 0 {
		t.Error("mutation: a five-option gate must fail the lint")
	}
	if len(mutate("commands/check.md", "Then run /lets:orc ask yourself.")) == 0 {
		t.Error("mutation: /lets:orc in plain prose must fail the lint")
	}
}
