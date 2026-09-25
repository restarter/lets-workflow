//go:build unix

package memberscmd_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/cli"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/memberscmd"
)

const (
	sidA = "aaaaaaaa-0000-4000-8000-000000000001"
	sidB = "bbbbbbbb-0000-4000-8000-000000000002"
	sidM = "cccccccc-0000-4000-8000-000000000003"
	sidN = "dddddddd-0000-4000-8000-000000000004"
)

var t0 = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// fakeRegistry is Claude Code's session registry in a temp dir: <dir>/sessions/<pid>.json,
// with ProcAlive answering from the same set.
type fakeRegistry struct {
	t     *testing.T
	dir   string
	alive map[int]bool
}

func newRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	r := &fakeRegistry{t: t, dir: t.TempDir(), alive: map[int]bool{}}
	if err := os.MkdirAll(filepath.Join(r.dir, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldHome, oldAlive, oldNow, oldSleep := ccregistry.HomeDir, ccregistry.ProcAlive, memberscmd.Now, memberscmd.Sleep
	ccregistry.HomeDir = func() string { return r.dir }
	ccregistry.ProcAlive = func(pid int) bool { return r.alive[pid] }
	memberscmd.Now = func() time.Time { return t0 }
	memberscmd.Sleep = func(time.Duration) {}
	t.Cleanup(func() {
		ccregistry.HomeDir, ccregistry.ProcAlive, memberscmd.Now, memberscmd.Sleep = oldHome, oldAlive, oldNow, oldSleep
	})
	return r
}

// put registers a live session: pid runs sid named name at cwd, started an hour before t0.
func (r *fakeRegistry) put(pid int, sid, name, cwd string) {
	r.t.Helper()
	b, _ := json.Marshal(map[string]any{
		"name": name, "nameSource": "user", "sessionId": sid, "cwd": cwd, "status": "idle",
		"startedAt": t0.Add(-time.Hour).UnixMilli(), "peerProtocol": 1,
	})
	if err := os.WriteFile(filepath.Join(r.dir, "sessions", fmt.Sprintf("%d.json", pid)), b, 0o600); err != nil {
		r.t.Fatal(err)
	}
	r.alive[pid] = true
}

// kill ends pid: its file goes and the process is dead.
func (r *fakeRegistry) kill(pid int) {
	_ = os.Remove(filepath.Join(r.dir, "sessions", fmt.Sprintf("%d.json", pid)))
	delete(r.alive, pid)
}

func realDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// opts is a scope in a fresh root, called from session sid.
func opts(t *testing.T, scope, sid string) memberscmd.Options {
	root := realDir(t)
	return memberscmd.Options{Root: root, CallerToplevel: root, Session: sid, Scope: scope}
}

func kindOf(err error) string {
	var e *memberscmd.Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

// claim records o's session as the lead of its team scope: Add in a team scope
// records members only for the lead.
func claim(t *testing.T, o memberscmd.Options) {
	t.Helper()
	if _, err := memberscmd.ClaimLead(o); err != nil {
		t.Fatalf("claim lead: %v", err)
	}
}

func readFile(t *testing.T, o memberscmd.Options) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(o.Root, ".lets", "execution", "members-"+o.Scope+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestResult_SchemaContract(t *testing.T) {
	if memberscmd.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; a bump is a breaking change - update member-run and cli/README.md first", memberscmd.SchemaVersion)
	}
	toMap := func(v any) map[string]any {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	need := func(label string, m map[string]any, keys ...string) {
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("%s: missing key %q", label, k)
			}
		}
	}
	env := []string{"schema_version", "ok", "subcommand", "scope", "steps"}
	member := &memberscmd.Member{}
	add := toMap(&memberscmd.AddResult{Member: member})
	need("add", add, append(env, "member")...)
	need("add.member", add["member"].(map[string]any), "name", "agent_name", "role", "model", "isolation", "worktree_path",
		"worktree_branch", "kind", "session", "pid", "set", "caller_toplevel", "lead_pid", "link", "status")
	need("dismiss", toMap(&memberscmd.DismissResult{Dismissed: []string{}}), append(env, "dismissed")...)
	status := toMap(&memberscmd.StatusResult{Members: []memberscmd.Member{}})
	need("status", status, append(env, "lead", "members")...)
	lead := toMap(&memberscmd.LeadResult{Lead: &memberscmd.LeadStatus{}})
	need("lead", lead, append(env, "lead", "claimed")...)
	need("lead.lead", lead["lead"].(map[string]any), "session", "name", "pid", "set", "status")
}

func TestShippedRoles_MatchAgents(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "plugins", "lets", "agents", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no agents found: %v", err)
	}
	var want []string
	for _, f := range files {
		if name := strings.TrimSuffix(filepath.Base(f), ".md"); name != "actor" {
			want = append(want, name)
		}
	}
	got := slices.Clone(memberscmd.ShippedRoles)
	sort.Strings(want)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("ShippedRoles = %v, agents/*.md minus actor = %v", got, want)
	}
}

// The caller's pid comes from its registry entry, never from os.Getppid.
func TestAdd_UsesRegistryPidNotPpid(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "run-abc123", sidA)
	r.put(4242, sidA, "lead", o.Root)
	res, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "impl-abc123-c1", Role: "lets:implementer"})
	if err != nil || !res.OK {
		t.Fatalf("add: %v", err)
	}
	m := res.Member
	if m.Pid != 4242 || m.LeadPid != 4242 || m.Pid == os.Getppid() || m.Session != sidA || m.Kind != memberscmd.KindInProcess {
		t.Errorf("member = %+v, want in-process with registry pid 4242", m)
	}
	if m.Role != "lets:implementer" || m.CallerToplevel != o.Root || m.Status != memberscmd.StatusLive {
		t.Errorf("member = %+v", m)
	}

	// No session id, or a session the registry does not show -> registry_unavailable.
	for _, sid := range []string{"", sidB} {
		o2 := o
		o2.Session = sid
		if _, err := memberscmd.Add(o2, memberscmd.AddOptions{Name: "x", Role: "qa"}); kindOf(err) != "registry_unavailable" {
			t.Errorf("session %q: err=%v, want registry_unavailable", sid, err)
		}
	}
}

func TestAdd_RefusesLiveName(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	claim(t, o)
	a := memberscmd.AddOptions{Name: "architect", Role: "architect"}
	if _, err := memberscmd.Add(o, a); err != nil {
		t.Fatal(err)
	}
	if _, err := memberscmd.Add(o, a); kindOf(err) != "name_live" {
		t.Fatalf("second add: err=%v, want name_live", err)
	}
	// dismissed (gone) -> the name is free again and the entry is replaced
	if _, err := memberscmd.Dismiss(o, "architect", false); err != nil {
		t.Fatal(err)
	}
	if _, err := memberscmd.Add(o, a); err != nil {
		t.Fatalf("add after dismiss: %v", err)
	}
	st, err := memberscmd.Status(o, "")
	if err != nil || len(st.Members) != 1 || st.Members[0].Status != memberscmd.StatusLive {
		t.Errorf("status after re-add: err=%v members=%+v", err, st.Members)
	}
	for _, bad := range []string{"", "Arch", "a_b", strings.Repeat("a", 41)} {
		if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: bad, Role: "qa"}); kindOf(err) != "name_invalid" {
			t.Errorf("name %q: err=%v, want name_invalid", bad, err)
		}
	}
}

// In a team scope only the recorded lead (or its same-process rotation) records a
// member; an execute scope has no lead gate.
func TestAdd_TeamScopeLeadOnly(t *testing.T) {
	r := newRegistry(t)
	oa := opts(t, "snake", sidA)
	ob := oa
	ob.Session = sidB
	r.put(100, sidA, "snake-lead", oa.Root)
	r.put(200, sidB, "snake-architect", oa.Root) // a pane member in the team cwd
	a := memberscmd.AddOptions{Name: "skeptic", Role: "skeptic"}
	if _, err := memberscmd.Add(oa, a); kindOf(err) != "no_lead" {
		t.Errorf("no recorded lead: err=%v, want no_lead", err)
	}
	claim(t, oa)
	res, err := memberscmd.Add(ob, a)
	if kindOf(err) != "lead_held" || res.OK {
		t.Fatalf("a non-lead caller: err=%v, want lead_held", err)
	}
	if st, _ := memberscmd.Status(oa, ""); len(st.Members) != 0 {
		t.Errorf("a refused add records nothing: %+v", st.Members)
	}
	res, err = memberscmd.Add(oa, a)
	if err != nil || res.Member.LeadPid != 100 {
		t.Fatalf("the lead: err=%v member=%+v", err, res.Member)
	}
	// /clear re-mints the lead's id in the same process: the new id is the lead
	oc := oa
	oc.Session = sidN
	r.put(100, sidN, "snake-lead", oa.Root)
	if res, err := memberscmd.Add(oc, memberscmd.AddOptions{Name: "explorer", Role: "explorer"}); err != nil || res.Member.LeadPid != 100 {
		t.Fatalf("the rotated lead: err=%v", err)
	}
	if got := readFile(t, oa)["lead"].(map[string]any)["session"]; got != sidN {
		t.Errorf("stored lead session = %v, want the rotation carried to %s", got, sidN)
	}
	if _, err := memberscmd.Add(ob, memberscmd.AddOptions{Name: "qa", Role: "qa"}); kindOf(err) != "lead_held" {
		t.Errorf("a non-lead after the rotation: err=%v, want lead_held", err)
	}
	// an execute scope: no lead recorded, any caller records its implementer
	or := opts(t, "run-abc123", sidB)
	r.put(300, sidB, "other", or.Root)
	if _, err := memberscmd.Add(or, memberscmd.AddOptions{Name: "impl-1", Role: "implementer"}); err != nil {
		t.Errorf("execute scope: %v", err)
	}
}

func TestAdd_RefusesActor(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	for _, role := range []string{"actor", "lets:actor", "general-purpose", "lets:general-purpose", ""} {
		if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "x", Role: role}); kindOf(err) != "role_not_allowed" {
			t.Errorf("role %q: err=%v, want role_not_allowed", role, err)
		}
	}
	if _, err := os.Stat(filepath.Join(o.Root, ".lets")); !os.IsNotExist(err) {
		t.Error("a refused add must write nothing")
	}
	if _, err := memberscmd.Add(opts(t, "Bad_Scope", sidA), memberscmd.AddOptions{Name: "x", Role: "qa"}); kindOf(err) != "invalid_scope" {
		t.Errorf("bad scope: err=%v", err)
	}
}

// A pane teammate is its own registry session: `<callsign>-<name>` at the caller's
// toplevel is recorded with its OWN sid and pid, and no retry is spent.
func TestAdd_PaneMemberOwnSid(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	r.put(200, sidM, "snake-architect", o.Root)
	claim(t, o)
	sleeps := 0
	memberscmd.Sleep = func(time.Duration) { sleeps++ }
	res, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "architect", Role: "architect"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Member
	if m.Kind != memberscmd.KindPane || m.Session != sidM || m.Pid != 200 || m.LeadPid != 100 || m.Link != memberscmd.LinkTeam || m.AgentName != "snake-architect" {
		t.Errorf("member = %+v, want pane with its own session", m)
	}
	if sleeps != 0 {
		t.Errorf("sleeps = %d, want 0", sleeps)
	}
}

// No session under the Agent name at the cwd -> in-process, liveness is the lead's.
func TestAdd_InProcessFallsBackToLead(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	r.put(200, sidM, "snake-architect", "/somewhere/else") // right name, wrong cwd
	r.put(300, sidN, "architect", o.Root)                  // right cwd, bare name
	claim(t, o)
	res, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "architect", Role: "architect"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Member; m.Kind != memberscmd.KindInProcess || m.Session != sidA || m.Pid != 100 {
		t.Errorf("member = %+v, want in-process on the lead's session", m)
	}
	// --link peer needs the member's own session.
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "skeptic", Role: "skeptic", Link: memberscmd.LinkPeer}); kindOf(err) != "registry_unavailable" {
		t.Errorf("peer without a session: err=%v", err)
	}
}

// The lookup retries 5 x 1 s: a pane that registers late is still found, one that
// never registers costs exactly 4 sleeps.
func TestAdd_RetryBounded(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	claim(t, o)
	var slept []time.Duration
	memberscmd.Sleep = func(d time.Duration) { slept = append(slept, d) }
	res, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "explorer", Role: "explorer"})
	if err != nil || res.Member.Kind != memberscmd.KindInProcess {
		t.Fatalf("never registers: err=%v member=%+v", err, res.Member)
	}
	if len(slept) != 4 || slept[0] != time.Second {
		t.Errorf("slept %v, want 4 x 1s", slept)
	}

	slept = nil
	memberscmd.Sleep = func(d time.Duration) {
		slept = append(slept, d)
		if len(slept) == 2 {
			r.put(200, sidM, "snake-qa", o.Root)
		}
	}
	res, err = memberscmd.Add(o, memberscmd.AddOptions{Name: "qa", Role: "qa"})
	if err != nil || res.Member.Kind != memberscmd.KindPane || res.Member.Session != sidM {
		t.Fatalf("late pane: err=%v member=%+v", err, res.Member)
	}
	if len(slept) != 2 {
		t.Errorf("slept %d times, want 2", len(slept))
	}
}

// Execute scope: the Agent name is the bare name, looked up in the isolated
// worktree the member runs in.
func TestAdd_ExecuteScopePaneOwnSid(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "run-abc123", sidA)
	wt := realDir(t)
	r.put(100, sidA, "lead", o.Root)
	r.put(200, sidM, "impl-abc123-c1", wt)
	res, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "impl-abc123-c1", Role: "implementer", Isolation: "worktree", WorktreePath: wt, WorktreeBranch: "worktree-agent-x"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Member; m.Kind != memberscmd.KindPane || m.Session != sidM || m.Pid != 200 || m.AgentName != "impl-abc123-c1" {
		t.Errorf("member = %+v", m)
	}
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "impl-2", Role: "implementer", Isolation: "worktree"}); kindOf(err) != "usage" {
		t.Errorf("isolation without a path: err=%v", err)
	}
}

func TestStatus_Live(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "run-abc123", sidA)
	r.put(100, sidA, "lead", o.Root)
	// No registry file: empty, and nothing is written.
	st, err := memberscmd.Status(o, "")
	if err != nil || !st.OK || st.Lead != nil || len(st.Members) != 0 {
		t.Fatalf("empty: err=%v res=%+v", err, st)
	}
	if _, err := os.Stat(filepath.Join(o.Root, ".lets")); !os.IsNotExist(err) {
		t.Error("status of an empty scope must write nothing")
	}
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "impl-1", Role: "implementer"}); err != nil {
		t.Fatal(err)
	}
	st, err = memberscmd.Status(o, "impl-1")
	if err != nil || len(st.Members) != 1 || st.Members[0].Status != memberscmd.StatusLive {
		t.Fatalf("err=%v members=%+v", err, st.Members)
	}
	// The lead's session ends: its in-process member is gone.
	r.kill(100)
	st, _ = memberscmd.Status(o, "")
	if m := st.Members[0]; m.Status != memberscmd.StatusGone || m.Reason != memberscmd.ReasonSessionDead {
		t.Errorf("after the lead exits: %+v", m)
	}
}

// /clear re-mints the id in the same process: reported rotated once, carried to the
// new id under the lock, live afterwards.
func TestStatus_RotatedCarried(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "run-abc123", sidA)
	r.put(100, sidA, "lead", o.Root)
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "impl-1", Role: "implementer"}); err != nil {
		t.Fatal(err)
	}
	r.put(100, sidB, "lead", o.Root) // same pid, new id, process started before set
	st, err := memberscmd.Status(o, "")
	if err != nil || st.Members[0].Status != memberscmd.StatusRotated || st.Members[0].Session != sidB {
		t.Fatalf("err=%v member=%+v, want rotated to %s", err, st.Members[0], sidB)
	}
	if got := readFile(t, o)["members"].([]any)[0].(map[string]any)["session"]; got != sidB {
		t.Errorf("stored session = %v, want carried to %s", got, sidB)
	}
	st, _ = memberscmd.Status(o, "")
	if st.Members[0].Status != memberscmd.StatusLive {
		t.Errorf("after the carry: %+v", st.Members[0])
	}
}

// `claude -r` runs the same id under a new pid: the member's process (and any
// in-process teammate with it) is gone.
func TestStatus_ResumedNewPidGone(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	r.put(200, sidM, "snake-architect", o.Root)
	claim(t, o)
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "architect", Role: "architect"}); err != nil {
		t.Fatal(err)
	}
	r.kill(200)
	r.put(201, sidM, "snake-architect", o.Root)
	st, _ := memberscmd.Status(o, "")
	if m := st.Members[0]; m.Status != memberscmd.StatusGone || m.Reason != memberscmd.ReasonSessionDead {
		t.Errorf("resumed under a new pid: %+v", m)
	}
}

// A pane member spawned by a lead whose pid then changes is reachable only across
// sessions: link team -> peer, persisted. Without a lead record, the spawning pid
// dying flips it the same way.
func TestStatus_LinkFlipsOnLeadPid(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	r.put(200, sidM, "snake-architect", o.Root)
	if _, err := memberscmd.ClaimLead(o); err != nil {
		t.Fatal(err)
	}
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "architect", Role: "architect"}); err != nil {
		t.Fatal(err)
	}
	st, _ := memberscmd.Status(o, "")
	if st.Members[0].Link != memberscmd.LinkTeam {
		t.Fatalf("before: %+v", st.Members[0])
	}
	r.kill(100)
	r.put(150, sidA, "snake-lead", o.Root) // the lead resumed with claude -r
	if err := memberscmd.RefreshLead(o.Root, o.Scope, sidA); err != nil {
		t.Fatal(err)
	}
	st, _ = memberscmd.Status(o, "")
	if m := st.Members[0]; m.Link != memberscmd.LinkPeer || m.Status != memberscmd.StatusLive {
		t.Errorf("after the lead's pid changed: %+v, want live, link peer", m)
	}
	if got := readFile(t, o)["members"].([]any)[0].(map[string]any)["link"]; got != memberscmd.LinkPeer {
		t.Errorf("stored link = %v", got)
	}

	// Execute scope, no lead record: the spawning process dies.
	o2 := opts(t, "run-abc123", sidB)
	r.put(300, sidB, "lead", o2.Root)
	r.put(301, sidN, "impl-1", o2.Root)
	if _, err := memberscmd.Add(o2, memberscmd.AddOptions{Name: "impl-1", Role: "implementer"}); err != nil {
		t.Fatal(err)
	}
	r.kill(300)
	st, _ = memberscmd.Status(o2, "")
	if m := st.Members[0]; m.Link != memberscmd.LinkPeer {
		t.Errorf("no lead record, spawner dead: %+v", m)
	}
}

func TestLead_Held(t *testing.T) {
	r := newRegistry(t)
	oa := opts(t, "snake", sidA)
	ob := oa
	ob.Session = sidB
	r.put(100, sidA, "snake-lead", oa.Root)
	r.put(200, sidB, "other", oa.Root)
	if _, err := memberscmd.ShowLead(oa); kindOf(err) != "no_lead" {
		t.Errorf("no lead yet: err=%v", err)
	}
	res, err := memberscmd.ClaimLead(oa)
	if err != nil || !res.Claimed || res.Lead.Pid != 100 || res.Lead.Name != "snake-lead" {
		t.Fatalf("claim: err=%v res=%+v", err, res)
	}
	if _, err := memberscmd.ClaimLead(ob); kindOf(err) != "lead_held" {
		t.Errorf("claim while the lead lives: err=%v", err)
	}
	// The lead resumed under a new pid is still the lead.
	r.kill(100)
	r.put(101, sidA, "snake-lead", oa.Root)
	if _, err := memberscmd.ClaimLead(ob); kindOf(err) != "lead_held" {
		t.Errorf("claim while the lead runs under a new pid: err=%v", err)
	}
	// Re-claim by the lead itself is allowed.
	if res, err := memberscmd.ClaimLead(oa); err != nil || res.Lead.Pid != 101 {
		t.Errorf("own re-claim: err=%v res=%+v", err, res)
	}
	show, err := memberscmd.ShowLead(ob)
	if err != nil || show.Lead.Session != sidA || show.Lead.Status != memberscmd.StatusLive {
		t.Errorf("show: err=%v res=%+v", err, show)
	}
}

func TestLead_TakeOverDead(t *testing.T) {
	r := newRegistry(t)
	oa := opts(t, "snake", sidA)
	ob := oa
	ob.Session = sidB
	r.put(100, sidA, "snake-lead", oa.Root)
	if _, err := memberscmd.ClaimLead(oa); err != nil {
		t.Fatal(err)
	}
	r.kill(100)
	r.put(200, sidB, "snake-lead", oa.Root)
	res, err := memberscmd.ClaimLead(ob)
	if err != nil || !res.Claimed || res.Lead.Session != sidB || res.Lead.Pid != 200 {
		t.Fatalf("takeover: err=%v res=%+v", err, res)
	}

	// /clear in the lead's own process re-mints its id: the new id claims as the lead.
	r.put(200, sidN, "snake-lead", oa.Root)
	on := oa
	on.Session = sidN
	if res, err := memberscmd.ClaimLead(on); err != nil || res.Lead.Session != sidN {
		t.Errorf("rotated into the caller: err=%v res=%+v", err, res)
	}
}

func TestRefreshLead_OnlyOwnSid(t *testing.T) {
	r := newRegistry(t)
	o := opts(t, "snake", sidA)
	r.put(100, sidA, "snake-lead", o.Root)
	r.put(200, sidB, "snake-architect", o.Root)
	if err := memberscmd.RefreshLead(o.Root, o.Scope, sidA); kindOf(err) != "no_lead" {
		t.Errorf("no lead: err=%v", err)
	}
	if _, err := memberscmd.ClaimLead(o); err != nil {
		t.Fatal(err)
	}
	r.kill(100)
	r.put(150, sidA, "snake-lead", o.Root)
	before, _ := os.ReadFile(filepath.Join(o.Root, ".lets", "execution", "members-snake.json"))
	if err := memberscmd.RefreshLead(o.Root, o.Scope, sidB); kindOf(err) != "lead_held" {
		t.Errorf("foreign sid: err=%v, want lead_held", err)
	}
	if after, _ := os.ReadFile(filepath.Join(o.Root, ".lets", "execution", "members-snake.json")); string(after) != string(before) {
		t.Error("a foreign sid changed the registry")
	}
	if err := memberscmd.RefreshLead(o.Root, o.Scope, sidA); err != nil {
		t.Fatal(err)
	}
	if l, err := memberscmd.ReadLead(o.Root, o.Scope); err != nil || l.Pid != 150 || l.Session != sidA {
		t.Errorf("refreshed lead = %+v err=%v", l, err)
	}

	// A busy lock (another members call holds it) is best-effort on the hook path:
	// no error, the record unchanged, and the wait bounded at about 1 s.
	r.kill(150)
	r.put(160, sidA, "snake-lead", o.Root)
	f, err := os.OpenFile(filepath.Join(o.Root, ".lets", "locks", "members-snake.lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := fsutil.LockFile(f); err != nil {
		t.Fatal(err)
	}
	before, _ = os.ReadFile(filepath.Join(o.Root, ".lets", "execution", "members-snake.json"))
	start := time.Now()
	if err := memberscmd.RefreshLead(o.Root, o.Scope, sidA); err != nil {
		t.Errorf("lock busy: err=%v, want nil", err)
	}
	if d := time.Since(start); d < 900*time.Millisecond || d > 3*time.Second {
		t.Errorf("lock wait = %v, want about 1s", d)
	}
	if after, _ := os.ReadFile(filepath.Join(o.Root, ".lets", "execution", "members-snake.json")); string(after) != string(before) {
		t.Error("a busy lock changed the registry")
	}
	_ = fsutil.UnlockFile(f)
}

// Isolated members are gone when their worktree is: not listed, prunable, or the
// directory missing.
func TestStatus_IsolatedWorktreeGone(t *testing.T) {
	r := newRegistry(t)
	root := realDir(t)
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("-c", "init.defaultBranch=main", "init", "-q")
	git("-c", "user.email=t@e", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
	o := memberscmd.Options{Root: root, CallerToplevel: root, Session: sidA, Scope: "run-abc123"}
	r.put(100, sidA, "lead", root)
	base := realDir(t)
	for _, n := range []string{"keep", "pruned", "listed-missing"} {
		wt := filepath.Join(base, n)
		git("worktree", "add", "-q", "-b", n, wt)
		if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: n, Role: "implementer", Isolation: "worktree", WorktreePath: wt}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := memberscmd.Add(o, memberscmd.AddOptions{Name: "never", Role: "implementer", Isolation: "worktree", WorktreePath: filepath.Join(base, "never")}); err != nil {
		t.Fatal(err)
	}
	// pruned: the directory goes and git marks it prunable.
	if err := os.RemoveAll(filepath.Join(base, "pruned")); err != nil {
		t.Fatal(err)
	}
	st, err := memberscmd.Status(o, "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"keep": "", "pruned": memberscmd.ReasonPrunable, "never": memberscmd.ReasonWorktreeMissing}
	for _, m := range st.Members {
		if w, ok := want[m.Name]; ok && m.Reason != w {
			t.Errorf("%s: status=%s reason=%s, want reason %q", m.Name, m.Status, m.Reason, w)
		}
	}
	// dir_missing: a LOCKED worktree (the harness locks an isolated agent's while it
	// runs) is never reported prunable, so a vanished directory stays listed.
	lm := filepath.Join(base, "listed-missing")
	git("worktree", "lock", lm)
	if err := os.RemoveAll(lm); err != nil {
		t.Fatal(err)
	}
	st, _ = memberscmd.Status(o, "listed-missing")
	if m := st.Members[0]; m.Status != memberscmd.StatusGone || m.Reason != memberscmd.ReasonDirMissing {
		t.Errorf("listed-missing: %+v", m)
	}
}

// The flags member-run passes are the flags the binary defines: this pins the
// review's flag list per subcommand, and every `lets members` flag the member-run
// skill uses.
func TestMembersFlags_MatchMemberRun(t *testing.T) {
	want := map[string][]string{
		"add":     {"scope", "json", "name", "role", "model", "isolation", "worktree-path", "worktree-branch", "link", "cwd"},
		"dismiss": {"scope", "json", "name", "all"},
		"status":  {"scope", "json", "name"},
		"lead":    {"scope", "json", "claim"},
	}
	var members *cobra.Command
	for _, c := range cli.NewRootCmd().Commands() {
		if c.Name() == "members" {
			members = c
		}
	}
	if members == nil {
		t.Fatal("lets members is not registered")
	}
	defined := map[string]map[string]bool{}
	for _, c := range members.Commands() {
		defined[c.Name()] = map[string]bool{}
		var got []string
		c.Flags().VisitAll(func(f *pflag.Flag) { got = append(got, f.Name); defined[c.Name()][f.Name] = true })
		sort.Strings(got)
		w := slices.Clone(want[c.Name()])
		sort.Strings(w)
		if !slices.Equal(got, w) {
			t.Errorf("members %s flags = %v, want %v", c.Name(), got, w)
		}
	}
	if len(defined) != len(want) {
		t.Errorf("subcommands = %v, want %v", defined, want)
	}
	skill := filepath.Join("..", "..", "..", "plugins", "lets", "skills", "member-run", "SKILL.md")
	data, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("member-run skill: %v - the flag pin must follow a rename, never lapse", err)
	}
	checked := 0
	for _, line := range strings.Split(string(data), "\n") {
		for rest := line; ; {
			i := strings.Index(rest, "lets members ")
			if i < 0 {
				break
			}
			rest = rest[i+len("lets members "):]
			fields := strings.Fields(rest)
			if len(fields) == 0 || defined[fields[0]] == nil {
				continue
			}
			for _, f := range fields[1:] {
				if strings.ContainsAny(f, "`|;") {
					break
				}
				if name, ok := strings.CutPrefix(f, "--"); ok {
					name, _, _ = strings.Cut(name, "=")
					checked++
					if !defined[fields[0]][name] {
						t.Errorf("member-run uses `lets members %s --%s`, which the binary does not define", fields[0], name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Error("no `lets members` flag found in member-run - scanner or skill broken")
	}
}
