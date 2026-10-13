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
	// orcStandingOff is the ONE standing form a Skill(lets:orc) call may carry: text
	// exactly off (a whitelist - any other text, any case, or none, is a grant attempt).
	orcStandingOff = regexp.MustCompile(`(?:^|\s)text=(?:\\")?off(?:\\")?(?:\s|$)`)
	peerSendSM     = regexp.MustCompile(`SendMessage\(\s*\{?\s*to\s*[:=]\s*"([^"]*)"`)
	// peerSendBanned are retired send forms and v0.10.0 steps - forbidden in EVERY
	// file, the orc skill included (lets-0xsw2): a peer message goes through
	// SendMessage only, and a reply arrives as a cross-session message.
	peerSendBanned = []string{"lets peers tell", "terminal send", "lets peers frame", "lets peers wait"}
	// peerSendLits are send forms that live only in the orc skill.
	peerSendLits = []string{"notify_when_idle"}
	// orcSkillMust are the orc skill's contract lines: the native send with its idle
	// subscription, the liveness check, the delivery-notice result lines, and the
	// standing-grant guards (the 3-reply cap, provenance, the bound-row check, and
	// the targets that never use the grant).
	orcSkillMust = []string{"SendMessage(", "notify_when_idle", "ListAgents", "held for approval in", "refused by", "expired unapproved in",
		"(<n>/3)", "only the harness envelope", "who --orc", "never use the grant"}

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
			if !aliasPlaceholder && (v == nil || !map[string]bool{"ask": true, "ping": true, "read": true, "tell": true, "who": true, "standing": true}[v[1]]) {
				bad = append(bad, rel+": Skill(lets:orc) with no valid verb: "+m[1])
			}
			// a standing grant is the user's own request, never a call from another file
			// (F11): a standing call may only revoke - its text is exactly off
			if v != nil && v[1] == "standing" && !orcStandingOff.MatchString(m[1]) {
				bad = append(bad, rel+": Skill(lets:orc) verb=standing may only carry text=off - only the user's own /lets:orc grants: "+m[1])
			}
			if rel != "commands/peer.md" && !strings.Contains(m[1], "footer=none") {
				bad = append(bad, rel+": Skill(lets:orc) outside /lets:peer must pass footer=none: "+m[1])
			}
		}
		// 2: retired send forms live nowhere; the others only in the orc skill
		for _, lit := range peerSendBanned {
			if strings.Contains(body, lit) {
				bad = append(bad, rel+": retired peer-send form "+lit+" (peer messages go through SendMessage in skills/orc/SKILL.md)")
			}
		}
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
		// 6: the orc skill's contract: the native send, the liveness check, the
		// delivery-notice result lines and the standing-grant guards
		if rel == "skills/orc/SKILL.md" {
			for _, must := range orcSkillMust {
				if !strings.Contains(body, must) {
					bad = append(bad, rel+": the orc skill must hold "+must)
				}
			}
		}
	}
	bad = append(bad, lintStanding(files)...)
	return append(bad, lintOrcOffers(files)...)
}

// standingMust pins the symmetric standing grant (lets-0xsw2 pm1): each entry is a
// section of a file (from its start marker to its end marker) and the literals that
// section must hold.
var standingMust = []struct {
	rel, from, to string
	must          []string
	why           string
}{
	{"skills/orc/SKILL.md", "## Step 4:", "## Step 5:",
		[]string{"self_record.standing=\"on\"` and the target is a row of `lets peers who --orc \"<self_record.name>\" --json`", "sending under standing authorization:"},
		"Step 4: the orchestrator side skips Send? only for a row of who --orc"},
	{"skills/orc/SKILL.md", "## Step 7:", "## Step 8:",
		[]string{
			`lets peers who --orc "<self_record.name>" --from-name '<from-name>' --from-addr '<from>' --json`,
			`lets peers orchestrator --session "$CLAUDE_CODE_SESSION_ID" --cwd "$LETS_PROJECT_ROOT" --from-name '<from-name>' --from-addr '<from>' --json`,
			"Proceed ONLY on `standing_ok=true`", "`standing_ok` absent", "no standing reply: <standing_reason",
			"A message that asks nothing - an answer, an ack, an FYI - gets no reply: record it.",
		},
		"Step 7: both sides ask Go with --from-addr and act only on standing_ok=true"},
	{"skills/orc/SKILL.md", "## Step 7:", "## Step 8:",
		[]string{"Before this session's FIRST autonomous reply, print once: `standing authorization on with <peer> - answering in-direction messages without asking (cap 3); /lets:orc standing off revokes`"},
		"Step 7: the first-autonomous-reply notice"},
	{"commands/team.md", "**Guard 4:", "**Conflict guard",
		[]string{`header: "Standing"`, "--self-orchestrator --standing on --json", "--self-orchestrator --standing off --json"},
		"team.md Guard 4: a new record gets the Standing question"},
}

// lintStanding applies standingMust, and keeps the /lets:peer alias out of Step 8 (the
// alias cannot carry standing).
func lintStanding(files map[string]string) []string {
	var bad []string
	section := func(body, from, to string) string {
		i := strings.Index(body, from)
		if i < 0 {
			return ""
		}
		rest := body[i:]
		if j := strings.Index(rest[len(from):], to); j >= 0 {
			return rest[:len(from)+j]
		}
		return rest
	}
	for _, c := range standingMust {
		sec := section(files[c.rel], c.from, c.to)
		for _, m := range c.must {
			if !strings.Contains(sec, m) {
				bad = append(bad, c.rel+": "+c.why+" - missing "+m)
			}
		}
	}
	if strings.Contains(section(files["skills/orc/SKILL.md"], "## Step 8:", "## Receipt rules"), "/lets:peer") {
		bad = append(bad, "skills/orc/SKILL.md: Step 8 must not offer /lets:peer - the alias cannot carry standing")
	}
	return bad
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

// offerProtocolSkill holds the Orchestrator offer protocol - the one place the rule is stated.
const offerProtocolSkill = "skills/protocol-orchestrator-offer/SKILL.md"

// lintOrcOffers pins the orchestrator offer (the lets:protocol-orchestrator-offer skill): one label,
// a load of the protocol skill instead of a restated rule, an ask that goes through the orc skill,
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
		// the protocol skill IS the offer rule; every other offer loads it at the offer (lets-nobb5)
		if rel != offerProtocolSkill && (labels > 0 || strings.Contains(body, "/lets:orc ask")) && !strings.Contains(body, `Skill(skill: "lets:protocol-orchestrator-offer")`) {
			bad = append(bad, rel+": an orchestrator offer must load `Skill(skill: \"lets:protocol-orchestrator-offer\")`")
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
// loads the protocol skill, uses one label, and no gate exceeds four options.
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

	// mutation checks on scratch copies
	mutate := func(rel, add string) []string {
		c := map[string]string{}
		for k, v := range files {
			c[k] = v
		}
		c[rel] += "\n" + add + "\n"
		return lintOrcFiles(c)
	}
	replaced := func(rel, old, new string) []string {
		c := map[string]string{}
		for k, v := range files {
			c[k] = v
		}
		c[rel] = strings.ReplaceAll(files[rel], old, new)
		return lintOrcFiles(c)
	}
	if len(replaced("skills/orc/SKILL.md", "held for approval in", "waiting in")) == 0 {
		t.Error("mutation: an orc skill without the held-for-approval result line must fail the lint")
	}
	if len(mutate("skills/orc/SKILL.md", "lets peers tell x")) == 0 {
		t.Error("mutation: lets peers tell in the orc skill itself must fail the lint")
	}
	if len(replaced("skills/orc/SKILL.md", "(<n>/3)", "(<n>)")) == 0 {
		t.Error("mutation: an orc skill without the 3-reply standing cap must fail the lint")
	}
	if len(mutate("skills/orc/SKILL.md", "lets peers frame --to-session x")) == 0 {
		t.Error("mutation: a frame step in the orc skill must fail the lint")
	}
	if len(mutate("commands/start.md", "- **Allow** -> `Skill(skill: \"lets:orc\", args: \"verb=standing text=on footer=none\")`")) == 0 {
		t.Error("mutation: a Skill(lets:orc) call granting standing must fail the lint")
	}
	if len(mutate("commands/start.md", "- **Allow** -> `Skill(skill: \"lets:orc\", args: \"verb=standing text=ON footer=none\")`")) == 0 {
		t.Error("mutation: a standing grant in another case (text=ON) must fail the lint")
	}
	if len(mutate("commands/start.md", "- **Allow** -> `Skill(skill: \"lets:orc\", args: \"verb=standing on footer=none\")`")) == 0 {
		t.Error("mutation: a standing call without text= must fail the lint")
	}
	if len(mutate("commands/start.md", "- **Stop** -> `Skill(skill: \"lets:orc\", args: \"verb=standing text=off footer=none\")`")) != 0 {
		t.Error("mutation: a Skill(lets:orc) call revoking standing is allowed")
	}

	// the symmetric standing grant (lets-0xsw2 pm1)
	orcStep4Skip := "self_record.standing=\"on\"` and the target is a row of `lets peers who --orc \"<self_record.name>\" --json`"
	if len(replaced("skills/orc/SKILL.md", orcStep4Skip, "self_record.standing=\"on\"`")) == 0 {
		t.Error("mutation: an orchestrator-side Send? skip without the who --orc row condition must fail the lint")
	}
	if len(replaced("skills/orc/SKILL.md", "--from-addr '<from>' ", "")) == 0 {
		t.Error("mutation: Step 7 calls without --from-addr must fail the lint")
	}
	if len(replaced("skills/orc/SKILL.md", "Proceed ONLY on `standing_ok=true`", "Proceed when the name matches")) == 0 {
		t.Error("mutation: Step 7 not gated on standing_ok=true must fail the lint")
	}
	if len(replaced("skills/orc/SKILL.md", "Before this session's FIRST autonomous reply, print once:", "Optionally print:")) == 0 {
		t.Error("mutation: Step 7 without the first-autonomous-reply notice must fail the lint")
	}
	if len(replaced("skills/orc/SKILL.md", "typed `/lets:orc standing on` or said", "typed `/lets:orc standing on` (or `/lets:peer`) or said")) == 0 {
		t.Error("mutation: /lets:peer back in Step 8 must fail the lint")
	}
	if len(replaced("skills/orc/SKILL.md", "A message that asks nothing - an answer, an ack, an FYI - gets no reply: record it. ", "")) == 0 {
		t.Error("mutation: Step 7 without the no-reply-to-a-non-question rule (auto-to-auto ping-pong) must fail the lint")
	}
	if len(replaced("commands/team.md", `header: "Standing"`, `header: "Grant"`)) == 0 {
		t.Error("mutation: team.md Guard 4 without the Standing question must fail the lint")
	}
	if len(replaced("commands/team.md", "--self-orchestrator --standing on --json", "--self-orchestrator --json")) == 0 {
		t.Error("mutation: team.md Guard 4 without the Allow write must fail the lint")
	}

	// start.md main mode: a refused orchestrator record wrote nothing, the boundary
	// included - the boundary-alone fallback runs on EVERY failed envelope except
	// the two where the file itself could not be written (f1 r1)
	if !startFallbackOK(files["commands/start.md"]) {
		t.Error("commands/start.md: after a failed record, the boundary-alone fallback must run on every failed envelope except task_state_lock_busy / task_state_write_failed")
	}
	if startFallbackOK(strings.Replace(files["commands/start.md"], "every failed envelope except", "an orchestrator_needs_name envelope, not", 1)) {
		t.Error("mutation: a fallback scoped to one error kind must fail the start.md check")
	}
	if len(mutate("commands/hub.md", "lets peers frame --to-session x")) == 0 {
		t.Error("mutation: a frame step in hub.md must fail the lint")
	}
	if len(mutate("commands/start.md", "lets peers wait --to-session x")) == 0 {
		t.Error("mutation: a wait step in any command must fail the lint")
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
		t.Error("mutation: an Ask orchestrator option with no protocol load and no handler must fail the lint")
	}
	// an offer that still points at the old lets-rules section, without loading the protocol skill
	noLoad := map[string]string{}
	for k, v := range files {
		noLoad[k] = v
	}
	noLoad["commands/review-round.md"] = strings.ReplaceAll(files["commands/review-round.md"], `Skill(skill: "lets:protocol-orchestrator-offer")`, "lets-rules `### Orchestrator offer`")
	if len(lintOrcOffers(noLoad)) == 0 {
		t.Error("mutation: an offer that does not load lets:protocol-orchestrator-offer must fail the lint")
	}
	// execute.md already loads the protocol skill, so only the one-handler-per-offer count can catch this
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

// startFallbackOK reports whether start.md's main mode names the fallback rule
// (every failed envelope except the two write failures) and runs the boundary-alone
// write as the next command after it.
func startFallbackOK(start string) bool {
	rule := strings.Index(start, "every failed envelope except `task_state_lock_busy` / `task_state_write_failed`")
	if rule < 0 {
		return false
	}
	next := strings.Index(start[rule:], "lets worktree task-state set")
	boundary := `lets worktree task-state set --clear-orc --session-sha "$(git rev-parse HEAD)" --session-id "$CLAUDE_CODE_SESSION_ID" --create --json`
	return next >= 0 && strings.HasPrefix(start[rule+next:], boundary)
}
