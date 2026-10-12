package cli_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

const membersEnvSession = "aaaaaaaa-0000-4000-8000-0000000000aa"

// membersFixture is a main checkout and a linked worktree the team file snake.md
// claims (the v0.10.0 team worktree); it returns both paths.
func membersFixture(t *testing.T) (mainRoot, teamWT string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mainRoot, teamWT = filepath.Join(base, "repo"), filepath.Join(base, "team_snake")
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	git(mainRoot, "init", "-q", "-b", "main")
	git(mainRoot, "commit", "-q", "--allow-empty", "-m", "init")
	git(mainRoot, "worktree", "add", "-q", "-b", "team_snake", teamWT)
	gitDir := git(teamWT, "rev-parse", "--absolute-git-dir")
	writeTestFile(t, filepath.Join(mainRoot, ".lets", "teams", "snake.md"),
		fmt.Sprintf("---\nteam: %q\nworktree: %q\ngit_dir: %q\n---\n", "snake", teamWT, gitDir))
	return mainRoot, teamWT
}

// runMembers runs `lets members <args>` in dir through the root command.
func runMembers(t *testing.T, dir string, args []string) (string, error) {
	t.Helper()
	chdirTo(t, dir)
	root := cli.NewRootCmd()
	root.SetArgs(append([]string{"members"}, args...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

// jsonPath walks a.b[0].c through decoded JSON.
func jsonPath(v any, path string) (any, bool) {
	for _, part := range strings.Split(path, ".") {
		name, idx, hasIdx := strings.Cut(part, "[")
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = m[name]; !ok {
			return nil, false
		}
		if hasIdx {
			i, err := strconv.Atoi(strings.TrimSuffix(idx, "]"))
			arr, ok := v.([]any)
			if err != nil || !ok || i >= len(arr) {
				return nil, false
			}
			v = arr[i]
		}
	}
	return v, true
}

// membersCondition checks one `path == value` against obj.
func membersCondition(obj map[string]any, cond string) error {
	path, want, ok := strings.Cut(cond, " == ")
	if !ok {
		return fmt.Errorf("malformed condition %q", cond)
	}
	got, found := jsonPath(obj, strings.TrimSpace(path))
	want = strings.TrimSpace(want)
	var wantV any
	switch want {
	case "env":
		wantV = membersEnvSession
	case "null":
		if found && got != nil {
			return fmt.Errorf("%s = %v, want null", path, got)
		}
		return nil
	case "[]":
		wantV = []any{}
	case "true", "false":
		wantV = want == "true"
	default:
		wantV = want
	}
	if !found || !reflect.DeepEqual(got, wantV) {
		return fmt.Errorf("%s = %#v (found=%v), want %#v", path, got, found, wantV)
	}
	return nil
}

// TestMembersStub_V010Calls runs every `lets members` call of the v0.10.0 plugin and
// checks the condition its reader acts on: exit 0, one JSON object (or one line).
func TestMembersStub_V010Calls(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	mainRoot, teamWT := membersFixture(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", membersEnvSession)
	f, err := os.Open(filepath.Join("testdata", "members-v010-calls.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, " | ")
		if len(parts) != 3 {
			t.Fatalf("malformed fixture line %q", line)
		}
		n++
		dir := map[string]string{"team": teamWT, "main": mainRoot}[parts[0]]
		if dir == "" {
			t.Fatalf("unknown cwd %q", parts[0])
		}
		out, err := runMembers(t, dir, strings.Fields(parts[1]))
		if err != nil {
			t.Errorf("%s: exit non-zero: %v\n%s", parts[1], err, out)
			continue
		}
		if text, isLine := strings.CutPrefix(parts[2], "line ~ "); isLine {
			if strings.Count(out, "\n") != 1 || !strings.Contains(out, text) {
				t.Errorf("%s: want one line containing %q, got %q", parts[1], text, out)
			}
			continue
		}
		dec := json.NewDecoder(strings.NewReader(out))
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			t.Errorf("%s: not a JSON object: %v\n%s", parts[1], err, out)
			continue
		}
		if dec.More() {
			t.Errorf("%s: more than one JSON value:\n%s", parts[1], out)
		}
		if obj["ok"] != true || !strings.HasSuffix(fmt.Sprint(obj["deprecated"]), "run /lets:update to load the matching plugin") {
			t.Errorf("%s: ok=%v deprecated=%v", parts[1], obj["ok"], obj["deprecated"])
		}
		for _, cond := range strings.Split(parts[2], " && ") {
			if err := membersCondition(obj, cond); err != nil {
				t.Errorf("%s [%s]: %v\n%s", parts[1], parts[0], err, out)
			}
		}
	}
	if n < 15 {
		t.Errorf("only %d fixture calls read", n)
	}
}

// TestMembersStub_V010Flags: the v0.10.0 flag sets and types, link defaulting to
// team, and an unknown flag still parsing.
func TestMembersStub_V010Flags(t *testing.T) {
	str := func(names ...string) map[string]string {
		m := map[string]string{}
		for _, n := range names {
			m[n] = "string"
		}
		return m
	}
	want := map[string]map[string]string{
		"add":     str("scope", "name", "role", "model", "isolation", "worktree-path", "worktree-branch", "link", "agent-id", "cwd"),
		"dismiss": str("scope", "name"),
		"status":  str("scope", "name"),
		"lead":    str("scope"),
	}
	want["add"]["json"] = "bool"
	want["dismiss"]["all"], want["dismiss"]["json"] = "bool", "bool"
	want["status"]["json"] = "bool"
	want["lead"]["claim"], want["lead"]["json"] = "bool", "bool"
	root := cli.NewMembersCmd()
	for verb, flags := range want {
		cmd, _, err := root.Find([]string{verb})
		if err != nil || cmd.Name() != verb {
			t.Fatalf("verb %s: %v", verb, err)
		}
		for name, typ := range flags {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Errorf("%s --%s missing", verb, name)
				continue
			}
			if f.Value.Type() != typ {
				t.Errorf("%s --%s is %s, want %s", verb, name, f.Value.Type(), typ)
			}
		}
	}
	add, _, _ := root.Find([]string{"add"})
	if add.Flags().Lookup("link").DefValue != "team" {
		t.Errorf("add --link default = %q, want team", add.Flags().Lookup("link").DefValue)
	}
	dir := t.TempDir()
	out, err := runMembers(t, dir, []string{"status", "--scope", "run-x", "--some-future-flag", "v", "--json"})
	if err != nil || !strings.Contains(out, `"ok":true`) {
		t.Errorf("an unknown flag must still parse: %v\n%s", err, out)
	}
}

// TestMembersStub_SchemaContract ports the v0.10.0 registry's TestResult_SchemaContract:
// the same envelope and nested key sets, plus deprecated.
func TestMembersStub_SchemaContract(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", membersEnvSession)
	dir := t.TempDir()
	get := func(args ...string) map[string]any {
		t.Helper()
		out, err := runMembers(t, dir, append(args, "--json"))
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return m
	}
	need := func(label string, m map[string]any, keys ...string) {
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("%s: missing key %q", label, k)
			}
		}
	}
	env := []string{"schema_version", "ok", "subcommand", "scope", "steps", "deprecated"}
	add := get("add", "--scope", "run-x", "--name", "impl-x", "--role", "lets:implementer")
	need("add", add, append(env, "member")...)
	member, _ := add["member"].(map[string]any)
	need("add.member", member, "name", "agent_name", "role", "model", "isolation", "worktree_path",
		"worktree_branch", "kind", "session", "pid", "set", "caller_toplevel", "lead_pid", "link", "status")
	need("dismiss", get("dismiss", "--scope", "run-x", "--all"), append(env, "dismissed")...)
	need("status", get("status", "--scope", "run-x"), append(env, "lead", "members")...)
	lead := get("lead", "--scope", "run-x")
	need("lead", lead, append(env, "lead", "claimed")...)
	leadObj, _ := lead["lead"].(map[string]any)
	need("lead.lead", leadObj, "session", "name", "pid", "set", "status")
	if add["schema_version"] != float64(1) {
		t.Errorf("schema_version = %v, want 1", add["schema_version"])
	}
	for _, k := range []string{"pid", "lead_pid"} {
		if _, ok := member[k].(float64); !ok {
			t.Errorf("add.member.%s must stay a number: %#v", k, member[k])
		}
	}
	if _, ok := member["set"].(string); !ok {
		t.Errorf("add.member.set must stay a string: %#v", member["set"])
	}
	steps, _ := add["steps"].([]any)
	if len(steps) != 1 || !strings.Contains(fmt.Sprint(steps[0]), "deprecated and records nothing") {
		t.Errorf("steps = %v", steps)
	}
}
