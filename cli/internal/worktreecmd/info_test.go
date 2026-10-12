//go:build unix

package worktreecmd_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestInfo_InMain(t *testing.T) {
	repo := initRepo(t)
	res, err := worktreecmd.Info(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if res.InWorktree {
		t.Error("expected in_worktree=false")
	}
	if res.MainRoot != repo {
		t.Errorf("MainRoot=%s, want %s", res.MainRoot, repo)
	}
}

func TestInfo_InWorktree(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "x", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	res, _ := worktreecmd.Info(context.Background(), cr.Worktree.Path)
	if !res.InWorktree {
		t.Error("expected in_worktree=true")
	}
	if res.MainRoot != repo {
		t.Errorf("MainRoot=%s, want %s", res.MainRoot, repo)
	}
	// ProjectRoot semantics: pinned to MainRoot in worktree mode so the
	// JSON envelope's "project_root" stays a stable handle across worktree
	// switches. A silent flip to cr.Worktree.Path here would break every
	// consumer (LETS_PROJECT_ROOT injection, statusline) that treats
	// project_root as the main repo path. Catch the drift early.
	if res.ProjectRoot != repo {
		t.Errorf("ProjectRoot=%s, want %s (must equal MainRoot in worktree mode)", res.ProjectRoot, repo)
	}
	if res.Worktree == nil || res.Worktree.Branch != "worktree-x" {
		t.Errorf("expected branch worktree-x, got %+v", res.Worktree)
	}
}

// Info called from a subdirectory of the worktree must resolve `dir` to the
// worktree root before probing for the .lets / .beads/.env symlinks; otherwise
// the symlink probes look in the wrong place and report LetsSymlinked=false on
// a perfectly valid worktree.
func TestInfo_FromSubdir(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "x", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(cr.Worktree.Path, "deep", "nest")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := worktreecmd.Info(context.Background(), sub)
	if err != nil || !res.OK {
		t.Fatalf("Info from subdir failed: err=%v ok=%v", err, res.OK)
	}
	if !res.InWorktree {
		t.Error("expected in_worktree=true")
	}
	if res.Worktree == nil || res.Worktree.Path != cr.Worktree.Path {
		t.Errorf("Worktree.Path = %v, want %s (subdir must resolve to worktree root)", res.Worktree, cr.Worktree.Path)
	}
	if res.Worktree == nil || !res.Worktree.LetsSymlinked {
		t.Errorf("LetsSymlinked = false on a valid worktree (subdir resolution probably broken)")
	}
}

func TestInfo_OutsideRepo(t *testing.T) {
	// Use a temp dir that's NOT in a git repo (its parent isn't either).
	tmp := t.TempDir()
	_, err := worktreecmd.Info(context.Background(), tmp)
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitNotInRepo {
		t.Errorf("want ExitNotInRepo, got %v", err)
	}
}

// info.team names the standing team whose file claims this worktree; a stale team
// file (the path was reused) is reported as a warn step, never fatal and never a team.
func TestInfo_Team(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "team_snake", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	wt := cr.Worktree.Path
	gitDir := strings.TrimSpace(gitOutput(t, wt, "rev-parse", "--absolute-git-dir"))
	teamPath := filepath.Join(repo, ".lets", "teams", "snake.md")
	write := func(body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(teamPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(teamPath, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// No team file: no team, no warning.
	res, err := worktreecmd.Info(context.Background(), wt)
	if err != nil || res.Team != "" || res.TeamLead != nil || len(res.Steps) != 0 {
		t.Fatalf("no team file: err=%v team=%q team_lead=%+v steps=%+v", err, res.Team, res.TeamLead, res.Steps)
	}

	write("---\nteam: \"snake\"\nworktree: \"" + wt + "\"\ngit_dir: \"" + gitDir + "\"\n---\n")
	res, err = worktreecmd.Info(context.Background(), wt)
	if err != nil || !res.OK || res.Team != "snake" {
		t.Fatalf("team file: err=%v ok=%v team=%q steps=%+v", err, res.OK, res.Team, res.Steps)
	}
	raw, _ := json.Marshal(res)
	if !strings.Contains(string(raw), `"team":"snake"`) {
		t.Errorf("json lacks team: %s", raw)
	}
	// The main checkout is not the team's worktree; team stays out of the JSON.
	res, _ = worktreecmd.Info(context.Background(), repo)
	raw, _ = json.Marshal(res)
	if res.Team != "" || strings.Contains(string(raw), `"team"`) || strings.Contains(string(raw), `"team_lead"`) {
		t.Errorf("main checkout: team=%q json=%s", res.Team, raw)
	}

	write("---\nteam: \"snake\"\nworktree: \"" + wt + "\"\ngit_dir: \"" + filepath.Join(repo, ".git", "worktrees", "gone") + "\"\n---\n")
	res, err = worktreecmd.Info(context.Background(), wt)
	if err != nil || !res.OK || res.Team != "" {
		t.Fatalf("stale team file: err=%v ok=%v team=%q", err, res.OK, res.Team)
	}
	if len(res.Steps) != 1 || res.Steps[0].Status != worktreecmd.StepWarn || !strings.Contains(res.Steps[0].Message, "stale team") {
		t.Errorf("stale team file: steps=%+v, want one warn naming the stale team", res.Steps)
	}
}

// info.team_lead compares the team file's lead_name with this session's registry
// name: lead, other, or unknown (no session id, no registry, not listed). A team
// file without lead_name names <team>-lead.
func TestInfo_TeamLead(t *testing.T) {
	const sid = "aaaaaaaa-0000-4000-8000-0000000000aa"
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "team_snake", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	wt := cr.Worktree.Path
	gitDir := strings.TrimSpace(gitOutput(t, wt, "rev-parse", "--absolute-git-dir"))
	teamFile := func(leadLine string) {
		t.Helper()
		p := filepath.Join(repo, ".lets", "teams", "snake.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nteam: \"snake\"\nworktree: \"" + wt + "\"\ngit_dir: \"" + gitDir + "\"\n" + leadLine + "---\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// registry lists this test process (a live pid) as sid, named name ("" = unnamed).
	registry := func(name string) string {
		t.Helper()
		cfg := t.TempDir()
		row := `{"sessionId":"` + sid + `","cwd":"/x","peerProtocol":1,"status":"idle","startedAt":1}`
		if name != "" {
			row = `{"sessionId":"` + sid + `","name":"` + name + `","cwd":"/x","peerProtocol":1,"status":"idle","startedAt":1}`
		}
		if err := os.MkdirAll(filepath.Join(cfg, "sessions"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, "sessions", strconv.Itoa(os.Getpid())+".json"), []byte(row), 0o600); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	// degraded also lists the parent process (live) under an unknown peerProtocol.
	degraded := func() string {
		cfg := registry("lets-workflow-snake-lead")
		row := `{"sessionId":"cccccccc-0000-4000-8000-0000000000cc","peerProtocol":2}`
		if err := os.WriteFile(filepath.Join(cfg, "sessions", strconv.Itoa(os.Getppid())+".json"), []byte(row), 0o600); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	cases := []struct {
		name     string
		leadLine string
		env      string
		cfg      string
		want     worktreecmd.TeamLead
	}{
		{"named row is the lead", "lead_name: \"lets-workflow-snake-lead\"\n", sid, registry("lets-workflow-snake-lead"), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Self: "lets-workflow-snake-lead", Match: "lead"}},
		{"another name", "lead_name: \"lets-workflow-snake-lead\"\n", sid, registry("snake-architect"), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Self: "snake-architect", Match: "other"}},
		{"unnamed row", "lead_name: \"lets-workflow-snake-lead\"\n", sid, registry(""), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Match: "other"}},
		{"no session id", "lead_name: \"lets-workflow-snake-lead\"\n", "", registry("lets-workflow-snake-lead"), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Match: "unknown"}},
		{"registry absent", "lead_name: \"lets-workflow-snake-lead\"\n", sid, t.TempDir(), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Match: "unknown"}},
		{"registry degraded", "lead_name: \"lets-workflow-snake-lead\"\n", sid, degraded(), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Match: "unknown"}},
		{"session not listed", "lead_name: \"lets-workflow-snake-lead\"\n", "bbbbbbbb-0000-4000-8000-0000000000bb", registry("lets-workflow-snake-lead"), worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Match: "unknown"}},
		{"no lead_name recorded", "", sid, registry("snake-lead"), worktreecmd.TeamLead{Name: "snake-lead", Self: "snake-lead", Match: "lead"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			teamFile(c.leadLine)
			t.Setenv("CLAUDE_CODE_SESSION_ID", c.env)
			t.Setenv("CLAUDE_CONFIG_DIR", c.cfg)
			res, err := worktreecmd.Info(context.Background(), wt)
			if err != nil || res.Team != "snake" || res.TeamLead == nil || *res.TeamLead != c.want {
				t.Fatalf("team=%q team_lead=%+v err=%v, want %+v", res.Team, res.TeamLead, err, c.want)
			}
		})
	}
}

// The info envelope carries team_lead with name and match (self omitted when
// empty) - the schema contract for a team worktree, additive to info's keys.
func TestInfo_TeamLeadSchema(t *testing.T) {
	r := &worktreecmd.InfoResult{
		Envelope: worktreecmd.Envelope{SchemaVersion: worktreecmd.SchemaVersion, OK: true, Subcommand: "info", ProjectRoot: "/p", Steps: []worktreecmd.Step{}},
		MainRoot: "/p", Team: "snake", TeamLead: &worktreecmd.TeamLead{Name: "lets-workflow-snake-lead", Match: "unknown"},
	}
	b, _ := json.Marshal(r)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	tl, ok := m["team_lead"].(map[string]any)
	if !ok || m["team"] != "snake" {
		t.Fatalf("team_lead missing: %s", b)
	}
	for _, k := range []string{"name", "match"} {
		if _, ok := tl[k]; !ok {
			t.Errorf("team_lead lacks %q: %s", k, b)
		}
	}
	if _, ok := tl["self"]; ok {
		t.Errorf("an empty self must be omitted: %s", b)
	}
	if worktreecmd.SchemaVersion != 1 {
		t.Errorf("team_lead is additive; SchemaVersion = %d", worktreecmd.SchemaVersion)
	}
}
