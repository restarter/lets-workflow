package cli_test

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// lets-me9qt: the division of labour lives in ONE file, protocol/roles.md. The
// core carries a one-line pointer; /lets:start loads the file on both entry
// paths; member-run (every member of every caller) and /lets:handoff paste it
// whole into the brief, because those agents never load the rules.
// rolesProblems reads only its arguments, so the test feeds it mutants.

const rolesFile = "protocol/roles.md"

var (
	routeNames = []string{"CLAIM", "PLAN-CHANGE", "FACT", "CHECK", "ACCEPT", "INDEPENDENT", "OWNER", "CROSS-TASK"}
	routeRow   = regexp.MustCompile(`(?m)^\|\s*(CLAIM|PLAN-CHANGE|FACT|CHECK|ACCEPT|INDEPENDENT|OWNER|CROSS-TASK)\s*\|`)
	rolesLoad  = "${CLAUDE_PLUGIN_ROOT}/" + rolesFile
	// each place that must load or deliver the routes: file, the heading its region starts at,
	// and the prefix of the next heading that ends it
	rolesLoaders = []struct{ file, from, to string }{
		{"commands/start.md", "\n## Step 8: Task Size Assessment", "\n## "},
		{"commands/start.md", "\n### Step M2: Set the stance", "\n### "},
		{"skills/member-run/SKILL.md", "\n## Step 2: Spawn", "\n## "},
		{"commands/handoff.md", "\n### 7.1 Save the brief", "\n### "},
		{"commands/handoff.md", "\n## Step 6: Deliver", "\n## "}, // print-only: a pointer, no file to insert into
	}
	claimMode = regexp.MustCompile(`(?s)\n## Modes\n.*\n### CLAIM \(.*\n## Report\n`)
	bashFence = regexp.MustCompile("(?s)```bash\n(.*?)```")
)

// region returns the text from the first `from` to the next `to` after it ("" when absent).
func region(body, from, to string) string {
	i := strings.Index(body, from)
	if i < 0 {
		return ""
	}
	rest := body[i+len(from):]
	if j := strings.Index(rest, to); j >= 0 {
		return rest[:j]
	}
	return rest
}

// pluginFiles returns every .md and .js file under the plugin, keyed by the
// path relative to the plugin root.
func pluginFiles(t *testing.T, pluginDir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(pluginDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(p); ext != ".md" && ext != ".js" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(pluginDir, p)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func rolesProblems(files map[string]string) []string {
	var bad []string
	law, ok := files[rolesFile]
	if !ok {
		return []string{rolesFile + " is missing"}
	}
	seen := map[string]int{}
	for _, m := range routeRow.FindAllStringSubmatch(law, -1) {
		seen[m[1]]++
	}
	for _, r := range routeNames {
		if seen[r] != 1 {
			bad = append(bad, rolesFile+": route "+r+" must appear in exactly one table row")
		}
	}
	for rel, body := range files {
		if rel != rolesFile && routeRow.MatchString(body) {
			bad = append(bad, rel+" restates a route row - point at "+rolesFile+" instead")
		}
	}
	if !strings.Contains(files["rules/lets-rules.md"], rolesFile) {
		bad = append(bad, "rules/lets-rules.md must name "+rolesFile+" in its who-does-what line")
	}
	for _, l := range rolesLoaders {
		if !strings.Contains(region(files[l.file], l.from, l.to), rolesLoad) {
			bad = append(bad, l.file+" "+strings.TrimSpace(l.from)+" must load or deliver "+rolesLoad)
		}
	}
	if !claimMode.MatchString(files["agents/skeptic.md"]) {
		bad = append(bad, "agents/skeptic.md needs a ### CLAIM mode inside ## Modes, before ## Report")
	}
	if !strings.Contains(files["agents/lead.md"], rolesFile) {
		bad = append(bad, "agents/lead.md must point at "+rolesFile)
	}
	return bad
}

func TestRolesLint(t *testing.T) {
	pluginDir := filepath.Join("..", "..", "..", "plugins", "lets")
	files := pluginFiles(t, pluginDir)
	for _, p := range rolesProblems(files) {
		t.Error(p)
	}

	// mutate replaces `old` inside the region of `from` (the whole file when from is "").
	mutate := func(rel, from, old, new string) map[string]string {
		m := make(map[string]string, len(files))
		for k, v := range files {
			m[k] = v
		}
		body := m[rel]
		start := 0
		if from != "" {
			if start = strings.Index(body, from); start < 0 {
				t.Fatalf("mutant region %q not in %s", from, rel)
			}
		}
		i := strings.Index(body[start:], old)
		if i < 0 {
			t.Fatalf("mutant anchor %q not in %s after %q", old, rel, from)
		}
		m[rel] = body[:start+i] + new + body[start+i+len(old):]
		return m
	}
	x := "${CLAUDE_PLUGIN_ROOT}/protocol/x.md"
	for name, m := range map[string]map[string]string{
		"a route row restated in the team template": mutate("templates/team.md", "", "## 5. Message format", "| CLAIM | x | y | z |\n\n## 5. Message format"),
		"a route row dropped from the law":          mutate(rolesFile, "", "| INDEPENDENT |", "| SOMETHING |"),
		"core pointer gone":                         mutate("rules/lets-rules.md", "", rolesFile, "protocol/x.md"),
		"start Step 8 no longer loads the routes":   mutate("commands/start.md", "\n## Step 8: Task Size Assessment", rolesLoad, x),
		"start Step M2 no longer loads the routes":  mutate("commands/start.md", "\n### Step M2: Set the stance", rolesLoad, x),
		"member-run no longer delivers the routes":  mutate("skills/member-run/SKILL.md", "\n## Step 2: Spawn", rolesLoad, x),
		"handoff no longer delivers the routes":     mutate("commands/handoff.md", "\n### 7.1 Save the brief", rolesLoad, x),
		"print-only handoff lost its pointer":       mutate("commands/handoff.md", "\n## Step 6: Deliver", rolesLoad, x),
		"skeptic CLAIM mode gone":                   mutate("agents/skeptic.md", "", "### CLAIM (", "### CLAIMS-GONE ("),
		"lead no longer points at the routes":       mutate("agents/lead.md", "", rolesFile, "protocol/x.md"),
	} {
		if len(rolesProblems(m)) == 0 {
			t.Errorf("mutant %q was not caught", name)
		}
	}
}

// TestRolesDelivery runs the two delivery snippets exactly as the plugin carries them.
func TestRolesDelivery(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	pluginDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "plugins", "lets"))
	lawBytes, err := os.ReadFile(filepath.Join(pluginDir, rolesFile))
	if err != nil {
		t.Fatal(err)
	}
	law := string(lawBytes)
	snippet := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(pluginDir, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range bashFence.FindAllStringSubmatch(string(b), -1) {
			if strings.Contains(m[1], "ROLES_FAILED") {
				return m[1]
			}
		}
		t.Fatalf("%s: no bash block containing ROLES_FAILED", rel)
		return ""
	}
	// run returns the status token (stdout) apart from any diagnostics (stderr).
	run := func(script, root string) (string, string) {
		var out, errb bytes.Buffer
		c := exec.Command(bash, "-c", script)
		c.Env = append(os.Environ(), "CLAUDE_PLUGIN_ROOT="+root)
		c.Stdout, c.Stderr = &out, &errb
		_ = c.Run()
		return strings.TrimSpace(out.String()), errb.String()
	}
	missing := t.TempDir() // a plugin root without protocol/roles.md
	emptyLaw := t.TempDir()
	if err := os.MkdirAll(filepath.Join(emptyLaw, "protocol"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyLaw, rolesFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, s := range []struct {
		rel, placeholder, marker string
	}{
		{"commands/handoff.md", "<ARTIFACT_FILE>", "## When you finish"},
		{"skills/member-run/SKILL.md", "{brief-file}", "REPORT_FILE: /abs/report.md"},
	} {
		code := snippet(s.rel)
		for name, c := range map[string]struct{ in, want string }{
			"inserted before the last marker": {
				in:   "# Brief\n\n" + s.marker + "\nquoted\n\n" + s.marker + "\nlast\n",
				want: "# Brief\n\n" + s.marker + "\nquoted\n\n" + law + "\n" + s.marker + "\nlast\n",
			},
			"appended without a marker": {
				in:   "# Brief\nbody\n",
				want: "# Brief\nbody\n\n" + law,
			},
		} {
			f := filepath.Join(t.TempDir(), "brief.md")
			if err := os.WriteFile(f, []byte(c.in), 0o600); err != nil {
				t.Fatal(err)
			}
			if tok, diag := run(strings.ReplaceAll(code, s.placeholder, f), pluginDir); tok != "ROLES_OK" {
				t.Fatalf("%s %s: %q (stderr %q)", s.rel, name, tok, diag)
			}
			if b, _ := os.ReadFile(f); string(b) != c.want {
				t.Errorf("%s %s: got\n%q\nwant\n%q", s.rel, name, b, c.want)
			}
		}
		for name, root := range map[string]string{"missing law": missing, "empty law": emptyLaw} {
			f := filepath.Join(t.TempDir(), "brief.md")
			_ = os.WriteFile(f, []byte("# Brief\n"), 0o600)
			if tok, diag := run(strings.ReplaceAll(code, s.placeholder, f), root); tok != "ROLES_FAILED" {
				t.Errorf("%s %s: %q (stderr %q), want ROLES_FAILED", s.rel, name, tok, diag)
			}
			if b, _ := os.ReadFile(f); string(b) != "# Brief\n" {
				t.Errorf("%s %s changed the brief: %q", s.rel, name, b)
			}
		}
	}
}
