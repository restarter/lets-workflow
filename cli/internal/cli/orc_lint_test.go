package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	orcSkillCall = regexp.MustCompile(`Skill\(\s*skill:\s*"lets:orc"\s*,\s*args:\s*"((?:[^"\\]|\\.)*)"`)
	orcVerbArg   = regexp.MustCompile(`\bverb=([a-z-]+|<verb>)`)
	peerSendSM   = regexp.MustCompile(`SendMessage\(\s*\{?\s*to\s*[:=]`)
	peerSendLits = []string{"lets peers tell", "terminal send", "notify_when_idle"}
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
			if peerSendSM.MatchString(body) {
				bad = append(bad, rel+": peer SendMessage outside skills/orc/SKILL.md")
			}
		}
		// 3: touchpoints only OFFER /lets:orc (inside an AskUserQuestion fence or on a handle line)
		if rel == "commands/done.md" || rel == "commands/execute.md" || rel == "commands/opinion.md" {
			inFence, fence := false, []string{}
			flush := func() {
				text := strings.Join(fence, "\n")
				if strings.Contains(text, "/lets:orc") && !strings.Contains(text, "AskUserQuestion(") {
					bad = append(bad, rel+": /lets:orc in a fence without AskUserQuestion(")
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
	}
	return bad
}

// TestOrcLint pins where peer messages may be sent from: only the orc skill sends,
// touchpoints only offer, and a delegated orc run never adds a second footer.
func TestOrcLint(t *testing.T) {
	pluginDir := filepath.Join("..", "..", "..", "plugins", "lets")
	files := orcLintFiles(t, pluginDir)
	if _, ok := files["commands/team.md"]; !ok {
		t.Fatal("commands/team.md not scanned - its Agent Teams SendMessage shape must be covered (and pass)")
	}
	for _, v := range lintOrcFiles(files) {
		t.Error(v)
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
	if len(mutate("commands/done.md", "lets peers tell x")) == 0 {
		t.Error("mutation: a peer send in done.md must fail the lint")
	}
	if len(mutate("commands/team.md", `SendMessage({to: "x"})`)) == 0 {
		t.Error("mutation: a peer SendMessage in team.md must fail the lint")
	}
	if len(mutate("commands/done.md", "```\nAskUserQuestion(\n```\n- **Ping** -> `Skill(skill: \"lets:orc\", args: \"verb=ping text=x\")`")) == 0 {
		t.Error("mutation: an orc call without footer=none must fail the lint")
	}
}
