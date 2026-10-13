//go:build unix

package worktreecmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// teamRepoSlug is the repo slug of teamSetup's main checkout, whose directory is
// named literally lets-workflow: lead names in these tests are literals, never
// computed with peername.RepoSlug.
const teamRepoSlug = "lets-workflow"

// teamSetup is a repo (directory lets-workflow) with a linked team worktree, a
// plugin root holding the real team template, and an empty session registry whose
// live names the test sets.
func teamSetup(t *testing.T, liveNames ...string) (repo, wt, pluginRoot string) {
	t.Helper()
	return teamSetupIn(t, "lets-workflow", liveNames...)
}

// teamSetupIn is teamSetup with the main checkout's directory named repoDir.
func teamSetupIn(t *testing.T, repoDir string, liveNames ...string) (repo, wt, pluginRoot string) {
	t.Helper()
	repo = filepath.Join(realTempDir(t), repoDir)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "-c", "init.defaultBranch=main", "init")
	runIn(t, repo, "git", "config", "user.email", "test@example.com")
	runIn(t, repo, "git", "config", "user.name", "test")
	runIn(t, repo, "git", "commit", "--allow-empty", "-m", "initial")
	wt = filepath.Join(realTempDir(t), "team_snake")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "team_snake", wt)

	tmpl, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "templates", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	pluginRoot = realTempDir(t)
	if err := os.MkdirAll(filepath.Join(pluginRoot, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "templates", "team.md"), tmpl, 0o644); err != nil {
		t.Fatal(err)
	}

	home := realTempDir(t)
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i, name := range liveNames {
		pid := 1000 + i
		body := fmt.Sprintf(`{"name":%q,"sessionId":"aaaaaaaa-0000-4000-8000-00000000000%d","cwd":"/x","peerProtocol":1}`, name, i)
		if err := os.WriteFile(filepath.Join(home, "sessions", fmt.Sprintf("%d.json", pid)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return home }
	ccregistry.ProcAlive = func(pid int) bool { return pid >= 1000 && pid < 1000+len(liveNames) }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
	return repo, wt, pluginRoot
}

func teamInit(repo, wt, pluginRoot, callsign string) (*worktreecmd.TeamInitResult, error) {
	return worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: callsign, Area: "the CLI", Worktree: wt, PluginRoot: pluginRoot})
}

func noTempFiles(t *testing.T, repo string) {
	t.Helper()
	if m, _ := filepath.Glob(filepath.Join(repo, ".lets", "teams", ".*.tmp-*")); len(m) > 0 {
		t.Errorf("temp files left: %v", m)
	}
}

func TestTeamInit_Renders(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || !res.OK {
		t.Fatalf("TeamInit: %+v, %v", res, err)
	}
	want := filepath.Join(repo, ".lets", "teams", "snake.md")
	if res.TeamFile != want {
		t.Errorf("team_file = %q, want %q", res.TeamFile, want)
	}
	b, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{`team: "snake"`, `area: "the CLI"`, `worktree: "` + wt + `"`, `lead_name: "lets-workflow-snake-lead"`, `agent_command: "claude"`, `orca_agent: "claude"`, `git_dir: "`} {
		if !strings.Contains(string(b), line) {
			t.Errorf("team file lacks %s", line)
		}
	}
	gitDir := strings.TrimSpace(gitOutput(t, wt, "rev-parse", "--absolute-git-dir"))
	if name, ok, _, err := teamfile.FindByWorktree(filepath.Join(repo, ".lets", "teams"), wt, gitDir); !ok || name != "snake" || err != nil {
		t.Errorf("FindByWorktree = %q, %v, %v", name, ok, err)
	}
	noTempFiles(t, repo)
}

func TestTeamInit_NeverOverwrites(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	if _, err := teamInit(repo, wt, pluginRoot, "snake"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".lets", "teams", "snake.md")
	before, _ := os.ReadFile(path)
	_, err := teamInit(repo, wt, pluginRoot, "snake")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitTeamExists {
		t.Fatalf("second init: exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitTeamExists)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("the team file was overwritten")
	}
}

func TestTeamInit_ForeignFileNotOverwritten(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	path := filepath.Join(repo, ".lets", "teams", "snake.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("written by someone else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := teamInit(repo, wt, pluginRoot, "snake")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitTeamExists {
		t.Fatalf("exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitTeamExists)
	}
	if b, _ := os.ReadFile(path); string(b) != "written by someone else\n" {
		t.Errorf("the foreign file changed: %q", b)
	}
}

func TestTeamInit_ConcurrentSameCallsign(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	const n = 8
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := teamInit(repo, wt, pluginRoot, "snake")
			codes[i] = worktreecmd.ExitCode(err)
		}(i)
	}
	wg.Wait()
	ok, exists := 0, 0
	for _, c := range codes {
		switch c {
		case worktreecmd.ExitOK:
			ok++
		case worktreecmd.ExitTeamExists:
			exists++
		}
	}
	if ok != 1 || exists != n-1 {
		t.Errorf("exit codes %v: want exactly one success and %d x %d", codes, n-1, worktreecmd.ExitTeamExists)
	}
	noTempFiles(t, repo)
}

func TestTeamInit_CallsignLive(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "lets-workflow-frog-lead")
	_, err := teamInit(repo, wt, pluginRoot, "frog")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive {
		t.Fatalf("exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams", "frog.md")); !os.IsNotExist(err) {
		t.Error("a team file was written for a live callsign")
	}
}

func TestTeamInit_TemplateMissing(t *testing.T) {
	repo, wt, _ := teamSetup(t)
	_, err := teamInit(repo, wt, realTempDir(t), "snake")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitTemplateMissing {
		t.Fatalf("exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitTemplateMissing)
	}
}

func TestTeamInit_RefusesControlChars(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	for _, o := range []worktreecmd.TeamInitOptions{
		{AgentCommand: "claude\nrm -rf /"},
		{OrcaAgent: "claude\x1b[2J"},
	} {
		o.Callsign, o.Area, o.Worktree, o.PluginRoot = "snake", "the CLI", wt, pluginRoot
		_, err := worktreecmd.TeamInit(context.Background(), repo, o)
		if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
			t.Errorf("%+v: exit %d (%v), want %d", o, worktreecmd.ExitCode(err), err, worktreecmd.ExitUsage)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams", "snake.md")); !os.IsNotExist(err) {
		t.Error("a refused init wrote a team file")
	}
}

func TestTeamInit_SuggestWritesNothing(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, teamRepoSlug+"-"+teamfile.Callsigns[1]+"-lead")
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true})
	if err != nil || !res.Suggested || res.Callsign != teamfile.Callsigns[0] {
		t.Fatalf("suggest: %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams")); !os.IsNotExist(err) {
		t.Error("--suggest-callsign wrote something")
	}
	if _, err := teamInit(repo, wt, pluginRoot, teamfile.Callsigns[0]); err != nil {
		t.Fatal(err)
	}
	res, _ = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true})
	if res.Callsign != teamfile.Callsigns[2] {
		t.Errorf("suggest skipped neither the file nor the live lead: %q", res.Callsign)
	}
}

// A file that appears after the early Lstat - another writer racing this one - is
// refused by the link itself: exit 27, the other writer's bytes untouched.
func TestTeamInit_LinkRefusesLateFile(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	const foreign = "written in the gap by another writer\n"
	worktreecmd.BeforeTeamLink = func(target string) {
		if err := os.WriteFile(target, []byte(foreign), 0o644); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { worktreecmd.BeforeTeamLink = nil })

	_, err := teamInit(repo, wt, pluginRoot, "snake")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitTeamExists {
		t.Fatalf("exit %d (%v), want %d from the link", worktreecmd.ExitCode(err), err, worktreecmd.ExitTeamExists)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, ".lets", "teams", "snake.md")); string(b) != foreign {
		t.Errorf("the late file changed: %q", b)
	}
	noTempFiles(t, repo)
}

// --check verifies a callsign before any worktree exists: it writes nothing and
// refuses exactly like the write does (27 file, 34 live lead, 2 invalid).
func TestTeamInit_Check(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "lets-workflow-frog-lead")
	check := func(c string) error {
		_, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: c, Check: true})
		return err
	}
	if err := check("snake"); err != nil {
		t.Fatalf("a free callsign: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams")); !os.IsNotExist(err) {
		t.Error("--check wrote something")
	}
	if code := worktreecmd.ExitCode(check("frog")); code != worktreecmd.ExitCallsignLive {
		t.Errorf("live lead: exit %d", code)
	}
	if code := worktreecmd.ExitCode(check("Bad")); code != worktreecmd.ExitUsage {
		t.Errorf("invalid callsign: exit %d", code)
	}
	if _, err := teamInit(repo, wt, pluginRoot, "snake"); err != nil {
		t.Fatal(err)
	}
	if code := worktreecmd.ExitCode(check("snake")); code != worktreecmd.ExitTeamExists {
		t.Errorf("existing team file: exit %d", code)
	}
}

// A callsign is unique per repository: the live-lead check compares the
// repo-embedding lead name, so another repo's lead of the same callsign - or a
// bare pre-rename `<callsign>-lead` - never blocks it; this repo's own does (34).
// --suggest-callsign and --check report the lead name too.
func TestTeamInit_CallsignPerRepo(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "other-repo-snake-lead", "frog-lead", "lets-workflow-otter-lead")
	check := func(c string) (*worktreecmd.TeamInitResult, error) {
		return worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: c, Check: true})
	}
	for _, c := range []string{"snake", "frog"} {
		res, err := check(c)
		if err != nil || res.LeadName != "lets-workflow-"+c+"-lead" {
			t.Errorf("%s: another repo's or a bare lead must not block: %+v, %v", c, res, err)
		}
	}
	_, err := check("otter")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive || !strings.Contains(err.Error(), "lets-workflow-otter-lead") {
		t.Errorf("this repo's live lead: exit %d (%v), want %d naming lets-workflow-otter-lead", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
	}
	sug, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true})
	if err != nil || sug.Callsign != "snake" || sug.LeadName != "lets-workflow-snake-lead" {
		t.Errorf("suggest: %+v, %v", sug, err)
	}
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || res.LeadName != "lets-workflow-snake-lead" {
		t.Fatalf("write: %+v, %v", res, err)
	}
	if got, err := teamfile.LeadName(filepath.Join(repo, ".lets", "teams"), "snake"); err != nil || got != "lets-workflow-snake-lead" {
		t.Errorf("the team file records lead_name %q, %v", got, err)
	}
}

// A repo whose slug makes <repo>-<callsign>-lead too long for the session-name
// grammar gets the short <callsign>-lead, and says so in a warn step.
func TestTeamInit_LongRepoShortLeadName(t *testing.T) {
	repo, wt, pluginRoot := teamSetupIn(t, strings.Repeat("r", 60))
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || res.LeadName != "snake-lead" {
		t.Fatalf("write: %+v, %v", res, err)
	}
	warned := false
	for _, s := range res.Steps {
		warned = warned || (s.Status == worktreecmd.StepWarn && strings.Contains(s.Message, "would exceed the session-name grammar; this team's lead is snake-lead"))
	}
	if !warned {
		t.Errorf("no warn step: %+v", res.Steps)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".lets", "teams", "snake.md"))
	if !strings.Contains(string(b), `lead_name: "snake-lead"`) {
		t.Errorf("team file lead_name:\n%s", b)
	}
}

// --launch-mode renders the lead's agent_command and orca_agent at creation; an
// explicit --agent-command wins; an unknown mode is refused with nothing written.
func TestTeamInit_LaunchMode(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "snake", Area: "the CLI", Worktree: wt, PluginRoot: pluginRoot, LaunchMode: "all-in"})
	if err != nil || !res.OK {
		t.Fatalf("all-in: %+v, %v", res, err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".lets", "teams", "snake.md"))
	for _, line := range []string{`agent_command: "claude --permission-mode bypassPermissions"`, `orca_agent: "claude --permission-mode bypassPermissions"`} {
		if !strings.Contains(string(b), line) {
			t.Errorf("team file lacks %s:\n%s", line, b)
		}
	}
	warned := false
	for _, s := range res.Steps {
		warned = warned || (s.Status == worktreecmd.StepWarn && strings.Contains(s.Message, "bypassPermissions"))
	}
	if !warned {
		t.Errorf("an all-in team must say so in a warn step: %+v", res.Steps)
	}
	if res.LaunchClass != worktreecmd.LaunchClassBypass || res.OrcaLaunchClass != worktreecmd.LaunchClassBypass {
		t.Errorf("all-in: launch_class %q, orca_launch_class %q; want bypass for both", res.LaunchClass, res.OrcaLaunchClass)
	}

	res, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "frog", Area: "docs", Worktree: wt, PluginRoot: pluginRoot, LaunchMode: "auto", AgentCommand: "docker compose run dev claude"})
	if err != nil || !res.OK {
		t.Fatalf("explicit agent: %+v, %v", res, err)
	}
	b, _ = os.ReadFile(filepath.Join(repo, ".lets", "teams", "frog.md"))
	if !strings.Contains(string(b), `agent_command: "docker compose run dev claude"`) || !strings.Contains(string(b), `orca_agent: "claude --permission-mode auto"`) {
		t.Errorf("an explicit --agent-command must win, orca_agent rendered:\n%s", b)
	}
	if res.LaunchClass != worktreecmd.LaunchClassDefault || res.OrcaLaunchClass != worktreecmd.LaunchClassAuto {
		t.Errorf("explicit agent: launch_class %q (want default - classified from the written command, not the mode), orca_launch_class %q (want auto)", res.LaunchClass, res.OrcaLaunchClass)
	}

	// A hand-given bypass command is classified too, and a --settings override warns.
	res, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "heron", Area: "x", Worktree: wt, PluginRoot: pluginRoot, AgentCommand: `claude --settings '{"permissions":{"defaultMode":"bypassPermissions"}}'`})
	if err != nil || !res.OK {
		t.Fatalf("settings agent: %+v, %v", res, err)
	}
	if res.LaunchClass != worktreecmd.LaunchClassBypass || !slices.Equal(res.LaunchWarnings, []string{worktreecmd.LaunchWarnSettingsOverride}) {
		t.Errorf("settings agent: launch_class %q, warnings %v; want bypass, [settings_override]", res.LaunchClass, res.LaunchWarnings)
	}
	warned = false
	for _, s := range res.Steps {
		warned = warned || (s.Status == worktreecmd.StepWarn && strings.Contains(s.Message, "agent_command: settings_override"))
	}
	if !warned {
		t.Errorf("a --settings agent_command must warn: %+v", res.Steps)
	}

	_, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "otter", Area: "x", Worktree: wt, PluginRoot: pluginRoot, LaunchMode: "bypassPermissions"})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("unknown mode: exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitUsage)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams", "otter.md")); !os.IsNotExist(err) {
		t.Error("an unknown launch mode wrote a team file")
	}
}

// A main checkout whose basename yields no repo slug gets the short <callsign>-lead,
// and the warn step names that cause, not the grammar.
func TestTeamInit_NoRepoSlugShortLeadName(t *testing.T) {
	repo, wt, pluginRoot := teamSetupIn(t, "___")
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || res.LeadName != "snake-lead" {
		t.Fatalf("write: %+v, %v", res, err)
	}
	warned := false
	for _, s := range res.Steps {
		if s.Status != worktreecmd.StepWarn {
			continue
		}
		if strings.Contains(s.Message, "session-name grammar") {
			t.Errorf("a missing slug is not a grammar problem: %q", s.Message)
		}
		warned = warned || strings.Contains(s.Message, `no repo slug derives from "___"; this team's lead is snake-lead`)
	}
	if !warned {
		t.Errorf("no warn step naming the missing slug: %+v", res.Steps)
	}
}

// team-init grants the lead's allow rules right after the team file is linked
// (lets-qysjv, ported by lets-gbfpd): an ok step names both rules and the revoke.
func TestTeamInit_GrantsAllowRules(t *testing.T) {
	isolateGitIgnore(t)
	repo, wt, pluginRoot := teamSetup(t)
	mustMkdir(t, filepath.Join(repo, ".lets"))
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || !res.OK {
		t.Fatalf("team-init: %v %+v", err, res)
	}
	if got := settingsAllow(t, wt); strings.Join(got, "|") != strings.Join(worktreecmd.TeamAllowRules(), "|") {
		t.Errorf("allow = %v", got)
	}
	granted := false
	for i, s := range res.Steps {
		if s.Status == worktreecmd.StepOK && s.Message == allowAdded {
			granted = true
			if i == 0 || !strings.HasPrefix(res.Steps[i-1].Message, "team file written: ") {
				t.Errorf("the grant must follow the team file write: %+v", res.Steps)
			}
		}
	}
	if !granted {
		t.Errorf("no ok step %q: %+v", allowAdded, res.Steps)
	}
	// A second team-init on the same worktree (the team file removed, the team created
	// again) does not duplicate a rule.
	if err := os.Remove(res.TeamFile); err != nil {
		t.Fatal(err)
	}
	again, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || !again.OK {
		t.Fatalf("second team-init: %v %+v", err, again)
	}
	if got := settingsAllow(t, wt); strings.Join(got, "|") != strings.Join(worktreecmd.TeamAllowRules(), "|") {
		t.Errorf("a second team-init duplicated a rule: %v", got)
	}
	if !stepsContain(again.Steps, "already lists Bash(lets worktree switch:*), Bash(lets worktree task-state:*)") {
		t.Errorf("the second grant must be a skip naming the rules: %+v", again.Steps)
	}
}

// A grant that fails is a warn step, never a failed team-init; a tracked
// settings.local.json is never written.
func TestTeamInit_GrantFailureIsAStep(t *testing.T) {
	isolateGitIgnore(t)
	repo, wt, pluginRoot := teamSetup(t)
	mustWrite(t, settingsPath(wt), "{}\n", 0o644)
	runIn(t, wt, "git", "add", "-f", ".claude/settings.local.json")
	runIn(t, wt, "git", "commit", "-q", "-m", "track it")
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || !res.OK {
		t.Fatalf("team-init must not fail on a grant warning: %v %+v", err, res)
	}
	warned := false
	for _, s := range res.Steps {
		warned = warned || (s.Status == worktreecmd.StepWarn && strings.HasPrefix(s.Message, worktreecmd.SettingsLocalTracked+": "))
	}
	if !warned {
		t.Errorf("no warn step for the tracked file: %+v", res.Steps)
	}
	if data, _ := os.ReadFile(settingsPath(wt)); string(data) != "{}\n" {
		t.Errorf("the tracked file was rewritten: %s", data)
	}
}
