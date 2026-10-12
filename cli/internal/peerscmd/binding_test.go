//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/peername"
)

const (
	sidM = "aaaaaaaa-0000-4000-8000-000000000001"
	sidN = "aaaaaaaa-0000-4000-8000-000000000002"
	sidO = "aaaaaaaa-0000-4000-8000-000000000003"
	sidW = "dddddddd-0000-4000-8000-00000000000d"
)

func withBranch(t *testing.T, branch string) {
	t.Helper()
	old := branchOf
	branchOf = func(context.Context, string) string { return branch }
	t.Cleanup(func() { branchOf = old })
}

func bindBranch(t *testing.T, root, branch, orc string) {
	t.Helper()
	p := filepath.Join(root, ".lets", "sessions", ".task-"+strings.ReplaceAll(branch, "/", "-"))
	if err := os.WriteFile(p, []byte("task: lets-abc\norc: "+orc+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeRepos makes Orca list exactly these main checkouts, indexed in this order.
func fakeRepos(t *testing.T, paths ...string) {
	t.Helper()
	old := listOrcaRepos
	listOrcaRepos = func(context.Context) (*orcacmd.ReposInfo, *orcacmd.Failure) {
		info := &orcacmd.ReposInfo{Repos: []orcacmd.RepoInfo{}}
		for i, p := range paths {
			info.Repos = append(info.Repos, orcacmd.RepoInfo{Index: i, Name: filepath.Base(filepath.Dir(p)), Path: p})
		}
		return info, nil
	}
	t.Cleanup(func() { listOrcaRepos = old })
}

// siblingRepo is a second git main checkout with a .lets dir, as Orca would list it.
func siblingRepo(t *testing.T) string {
	t.Helper()
	repo := gitRepo(t)
	_ = os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755)
	return repo
}

// treeState is every file under dir: mode, mtime and content.
func treeState(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		s := fi.Mode().String() + " " + fi.ModTime().String()
		if !fi.IsDir() {
			b, _ := os.ReadFile(p)
			s += " " + string(b)
		}
		out[p] = s
		return nil
	})
	return out
}

// ---- name resolution (ResolveOrchestrator, `lets peers orchestrator`) ----

// recordOrchestrators writes the merge-branch (main) task-state of root.
func recordOrchestrators(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".lets", "sessions", ".task-main"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// orcEnv runs the verb and returns its result and the emitted JSON envelope.
func orcEnv(t *testing.T, root, session string) (*OrchestratorResult, map[string]any) {
	t.Helper()
	res, err := Orchestrator(context.Background(), root, session)
	if err != nil || !res.OK {
		t.Fatalf("orchestrator: %+v %v", res, err)
	}
	b, _ := json.Marshal(res)
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	return res, env
}

// skillStanding is the orc skill's condition for a standing grant: target.standing
// is the JSON true.
func skillStanding(env map[string]any) bool {
	tg, _ := env["target"].(map[string]any)
	v, present := tg["standing"]
	return present && v == true
}

// selfStanding is self_record.standing as the skill reads it; ok=false when
// self_record is null or absent.
func selfStanding(env map[string]any) (string, bool) {
	sr, present := env["self_record"]
	if !present {
		panic("self_record must always be in the envelope")
	}
	m, ok := sr.(map[string]any)
	if !ok {
		return "", false
	}
	st, _ := m["standing"].(string)
	return st, true
}

func TestResolveByName_Self(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	recordOrchestrators(t, root, "orchestrator: MAIN-LETS scope=docs standing=on\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}})
	res, env := orcEnv(t, root, sidM)
	if res.Source != "self" || res.Target == nil || res.Target.Session != sidM || !skillStanding(env) {
		t.Errorf("self: %+v", res)
	}
	if res.SelfRecord == nil || *res.SelfRecord != (SelfRecord{Name: "MAIN-LETS", Scope: "docs", Standing: "on"}) {
		t.Errorf("self_record: %+v", res.SelfRecord)
	}
}

// bound wins first, even for a session that is itself a recorded orchestrator;
// self_record still says it is one.
func TestResolveByName_BoundFirst(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "OTHER")
	recordOrchestrators(t, root, "orchestrator: MAIN-LETS\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, "OTHER", root}})
	res, env := orcEnv(t, root, sidM)
	if res.Source != "bound" || res.Target == nil || res.Target.Session != sidN {
		t.Errorf("bound first: %+v", res)
	}
	if st, ok := selfStanding(env); !ok || st != "" {
		t.Errorf("self_record keeps the unset standing: %q %v", st, ok)
	}
}

// target.standing is true only for a record's standing=on; self_record.standing
// keeps the string. Checked on the emitted envelope, as the skill reads it.
func TestResolveByName_Standing(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN-PWA")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {3, sidW, "W", root}})
	for line, want := range map[string]struct {
		target bool
		self   string
	}{
		"orchestrator: MAIN-PWA standing=on\n":  {true, "on"},
		"orchestrator: MAIN-PWA standing=off\n": {false, "off"},
		"orchestrator: MAIN-PWA\n":              {false, ""},
	} {
		recordOrchestrators(t, root, line)
		res, env := orcEnv(t, root, sidW)
		tg, _ := env["target"].(map[string]any)
		if _, present := tg["standing"]; res.Source != "bound" || skillStanding(env) != want.target || !present {
			t.Errorf("%q: worker sees target.standing=%v (present=%v), want %v", line, tg["standing"], present, want.target)
		}
		if st, ok := selfStanding(env); ok {
			t.Errorf("%q: the worker has no record of its own: %q", line, st)
		}
		_, env = orcEnv(t, root, sidM)
		if st, ok := selfStanding(env); !ok || st != want.self || skillStanding(env) != want.target {
			t.Errorf("%q: the orchestrator sees self_record.standing=%q (ok=%v), target.standing=%v", line, st, ok, skillStanding(env))
		}
	}
}

// A bound name whose only live session runs in another directory is the target -
// by name, with its cwd, no repo_index, and never with standing (cross-repo stays
// per-message).
func TestResolveByName_BoundElsewhere(t *testing.T) {
	root := repoWithLets(t, "")
	far := gitRepo(t)
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "FAR")
	recordOrchestrators(t, root, "orchestrator: FAR standing=on\n")
	claudeHome(t, []regRow{{1, sidM, "FAR", far}, {3, sidW, "W", root}})
	res, env := orcEnv(t, root, sidW)
	if res.Source != "bound" || res.Target == nil || res.Target.Session != sidM || res.Target.Cwd != far || res.Target.RepoIndex != nil || res.Reason != "" || len(res.Refused) != 0 {
		t.Fatalf("cross-repo bound target: %+v", res)
	}
	if skillStanding(env) {
		t.Errorf("a cross-repo target never carries standing: %+v", env["target"])
	}
}

func TestResolveByName_BoundRefusals(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "GONE")
	claudeHome(t, []regRow{{3, sidW, "W", root}})
	res, _ := orcEnv(t, root, sidW)
	if res.Source != "bound" || res.Target != nil || res.Reason != "target_not_alive" || len(res.Refused) != 1 || res.Refused[0].Name != "GONE" ||
		!strings.Contains(res.Remediation, "GONE") || !strings.Contains(res.Remediation, "/lets:start") {
		t.Errorf("bound dead: %+v", res)
	}
	bindBranch(t, root, "feature/x", "DUP")
	claudeHome(t, []regRow{{1, sidM, "DUP", root}, {2, sidN, "DUP", gitRepo(t)}, {3, sidW, "W", root}})
	res, _ = orcEnv(t, root, sidW)
	if res.Target != nil || res.Reason != "target_unsendable" || res.Refused[0].Detail != "name_not_unique" || !strings.Contains(res.Remediation, "/rename") {
		t.Errorf("bound duplicated: %+v", res)
	}
	bindBranch(t, root, "feature/x", "HIDDEN")
	claudeHome(t, []regRow{{3, sidW, "W", root}}, 77)
	res, _ = orcEnv(t, root, sidW)
	if res.Target != nil || res.Reason != "target_unsendable" || res.Refused[0].Detail != "liveness_unknown" {
		t.Errorf("an unreadable registry entry may carry the name: %+v", res)
	}
}

func TestResolveByName_Unbound(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	conv := peername.Orchestrator(peername.RepoSlug(root), "")
	for _, c := range []struct {
		name    string
		records string
		rows    []regRow
		source  string
		target  string
	}{
		{"single recorded", "orchestrator: MAIN-LETS\n", []regRow{{1, sidM, "MAIN-LETS", root}}, "single", sidM},
		{"two scopes", "orchestrator: MAIN-PWA scope=pwa\norchestrator: MAIN-LIC scope=lic\n", []regRow{{1, sidM, "MAIN-PWA", root}, {2, sidN, "MAIN-LIC", root}}, "ambiguous", ""},
		{"not recorded, no convention", "", []regRow{{1, sidM, "MAIN-LETS", root}}, "none", ""},
		{"recorded not live, convention live", "orchestrator: MAIN-GONE\n", []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, conv, root}}, "single", sidN},
		{"recorded live beats the convention", "orchestrator: MAIN-LETS\n", []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, conv, root}}, "single", sidM},
	} {
		recordOrchestrators(t, root, c.records)
		claudeHome(t, append(c.rows, regRow{3, sidW, "W", root}))
		res, env := orcEnv(t, root, sidW)
		got := ""
		if res.Target != nil {
			got = res.Target.Session
		}
		if res.Source != c.source || got != c.target {
			t.Errorf("%s: source=%s target=%s, want %s %s (%+v)", c.name, res.Source, got, c.source, c.target, res)
		}
		if c.source == "ambiguous" && (len(res.Candidates) != 2 || res.Candidates[0].Scope == "" || res.Candidates[1].Scope == "") {
			t.Errorf("%s: candidates %+v", c.name, res.Candidates)
		}
		if sr, present := env["self_record"]; !present || sr != nil {
			t.Errorf("%s: self_record must be null for a worker: %v", c.name, sr)
		}
	}
}

// orc: on the merge-branch file binds nobody.
func TestResolveByName_MergeBranchOrcIgnored(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "main")
	recordOrchestrators(t, root, "orc: MAIN-PWA\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {3, sidW, "W", root}})
	if res, _ := orcEnv(t, root, sidW); res.Source == "bound" {
		t.Errorf("orc: on the merge-branch is ignored: %+v", res)
	}
}

// A spent budget never falls through to another orchestrator; a branch read before
// the budget still resolves its binding.
func TestResolveByName_Budget(t *testing.T) {
	root := repoWithLets(t, "")
	bindBranch(t, root, "feature/x", "MAIN-PWA")
	recordOrchestrators(t, root, "orchestrator: MAIN-PWA\norchestrator: MAIN-LIC\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {2, sidN, "MAIN-LIC", gitRepo(t)}, {3, sidW, "W", root}})
	v := loadNameView(context.Background(), root)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if r := ResolveOrchestrator(cancelled, v, ResolveOptions{Session: sidW, Cwd: root, Branch: "feature/x"}); r.Source != "bound" || r.Target == nil || r.Target.Session != sidM {
		t.Errorf("a branch read outside the budget keeps its binding: %+v", r)
	}
	if r := ResolveOrchestrator(cancelled, v, ResolveOptions{Session: sidW, Cwd: root, Branch: "feature/y"}); r.Source != "none" || r.Target != nil || r.Reason != "budget_exhausted" || r.Remediation == "" {
		t.Errorf("a spent budget on an unbound branch must not resolve single: %+v", r)
	}
	old := branchOf
	branchOf = func(context.Context, string) string { return "" }
	t.Cleanup(func() { branchOf = old })
	if r := ResolveOrchestrator(cancelled, v, ResolveOptions{Session: sidW, Cwd: root}); r.Reason != "branch_unreadable" || r.Target != nil {
		t.Errorf("an unreadable branch under a spent budget: %+v", r)
	}
}

// One orchestrator row beside a registry entry that could not be read is ambiguous,
// never single: the unreadable entry may be another orchestrator (F8).
func TestResolveByName_DegradedRegistryNeverSingle(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	recordOrchestrators(t, root, "orchestrator: MAIN-LETS\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}, {3, sidW, "W", root}}, 77)
	res, _ := orcEnv(t, root, sidW)
	if res.Source != "ambiguous" || res.Target != nil || len(res.Candidates) != 1 || res.Candidates[0].Session != sidM {
		t.Errorf("degraded registry: %+v", res)
	}
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}, {3, sidW, "W", root}})
	if res, _ := orcEnv(t, root, sidW); res.Source != "single" {
		t.Errorf("a healthy registry resolves single: %+v", res)
	}
}

// Two live processes share one session id (`claude -r` beside the running original)
// under different names and repos: a binding to the foreign one's name returns THAT
// process - its name, its cwd, no standing - never the local row's name or grant.
func TestResolveByName_BoundTargetIsTheSameProcess(t *testing.T) {
	root := repoWithLets(t, "")
	far := gitRepo(t)
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "ORC")
	recordOrchestrators(t, root, "orchestrator: W-LOCAL standing=on\norchestrator: ORC standing=on\n")
	claudeHome(t, []regRow{{1, sidM, "W-LOCAL", root}, {2, sidM, "ORC", far}, {3, sidW, "W", root}})
	res, env := orcEnv(t, root, sidW)
	if res.Source != "bound" || res.Target == nil || res.Target.Name != "ORC" || res.Target.Cwd != far || res.Target.Standing || res.Target.RepoIndex != nil {
		t.Fatalf("bound to the foreign process: %+v", res.Target)
	}
	if skillStanding(env) {
		t.Error("the local row's standing must never be substituted")
	}
}
