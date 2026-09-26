package initcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// smokeFactProblems returns every design fact the live smoke (docs/smoke/
// parallel-implementers.md) rests on that the plugin files no longer hold. It reads
// only its argument, so TestSmokeFacts can feed it in-memory mutants.
func smokeFactProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	// The harness ignores the Agent tool's team and mode parameters (lets-7dwc1): no
	// command may pass them, or a run silently loses what it thought it asked for.
	for _, key := range []string{"execute", "team"} {
		for _, bad := range []string{"team_name", "mode="} {
			if strings.Contains(f[key], bad) {
				add(key + ".md must not pass " + bad + " - the harness ignores it")
			}
		}
	}

	// Every brief - an isolated one included - carries BASE, outside the
	// isolated-only block: the isolated agent's guard switches to it.
	exec := f["execute"]
	brief := ""
	if i := strings.Index(exec, "The brief, `.lets/cache/chunk-"); i >= 0 {
		brief = exec[i:]
		if j := strings.Index(brief, "\n```\n\n"); j >= 0 {
			brief = brief[:j]
		}
	}
	base, isolatedOnly := strings.Index(brief, "\nBASE: {base sha}"), strings.Index(brief, "{isolated only")
	if base < 0 || isolatedOnly < 0 || base > isolatedOnly {
		add("every chunk brief must carry `BASE: {base sha}` before the isolated-only block")
	}

	// An isolated chunk is integrated into the caller's tree before it can be accepted.
	integrate, accept := strings.Index(exec, "lets integrate --from"), strings.Index(exec, "**Accept dispatch")
	if integrate < 0 || accept < 0 || integrate > accept {
		add("execute.md must run `lets integrate` before the Accept dispatch")
	}

	// member-run never messages a gone member: its Step 3 returns agent_gone before
	// any SendMessage.
	next := sectionSpan(f["member-run"], "## Step 3: Next / Correct")
	gone, send := strings.Index(next, "`gone`"), strings.Index(next, "SendMessage(")
	if gone < 0 || send < 0 || gone > send || !strings.Contains(next[:send], "agent_gone") || !strings.Contains(next[:send], "Never message it") {
		add("member-run Step 3 must return agent_gone for a gone member and never message it, before its SendMessage")
	}
	return p
}

// TestSmokeFacts pins the design facts the live smoke rests on, on the real plugin
// files, then proves each guard can fail.
func TestSmokeFacts(t *testing.T) {
	files := map[string]string{
		"execute":    readPlugin(t, filepath.Join("commands", "execute.md")),
		"team":       readPlugin(t, filepath.Join("commands", "team.md")),
		"member-run": readPlugin(t, filepath.Join("skills", "member-run", "SKILL.md")),
	}
	for _, problem := range smokeFactProblems(files) {
		t.Error(problem)
	}
	mutants := []struct{ name, key, old, repl, want string }{
		{"team passes team_name", "team", "## Rules", "Agent(team_name=\"x\")\n\n## Rules", "team_name"},
		{"execute passes mode=", "execute", "## Rules", "Agent(mode=\"plan\")\n\n## Rules", "mode="},
		{"BASE moves into the isolated block", "execute", "\nBASE: {base sha}", "\nBASE_SHA: {base sha}", "BASE"},
		{"integrate after Accept", "execute", "lets integrate --from", "lets integrate --frm", "lets integrate"},
		{"member-run messages a gone member", "member-run", "Never message it", "Message it anyway", "agent_gone"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(files[m.key], m.old) {
				t.Fatalf("mutant cannot apply: %q is not in %s", m.old, m.key)
			}
			mutated := map[string]string{}
			for k, v := range files {
				mutated[k] = v
			}
			mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, 1)
			for _, problem := range smokeFactProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
