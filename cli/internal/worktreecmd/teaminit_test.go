//go:build unix

package worktreecmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// teamSetup is a repo with a linked team worktree, a plugin root holding the real
// team template, and an empty session registry whose live names the test sets.
func teamSetup(t *testing.T, liveNames ...string) (repo, wt, pluginRoot string) {
	t.Helper()
	repo = initRepo(t)
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

func TestTeamInit_CallsignLive(t *testing.T) {
	repo, wt, pluginRoot := teamSetup(t, "frog-lead")
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
	repo, wt, pluginRoot := teamSetup(t, teamfile.Callsigns[1]+"-lead")
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
