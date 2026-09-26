//go:build unix

package orcacmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// teamRepo is a main checkout on main pushed to a bare origin, and a separate dir
// standing in for Orca's workspace directory.
type teamRepo struct{ repo, origin, ws string }

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func realTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func newTeamRepo(t *testing.T) teamRepo {
	t.Helper()
	r := teamRepo{repo: realTemp(t), origin: filepath.Join(realTemp(t), "origin.git"), ws: realTemp(t)}
	gitT(t, r.repo, "-c", "init.defaultBranch=main", "init", "-q")
	gitT(t, r.repo, "config", "user.email", "t@e")
	gitT(t, r.repo, "config", "user.name", "t")
	gitT(t, r.repo, "commit", "-q", "--allow-empty", "-m", "base")
	gitT(t, r.repo, "init", "-q", "--bare", "-b", "main", r.origin)
	gitT(t, r.repo, "remote", "add", "origin", r.origin)
	gitT(t, r.repo, "push", "-q", "-u", "origin", "main")
	return r
}

// fakeTeamOrca is a stateful fake Orca: ps rows are JSON objects, create / rm run
// the given callbacks, terminal list returns terms.
type fakeTeamOrca struct {
	rows    []string
	terms   string
	create  func(args []string) (string, bool)
	rm      func(args []string) (string, bool)
	psFails bool
	calls   *[]fakeCall
}

func (r teamRepo) fake(t *testing.T, f *fakeTeamOrca) *fakeTeamOrca {
	t.Helper()
	if f.terms == "" {
		f.terms = `{"ok":true,"result":{"terminals":[]}}`
	}
	f.calls = useFakeOrca(t, func(args []string) (string, string, bool) {
		switch {
		case joined(args) == "status --json":
			return statusRunning, "", false
		case joined(args) == "worktree ps --json":
			if f.psFails {
				return "", "boom", true
			}
			return `{"ok":true,"result":{"worktrees":[` + strings.Join(f.rows, ",") + `]}}`, "", false
		case joined(args) == "terminal list --json":
			return f.terms, "", false
		case len(args) > 1 && args[0] == "worktree" && args[1] == "create":
			out, fail := f.create(args)
			return out, "", fail
		case len(args) > 1 && args[0] == "worktree" && args[1] == "rm":
			out, fail := f.rm(args)
			return out, "", fail
		}
		return "{}", "", false
	})
	return f
}

// orcaAdd simulates Orca creating the worktree: git worktree add at <ws>/<name> on
// branch branch from start, optionally listed by ps.
func (r teamRepo) orcaAdd(t *testing.T, f *fakeTeamOrca, name, branch, start string, list bool) string {
	t.Helper()
	p := filepath.Join(r.ws, name)
	gitT(t, r.repo, "worktree", "add", "-q", "-b", branch, p, start)
	if list {
		f.rows = append(f.rows, fmt.Sprintf(`{"worktreeId":"r::%s","path":%q,"branch":"refs/heads/%s","displayName":%q}`, p, p, branch, name))
	}
	return p
}

func (f *fakeTeamOrca) count(verb string) int {
	n := 0
	for _, c := range *f.calls {
		if strings.HasPrefix(joined(c.args), verb) {
			n++
		}
	}
	return n
}

func teamCreate(r teamRepo) (*TeamCreateResult, error) {
	return TeamCreate(context.Background(), TeamCreateOptions{Repo: r.repo, Name: "team_snake", BaseBranch: "origin/main"})
}

func wantExit(t *testing.T, err error, code int, kind string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code || e.Kind != kind {
		t.Fatalf("err = %v, want exit %d kind %s", err, code, kind)
	}
}

func TestTeamCreate_Argv(t *testing.T) {
	r := newTeamRepo(t)
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) {
		r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", true)
		return `{"ok":true}`, false
	}})
	res, err := teamCreate(r)
	if err != nil || res.Team.State != TeamCreated || !res.Team.Created || !res.Team.Attempted {
		t.Fatalf("create: %+v, %v", res.Team, err)
	}
	want := []string{"worktree", "create", "--repo", "path:" + r.repo, "--name", "team_snake", "--base-branch", "origin/main", "--setup", "skip", "--no-parent", "--json"}
	for _, c := range *f.calls {
		if len(c.args) > 1 && c.args[1] == "create" {
			if !slices.Equal(c.args, want) {
				t.Errorf("argv = %q\nwant  %q", c.args, want)
			}
			if slices.Contains(c.args, "--agent") || slices.Contains(c.args, "--prompt") {
				t.Error("team-create must pass no agent and no prompt")
			}
		}
	}
}

func TestTeamCreate_RefusesNonTeamName(t *testing.T) {
	r := newTeamRepo(t)
	f := r.fake(t, &fakeTeamOrca{})
	for _, n := range []string{"snake", "team_", "Team_snake", "team_a/b", "team_x..y"} {
		_, err := TeamCreate(context.Background(), TeamCreateOptions{Repo: r.repo, Name: n, BaseBranch: "origin/main"})
		wantExit(t, err, ExitUsage, "name_invalid")
	}
	if len(*f.calls) != 0 {
		t.Error("a refused name reached Orca")
	}
}

func TestTeamCreate_RefusesLocalBase(t *testing.T) {
	r := newTeamRepo(t)
	r.fake(t, &fakeTeamOrca{})
	for _, b := range []string{"main", "origin/", ""} {
		_, err := TeamCreate(context.Background(), TeamCreateOptions{Repo: r.repo, Name: "team_snake", BaseBranch: b})
		wantExit(t, err, ExitUsage, "base_invalid")
	}
}

func TestTeamCreate_OrcaAbsentNotAttempted(t *testing.T) {
	r := newTeamRepo(t)
	f := r.fake(t, &fakeTeamOrca{})
	lookOrca = func() (string, bool) { return "", false }
	res, err := teamCreate(r)
	if err != nil || res.Team.State != TeamNotAttempted || res.Team.Attempted || res.Team.Reason != ReasonNotFound || f.count("worktree create") != 0 {
		t.Errorf("orca absent: %+v, %v", res.Team, err)
	}
}

func TestTeamCreate_PsFailsNotAttempted(t *testing.T) {
	r := newTeamRepo(t)
	f := r.fake(t, &fakeTeamOrca{psFails: true})
	res, err := teamCreate(r)
	if err != nil || res.Team.State != TeamNotAttempted || res.Team.Reason != "ps_failed" || f.count("worktree create") != 0 {
		t.Errorf("ps failed: %+v, %v", res.Team, err)
	}
}

func TestTeamCreate_AlreadyExistsNoCreate(t *testing.T) {
	r := newTeamRepo(t)
	f := r.fake(t, &fakeTeamOrca{rows: []string{`{"worktreeId":"r::/o/team_snake","path":"/o/team_snake","branch":"refs/heads/team_snake","displayName":"team_snake"}`}})
	res, err := teamCreate(r)
	if err != nil || res.Team.State != TeamAlreadyExists || res.Team.Path != "/o/team_snake" || f.count("worktree create") != 0 {
		t.Errorf("already exists: %+v, %v", res.Team, err)
	}
}

func TestTeamCreate_PsBranchPrefixTrimmed(t *testing.T) {
	r := newTeamRepo(t)
	r.fake(t, &fakeTeamOrca{rows: []string{`{"worktreeId":"r::/o/x","path":"/o/x","branch":"refs/heads/team_snake","displayName":"something else"}`}})
	if res, _ := teamCreate(r); res.Team.State != TeamAlreadyExists || res.Team.Branch != "team_snake" {
		t.Errorf("branch match after the refs/heads/ trim: %+v", res.Team)
	}
}

func TestTeamCreate_UnrecognizedButListedIsCreated(t *testing.T) {
	r := newTeamRepo(t)
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) {
		r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", true)
		return `not json at all`, true
	}})
	if res, err := teamCreate(r); err != nil || res.Team.State != TeamCreated {
		t.Errorf("a create both sides show is created, whatever it printed: %+v, %v", res.Team, err)
	}
}

func TestTeamCreate_AbsentEverywhereFailsOnce(t *testing.T) {
	r := newTeamRepo(t)
	f := r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) { return `{"ok":true}`, false }})
	res, err := teamCreate(r)
	wantExit(t, err, ExitCreateFailed, "create_failed")
	if !res.Team.Attempted || res.Team.State != TeamFailed || f.count("worktree create") != 1 {
		t.Errorf("failed: %+v, creates=%d", res.Team, f.count("worktree create"))
	}
}

func TestTeamCreate_GitOnlyAmbiguous(t *testing.T) {
	r := newTeamRepo(t)
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) {
		r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", false)
		return "", true
	}})
	res, err := teamCreate(r)
	wantExit(t, err, ExitCreateAmbiguous, "create_ambiguous")
	if res.Team.State != TeamAmbiguous || res.Team.State == TeamNotAttempted || f.count("worktree create") != 1 {
		t.Errorf("ambiguous: %+v", res.Team)
	}
}

func TestTeamCreate_BranchMismatch(t *testing.T) {
	r := newTeamRepo(t)
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) {
		p := r.orcaAdd(t, f, "team_snake", "other", "origin/main", false)
		f.rows = append(f.rows, fmt.Sprintf(`{"path":%q,"branch":"refs/heads/other","displayName":"team_snake"}`, p))
		return `{"ok":true}`, false
	}})
	_, err := teamCreate(r)
	wantExit(t, err, ExitTeamWorktreeMismatch, "team_worktree_mismatch")
	if !strings.Contains(err.Error(), "branch \"other\"") {
		t.Errorf("the mismatch must name the actual branch: %v", err)
	}
}

func TestTeamCreate_HeadNotBase(t *testing.T) {
	r := newTeamRepo(t)
	gitT(t, r.repo, "commit", "-q", "--allow-empty", "-m", "local only")
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) {
		r.orcaAdd(t, f, "team_snake", "team_snake", "main", true) // the LOCAL ref, one commit ahead
		return `{"ok":true}`, false
	}})
	_, err := teamCreate(r)
	wantExit(t, err, ExitTeamWorktreeMismatch, "team_worktree_mismatch")
	if _, serr := os.Stat(filepath.Join(r.ws, "team_snake")); serr != nil {
		t.Error("a mismatched worktree stays")
	}
}

func TestTeamCreate_AgentTerminalRefused(t *testing.T) {
	r := newTeamRepo(t)
	p := filepath.Join(r.ws, "team_snake")
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{
		terms: `{"ok":true,"result":{"terminals":[{"handle":"term_a","worktreePath":"` + p + `","title":"claude","tabId":"t","leafId":"l","agentIdentity":"claude"}]}}`,
		create: func([]string) (string, bool) {
			r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", true)
			return `{"ok":true}`, false
		},
	})
	_, err := teamCreate(r)
	wantExit(t, err, ExitAgentTerminalPresent, "agent_terminal_present")
}

func TestTeamCreate_ShellTerminalsListed(t *testing.T) {
	r := newTeamRepo(t)
	p := filepath.Join(r.ws, "team_snake")
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{
		terms: `{"ok":true,"result":{"terminals":[{"handle":"term_s","worktreePath":"` + p + `","title":"Terminal 1","tabId":"t","leafId":"l","agentIdentity":null},{"handle":"term_o","worktreePath":"/elsewhere","title":"other","tabId":"t2","leafId":"l2","agentIdentity":"claude"}]}}`,
		create: func([]string) (string, bool) {
			r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", true)
			return `{"ok":true}`, false
		},
	})
	res, err := teamCreate(r)
	if err != nil || res.Team.State != TeamCreated || len(res.Team.Terminals) != 1 || res.Team.Terminals[0].Title != "Terminal 1" {
		t.Errorf("plain shell passes and is listed; another worktree's agent is ignored: %+v, %v", res.Team, err)
	}
}

func TestTeamCreate_PathComparedBySameDir(t *testing.T) {
	r := newTeamRepo(t)
	link := filepath.Join(realTemp(t), "ws-link")
	if err := os.Symlink(r.ws, link); err != nil {
		t.Fatal(err)
	}
	var f *fakeTeamOrca
	f = r.fake(t, &fakeTeamOrca{create: func([]string) (string, bool) {
		r.orcaAdd(t, f, "team_snake", "team_snake", "origin/main", false)
		lp := filepath.Join(link, "team_snake") // Orca reports the path through a symlink
		f.rows = append(f.rows, fmt.Sprintf(`{"path":%q,"branch":"refs/heads/team_snake","displayName":"team_snake"}`, lp))
		return `{"ok":true}`, false
	}})
	if res, err := teamCreate(r); err != nil || res.Team.State != TeamCreated {
		t.Errorf("the same dir through a symlink is the same worktree: %+v, %v", res.Team, err)
	}
}
