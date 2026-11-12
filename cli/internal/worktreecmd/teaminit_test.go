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
	for _, line := range []string{`team: "snake"`, `area: "the CLI"`, `worktree: "` + wt + `"`, `lead_name: "snake-lead"`, `agent_command: "claude"`, `orca_agent: "claude"`, `git_dir: "`} {
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

// Both the short and the repo-prefixed lead name held -> 34 callsign_live.
func TestTeamInit_CallsignLive(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "frog-lead", "lets-workflow-frog-lead")
	res, err := teamInit(repo, wt, pluginRoot, "frog")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive || res.Error == nil || res.Error.Kind != "callsign_live" {
		t.Fatalf("exit %d (%v), want %d callsign_live", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
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
	repo, wt, pluginRoot := teamSetup(t, teamfile.Callsigns[1]+"-lead", teamRepoSlug+"-"+teamfile.Callsigns[1]+"-lead")
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
	repo, wt, pluginRoot := teamSetup(t, "frog-lead", "lets-workflow-frog-lead")
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

// lets-rpufs: a new team's lead is the short <callsign>-lead; a live holder of the
// short name (anywhere on the machine) moves it to <repo>-<callsign>-lead with one
// warn step; neither free -> 34. --suggest-callsign and --check report the name too.
func TestTeamInit_ShortLeadNameAndFallback(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "other-repo-snake-lead", "frog-lead", "otter-lead", "lets-workflow-otter-lead")
	check := func(c string) (*worktreecmd.TeamInitResult, error) {
		return worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: c, Check: true})
	}
	res, err := check("snake")
	if err != nil || res.LeadName != "snake-lead" || hasWarn(res.Steps) {
		t.Errorf("snake: want snake-lead with no warn step: %+v, %v", res, err)
	}
	res, err = check("frog")
	if err != nil || res.LeadName != "lets-workflow-frog-lead" || !warnContains(res.Steps, "the lead name frog-lead is held by a live session") {
		t.Errorf("frog: want the repo-prefixed fallback with a warn step: %+v, %v", res, err)
	}
	_, err = check("otter")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive || !strings.Contains(err.Error(), "otter-lead") {
		t.Errorf("otter: exit %d (%v), want %d naming otter-lead", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
	}
	sug, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true})
	if err != nil || sug.Callsign != "snake" || sug.LeadName != "snake-lead" {
		t.Errorf("suggest: %+v, %v", sug, err)
	}
	res, err = teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || res.LeadName != "snake-lead" {
		t.Fatalf("write: %+v, %v", res, err)
	}
	if got, err := teamfile.LeadName(filepath.Join(repo, ".lets", "teams"), "snake"); err != nil || got != "snake-lead" {
		t.Errorf("the team file records lead_name %q, %v", got, err)
	}
}

// A repo whose slug makes <repo>-<callsign>-lead too long gets the short name when
// it is free, and no fallback when it is held (34).
func TestTeamInit_LongRepoNoFallback(t *testing.T) {
	repo, wt, pluginRoot := teamSetupIn(t, strings.Repeat("r", 60))
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || res.LeadName != "snake-lead" || hasWarn(res.Steps) {
		t.Fatalf("write: %+v, %v", res, err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".lets", "teams", "snake.md"))
	if !strings.Contains(string(b), `lead_name: "snake-lead"`) {
		t.Errorf("team file lead_name:\n%s", b)
	}

	repo, _, _ = teamSetupIn(t, strings.Repeat("r", 60), "snake-lead")
	_, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "snake", Check: true})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive {
		t.Errorf("held short name, long form too long: exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
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

// A main checkout whose basename yields no repo slug gets the short <callsign>-lead
// with no warn step; a held short name has no fallback (34).
func TestTeamInit_NoRepoSlugShortLeadName(t *testing.T) {
	repo, wt, pluginRoot := teamSetupIn(t, "___")
	res, err := teamInit(repo, wt, pluginRoot, "snake")
	if err != nil || res.LeadName != "snake-lead" || hasWarn(res.Steps) {
		t.Fatalf("write: %+v, %v", res, err)
	}

	repo, _, _ = teamSetupIn(t, "___", "snake-lead")
	_, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "snake", Check: true})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive {
		t.Errorf("held short name, no slug: exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
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

// hasWarn reports whether steps carry any warn step.
func hasWarn(steps []worktreecmd.Step) bool {
	for _, s := range steps {
		if s.Status == worktreecmd.StepWarn {
			return true
		}
	}
	return false
}

// warnContains reports whether a warn step's message contains sub.
func warnContains(steps []worktreecmd.Step, sub string) bool {
	for _, s := range steps {
		if s.Status == worktreecmd.StepWarn && strings.Contains(s.Message, sub) {
			return true
		}
	}
	return false
}

// teamInitLead is teamInit with an explicit --lead-name.
func teamInitLead(repo, wt, pluginRoot, callsign, lead string) (*worktreecmd.TeamInitResult, error) {
	return worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: callsign, Area: "the CLI", Worktree: wt, PluginRoot: pluginRoot, LeadName: lead})
}

// checkLead is --check with an explicit --lead-name.
func checkLead(repo, callsign, lead string) (*worktreecmd.TeamInitResult, error) {
	return worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: callsign, Check: true, LeadName: lead})
}

// writeTeamFile writes .lets/teams/<team>.md by hand with the given frontmatter body.
func writeTeamFile(t *testing.T, repo, team, frontmatter string) {
	t.Helper()
	dir := filepath.Join(repo, ".lets", "teams")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nteam: \"" + team + "\"\n" + frontmatter + "---\n# team " + team + "\n"
	if err := os.WriteFile(filepath.Join(dir, team+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func noTeamFile(t *testing.T, repo, team string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams", team+".md")); !os.IsNotExist(err) {
		t.Errorf("a team file was written for %s", team)
	}
}

// wantRefusal asserts exit code and error kind on a TeamInit result.
func wantRefusal(t *testing.T, what string, res *worktreecmd.TeamInitResult, err error, code int, kind string) {
	t.Helper()
	if worktreecmd.ExitCode(err) != code || res == nil || res.Error == nil || res.Error.Kind != kind {
		var got string
		if res != nil && res.Error != nil {
			got = res.Error.Kind
		}
		t.Errorf("%s: exit %d kind %q (%v), want %d %s", what, worktreecmd.ExitCode(err), got, err, code, kind)
	}
}

// --lead-name (lets-rpufs): recorded as given; invalid -> 2; held by a live
// session -> 34 lead_name_live, never replaced; with --suggest-callsign -> 2.
func TestTeamInit_LeadNameFlag(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	res, err := teamInitLead(repo, wt, pluginRoot, "snake", "pwa-lead")
	if err != nil || res.LeadName != "pwa-lead" || hasWarn(res.Steps) {
		t.Fatalf("--lead-name pwa-lead: %+v, %v", res, err)
	}
	if got, err := teamfile.LeadName(filepath.Join(repo, ".lets", "teams"), "snake"); err != nil || got != "pwa-lead" {
		t.Errorf("the team file records lead_name %q, %v", got, err)
	}

	res, err = teamInitLead(repo, wt, pluginRoot, "frog", "bad name!")
	wantRefusal(t, "invalid --lead-name", res, err, worktreecmd.ExitUsage, "lead_name_invalid")
	noTeamFile(t, repo, "frog")

	repo, wt, pluginRoot = teamSetup(t, "pwa-lead")
	res, err = checkLead(repo, "snake", "pwa-lead")
	wantRefusal(t, "check, live --lead-name", res, err, worktreecmd.ExitCallsignLive, "lead_name_live")
	res, err = teamInitLead(repo, wt, pluginRoot, "snake", "pwa-lead")
	wantRefusal(t, "write, live --lead-name", res, err, worktreecmd.ExitCallsignLive, "lead_name_live")
	if res != nil && res.LeadName != "pwa-lead" {
		t.Errorf("a held --lead-name was replaced: %q", res.LeadName)
	}
	noTeamFile(t, repo, "snake")

	_, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true, LeadName: "x"})
	if worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("--suggest-callsign --lead-name: exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitUsage)
	}
	if _, err := os.Stat(filepath.Join(repo, ".lets", "teams")); !os.IsNotExist(err) {
		t.Error("--suggest-callsign --lead-name wrote something")
	}
}

// A callsign whose short lead name is no session name (`-frog-lead`) falls back to
// the repo-prefixed name with the real cause in the warn step; with no repo slug it
// is refused 34 naming that cause - never "held by a live session".
func TestTeamInit_LeadingHyphenCallsign(t *testing.T) {
	repo, _, _ := teamSetup(t)
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "-frog", Check: true})
	if err != nil || res.LeadName != "lets-workflow--frog-lead" || !warnContains(res.Steps, "the lead name -frog-lead is not a valid session name; this team's lead is lets-workflow--frog-lead") || warnContains(res.Steps, "held by a live session") {
		t.Errorf("-frog: %+v, %v", res, err)
	}

	repo, _, _ = teamSetupIn(t, "___")
	res, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "-frog", Check: true})
	wantRefusal(t, "-frog, no slug", res, err, worktreecmd.ExitCallsignLive, "callsign_live")
	if err == nil || !strings.Contains(err.Error(), "is not a valid session name") || strings.Contains(err.Error(), "held by a live session") {
		t.Errorf("-frog, no slug: message %v must name the invalid name", err)
	}
}

// An explicit --lead-name another team file of this repo records is refused 34
// lead_name_taken, naming that team.
func TestTeamInit_LeadNameTaken(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	writeTeamFile(t, repo, "gold", "lead_name: \"pwa-lead\"\n")
	res, err := checkLead(repo, "snake", "pwa-lead")
	wantRefusal(t, "check", res, err, worktreecmd.ExitCallsignLive, "lead_name_taken")
	if err == nil || !strings.Contains(err.Error(), "gold") {
		t.Errorf("check: %v must name team gold", err)
	}
	res, err = teamInitLead(repo, wt, pluginRoot, "snake", "pwa-lead")
	wantRefusal(t, "write", res, err, worktreecmd.ExitCallsignLive, "lead_name_taken")
	if err == nil || !strings.Contains(err.Error(), "gold") {
		t.Errorf("write: %v must name team gold", err)
	}
	noTeamFile(t, repo, "snake")
}

// A short name another team file records is skipped like a live holder; a legacy
// team file without lead_name records <team>-lead.
func TestTeamInit_ShortRecordedByOtherTeam(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	writeTeamFile(t, repo, "gold", "lead_name: \"snake-lead\"\n")
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "snake", Check: true})
	if err != nil || res.LeadName != "lets-workflow-snake-lead" || !warnContains(res.Steps, "the lead name snake-lead is recorded by team gold") {
		t.Errorf("snake: %+v, %v", res, err)
	}

	writeTeamFile(t, repo, "otter", "area: \"legacy\"\n")
	res, err = checkLead(repo, "snake", "otter-lead")
	wantRefusal(t, "check, legacy otter-lead", res, err, worktreecmd.ExitCallsignLive, "lead_name_taken")
	if err == nil || !strings.Contains(err.Error(), "otter") {
		t.Errorf("check: %v must name team otter", err)
	}
	res, err = teamInitLead(repo, wt, pluginRoot, "snake", "otter-lead")
	wantRefusal(t, "write, legacy otter-lead", res, err, worktreecmd.ExitCallsignLive, "lead_name_taken")
	noTeamFile(t, repo, "snake")
}

// Refusal order: usage (2) before team_exists (27) before the lead-name refusals (34),
// in --check and in the write.
func TestTeamInit_RefusalPrecedence(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "pwa-lead")
	writeTeamFile(t, repo, "snake", "lead_name: \"snake-lead\"\n")
	for _, mode := range []string{"check", "write"} {
		run := func(c, lead string) (*worktreecmd.TeamInitResult, error) {
			if mode == "check" {
				return checkLead(repo, c, lead)
			}
			return teamInitLead(repo, wt, pluginRoot, c, lead)
		}
		res, err := run("snake", "bad name!")
		wantRefusal(t, mode+": invalid --lead-name + existing file", res, err, worktreecmd.ExitUsage, "lead_name_invalid")
		res, err = run("snake", "pwa-lead")
		wantRefusal(t, mode+": existing file + held --lead-name", res, err, worktreecmd.ExitTeamExists, "team_exists")
		res, err = run("frog", "pwa-lead")
		wantRefusal(t, mode+": held --lead-name, no file", res, err, worktreecmd.ExitCallsignLive, "lead_name_live")
	}
	noTeamFile(t, repo, "frog")
}

// A degraded registry keeps its warn step, and a readable holder of the short name
// still moves the lead to the repo-prefixed name.
func TestTeamInit_DegradedRegistryWarns(t *testing.T) {
	repo, _, _ := teamSetup(t, "snake-lead", "placeholder")
	// pid 1001 is live in the fixture: replace its row with an unknown protocol.
	row := `{"sessionId":"cccccccc-0000-4000-8000-0000000000cc","peerProtocol":2}`
	if err := os.WriteFile(filepath.Join(ccregistry.HomeDir(), "sessions", "1001.json"), []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "snake", Check: true})
	if err != nil || res.LeadName != "lets-workflow-snake-lead" {
		t.Fatalf("degraded: %+v, %v", res, err)
	}
	if !warnContains(res.Steps, "session registry degraded") {
		t.Errorf("no degraded-registry warn step: %+v", res.Steps)
	}
	if !warnContains(res.Steps, "the lead name snake-lead is held by a live session") {
		t.Errorf("no fallback warn step: %+v", res.Steps)
	}
}

// A team file whose lead name cannot be read (a duplicate lead_name key) is a warn
// step, never a refusal: a free callsign still passes --check.
func TestTeamInit_UnreadableTeamFileWarns(t *testing.T) {
	repo, _, _ := teamSetup(t)
	writeTeamFile(t, repo, "gold", "lead_name: \"a-lead\"\nlead_name: \"b-lead\"\n")
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Callsign: "snake", Check: true})
	if err != nil || !res.OK {
		t.Fatalf("check: %+v, %v", res, err)
	}
	if !warnContains(res.Steps, "is unreadable") || !warnContains(res.Steps, "its lead name is not checked") {
		t.Errorf("no unreadable-team-file warn step: %+v", res.Steps)
	}
}

// --suggest-callsign uses the same rule as --check: a held short name with a free
// repo-prefixed one still proposes the callsign; both held (one recorded by another
// team file, one live) skips it.
func TestTeamInit_SuggestLeadFallback(t *testing.T) {
	c := teamfile.Callsigns[0]
	repo, _, _ := teamSetup(t, c+"-lead")
	res, err := worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true})
	if err != nil || res.Callsign != c || res.LeadName != teamRepoSlug+"-"+c+"-lead" {
		t.Errorf("short held, long free: %+v, %v", res, err)
	}

	repo, _, _ = teamSetup(t, teamRepoSlug+"-"+c+"-lead")
	writeTeamFile(t, repo, "zzteam", "lead_name: \""+c+"-lead\"\n")
	res, err = worktreecmd.TeamInit(context.Background(), repo, worktreecmd.TeamInitOptions{Suggest: true})
	if err != nil || res.Callsign == c {
		t.Errorf("short recorded, long live: suggest must skip %s: %+v, %v", c, res, err)
	}
}

// An explicit --lead-name that reads as this repo's orchestrator name is refused 2
// lead_name_invalid; a name that only starts with the same letters is accepted.
func TestTeamInit_LeadNameOrchestratorRefused(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t)
	for _, n := range []string{"lets-workflow-orc", "lets-workflow-orc-x"} {
		res, err := teamInitLead(repo, wt, pluginRoot, "snake", n)
		wantRefusal(t, n, res, err, worktreecmd.ExitUsage, "lead_name_invalid")
		noTeamFile(t, repo, "snake")
	}
	res, err := teamInitLead(repo, wt, pluginRoot, "snake", "lets-workflow-orchid-lead")
	if err != nil || !res.OK || res.LeadName != "lets-workflow-orchid-lead" {
		t.Errorf("lets-workflow-orchid-lead: %+v, %v", res, err)
	}
}

// A derived lead name that reads as this repo's orchestrator name is never used:
// callsign `orc` falls back to lets-workflow-orc-lead (= orchestrator scope "lead"),
// so with orc-lead live there is no usable name (34); callsign `lets-workflow-orc`
// has that problem in its short name and takes the repo-prefixed one, saying why.
func TestTeamInit_DerivedOrchestratorNameSkipped(t *testing.T) {
	repo, _, _ := teamSetup(t, "orc-lead")
	_, err := checkLead(repo, "orc", "")
	if worktreecmd.ExitCode(err) != worktreecmd.ExitCallsignLive {
		t.Errorf("orc with orc-lead live: exit %d (%v), want %d", worktreecmd.ExitCode(err), err, worktreecmd.ExitCallsignLive)
	}
	res, err := checkLead(repo, "lets-workflow-orc", "")
	if err != nil || res.LeadName != "lets-workflow-lets-workflow-orc-lead" || !warnContains(res.Steps, "reads as an orchestrator name of this repository") {
		t.Errorf("orchestrator-like short name: %+v, %v", res, err)
	}
}
