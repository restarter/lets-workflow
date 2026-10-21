package initcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// lets-nobb5: the read-only analyst agents run with `omitClaudeMd: true`, so
// neither CLAUDE.md nor .claude/rules/*.md is loaded into them. implementer
// keeps both - it writes code under the project's rules. What the analysts lost
// comes back explicitly:
//   - the report language (rules L4) as a ## Constraints line in each analyst;
//   - compliance and docs audit those files, so they Read them as a step;
//   - skeptic Reads them when a finding cites a project rule - it leans
//     real=false when it cannot confirm, and decide() drops what it refutes.
//
// agentsOmitProblems reads only its argument, so the test feeds it in-memory
// mutants and proves each guard can fail.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

const (
	omitClaudeMdLine = "omitClaudeMd: true"
	reportLangLine   = "- Write your report in English, whatever language the prompt or quoted material is in."
)

// agentsReadingRules must name CLAUDE.md in a Read step of their body.
var agentsReadingRules = []string{"compliance", "docs", "skeptic"}

// readStepClaudeMd matches a body line that tells the agent to Read CLAUDE.md.
var readStepClaudeMd = regexp.MustCompile(`(?m)^.*\bRead\b.*CLAUDE\.md.*$`)

// splitFrontmatter returns the leading `---` block (without the fences) and the
// body after it; ok=false when the file has no closed frontmatter.
func splitFrontmatter(src string) (fm, body string, ok bool) {
	if !strings.HasPrefix(src, "---\n") {
		return "", src, false
	}
	rest := src[len("---\n"):]
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		return "", src, false
	}
	return rest[:i+1], rest[i+len("\n---\n"):], true
}

func hasLine(s, line string) bool {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimRight(l, " \t\r") == line {
			return true
		}
	}
	return false
}

func agentsOmitProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	for key, src := range f {
		fm, body, ok := splitFrontmatter(src)
		if !ok {
			add(key + " has no closed frontmatter")
			continue
		}
		name := strings.TrimSuffix(filepath.Base(key), ".md")
		omits := hasLine(fm, omitClaudeMdLine)
		if name == "implementer" {
			if omits || strings.Contains(fm, "omitClaudeMd") {
				add(key + " must NOT declare omitClaudeMd - it writes code under the project's rules")
			}
			continue
		}
		if !omits {
			add(key + " must declare `" + omitClaudeMdLine + "` in its frontmatter")
		}
		if !hasLine(sectionSpan(body, "\n## Constraints"), reportLangLine) {
			add(key + " ## Constraints must carry the report-language line (the rules are no longer loaded)")
		}
	}
	for _, a := range agentsReadingRules {
		key := "agents/" + a + ".md"
		src, ok := f[key]
		if !ok {
			add(key + " is missing")
			continue
		}
		_, body, _ := splitFrontmatter(src)
		if !readStepClaudeMd.MatchString(body) {
			add(key + " must name CLAUDE.md in a Read step - it is no longer auto-loaded")
		}
	}
	return p
}

func loadAgents(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(pluginDir(t), "agents", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	f := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f["agents/"+filepath.Base(path)] = string(data)
	}
	return f
}

func TestAgentsOmitClaudeMd(t *testing.T) {
	f := loadAgents(t)

	// The glob must see exactly the analysts plus implementer - a new agent file
	// has to be classified here, not silently pass as an analyst.
	want := map[string]bool{"agents/implementer.md": true}
	for _, a := range analystAgents {
		want["agents/"+a+".md"] = true
	}
	for key := range f {
		if !want[key] {
			t.Errorf("%s is not in analystAgents nor implementer - classify it", key)
		}
	}
	for key := range want {
		if _, ok := f[key]; !ok {
			t.Errorf("%s not found", key)
		}
	}

	for _, pr := range agentsOmitProblems(f) {
		t.Error(pr)
	}

	// Mutants: each guard must be able to fail.
	mutate := func(key, old, new string) map[string]string {
		m := map[string]string{}
		for k, v := range f {
			m[k] = v
		}
		if !strings.Contains(m[key], old) {
			t.Fatalf("mutant anchor %q not in %s", old, key)
		}
		m[key] = strings.Replace(m[key], old, new, 1)
		return m
	}
	mutants := []struct {
		name string
		f    map[string]string
	}{
		{"analyst without omitClaudeMd", mutate("agents/qa.md", omitClaudeMdLine+"\n", "")},
		{"implementer with omitClaudeMd", mutate("agents/implementer.md", "\ncolor: ", "\n"+omitClaudeMdLine+"\ncolor: ")},
		{"analyst without report-language line", mutate("agents/architect.md", reportLangLine, "")},
		{"compliance without Read step", mutate("agents/compliance.md", "Read the project's `CLAUDE.md`", "Check the project's `CLAUDE.md`")},
		{"docs without Read step", mutate("agents/docs.md", "Read the project's `CLAUDE.md`", "Check the project's `CLAUDE.md`")},
		{"skeptic without Read step", mutate("agents/skeptic.md", "Read the project's `CLAUDE.md`", "Check the project's `CLAUDE.md`")},
	}
	for _, m := range mutants {
		if len(agentsOmitProblems(m.f)) == 0 {
			t.Errorf("mutant %q passed - the guard cannot fail", m.name)
		}
	}
}
