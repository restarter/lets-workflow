//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"testing"
)

// sock is the envelope from attribute of the Claude Code process pid.
func sock(pid int) string { return "uds:/tmp/cc-socks/" + itoa(pid) + ".sock" }

const (
	pidOrc = 101 // MAIN-LETS, the orchestrator, in the main checkout
	pidW   = 103 // W, a worker on feature/x bound to MAIN-LETS
	pidU   = 104 // U, a worker on feature/y, unbound
)

// standingRepo is a repo with an orchestrator MAIN-LETS (main checkout), a worker W on
// feature/x bound to it and a worker U on feature/y bound to nobody; records is the
// merge-branch task-state.
func standingRepo(t *testing.T, records string) (root, wtX, wtY string) {
	t.Helper()
	root = repoWithLets(t, "")
	wtX = linkedWorktree(t, root, "feature/x")
	wtY = linkedWorktree(t, root, "feature/y")
	taskState(t, root, "feature/x", "task: lets-abc\norc: MAIN-LETS\n")
	taskState(t, root, "feature/y", "task: lets-def\n")
	recordOrchestrators(t, root, records)
	return root, wtX, wtY
}

// standingEnv is the emitted JSON envelope of res.
func standingEnv(t *testing.T, res any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(res)
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	return env
}

// checkStanding asserts the envelope's answer: standing_ok present as a JSON bool,
// standing_reason set exactly when it is false.
func checkStanding(t *testing.T, label string, env map[string]any, wantOK bool, wantReason string) {
	t.Helper()
	ok, present := env["standing_ok"].(bool)
	reason, _ := env["standing_reason"].(string)
	if !present || ok != wantOK || reason != wantReason {
		t.Errorf("%s: standing_ok=%v (present=%v) standing_reason=%q, want %v %q", label, env["standing_ok"], present, reason, wantOK, wantReason)
	}
}

// The orchestrator side: `who --orc <own> --from-name --from-addr` answers whether a
// message may be answered under the grant. Every reason, and the happy path.
func TestStanding_OrchestratorSide(t *testing.T) {
	for _, c := range []struct {
		name     string
		records  string
		extra    func(root, far string) []regRow // added to the base registry
		unknown  []int
		own      string
		caller   string // default: the orchestrator MAIN-LETS
		fromName string
		fromAddr string
		ok       bool
		reason   string
	}{
		{name: "bound worker, its own process", fromName: "W", fromAddr: sock(pidW), ok: true},
		{name: "addr not a socket", fromName: "W", fromAddr: "tcp:127.0.0.1:3", reason: "from_addr_unparseable"},
		{name: "addr without a pid", fromName: "W", fromAddr: "uds:/tmp/cc-socks/abc.sock", reason: "from_addr_unparseable"},
		{name: "addr pid zero", fromName: "W", fromAddr: "uds:/tmp/cc-socks/0.sock", reason: "from_addr_unparseable"},
		{name: "addr wrong suffix", fromName: "W", fromAddr: "uds:/tmp/cc-socks/103.sockx", reason: "from_addr_unparseable"},
		{name: "degraded registry", unknown: []int{77}, fromName: "W", fromAddr: sock(pidW), reason: "liveness_unknown"},
		{name: "sender gone", fromName: "GHOST", fromAddr: sock(pidW), reason: "sender_not_live"},
		{name: "sender name twice", extra: func(_, far string) []regRow { return []regRow{{105, sidO, "W", far}} }, fromName: "W", fromAddr: sock(pidW), reason: "name_not_unique"},
		{name: "another process claims the name", fromName: "W", fromAddr: sock(pidU), reason: "pid_mismatch"},
		{name: "grant off", records: "orchestrator: MAIN-LETS standing=off\n", fromName: "W", fromAddr: sock(pidW), reason: "standing_off"},
		{name: "never asked", records: "orchestrator: MAIN-LETS\n", fromName: "W", fromAddr: sock(pidW), reason: "standing_off"},
		{name: "no record", records: "orchestrator: OTHER standing=on\n", fromName: "W", fromAddr: sock(pidW), reason: "no_self_record"},
		{name: "caller not in the registry", caller: "aaaaaaaa-0000-4000-8000-0000000000ff", fromName: "W", fromAddr: sock(pidW), reason: "no_self_record"},
		{name: "wrong own name, grant off", records: "orchestrator: MAIN-LETS standing=off\n", own: "MAIN-ETH", fromName: "W", fromAddr: sock(pidW), reason: "not_own_name"},
		{name: "own name reused", extra: func(_, far string) []regRow { return []regRow{{106, sidO, "MAIN-LETS", far}} }, fromName: "W", fromAddr: sock(pidW), reason: "self_name_not_unique"},
		{name: "asks for another name", own: "MAIN-ETH", fromName: "W", fromAddr: sock(pidW), reason: "not_own_name"},
		{name: "unbound worker", fromName: "U", fromAddr: sock(pidU), reason: "not_bound_here"},
	} {
		t.Run(c.name, func(t *testing.T) {
			records := c.records
			if records == "" {
				records = "orchestrator: MAIN-LETS standing=on\n"
			}
			root, wtX, wtY := standingRepo(t, records)
			rows := []regRow{{pidOrc, sidM, "MAIN-LETS", root}, {pidW, sidW, "W", wtX}, {pidU, sidN, "U", wtY}}
			if c.extra != nil {
				rows = append(rows, c.extra(root, gitRepo(t))...)
			}
			claudeHome(t, rows, c.unknown...)
			own := c.own
			if own == "" {
				own = "MAIN-LETS"
			}
			caller := c.caller
			if caller == "" {
				caller = sidM
			}
			res := who(t, WhoOptions{Cwd: root, Orc: own, Caller: caller, Probe: StandingProbe{FromName: c.fromName, FromAddr: c.fromAddr}})
			checkStanding(t, c.name, standingEnv(t, res), c.ok, c.reason)
		})
	}
}

// A sender that is the bound worker's name but runs in another repo is no row here.
func TestStanding_OrchestratorSideCrossRepoSender(t *testing.T) {
	root, _, _ := standingRepo(t, "orchestrator: MAIN-LETS standing=on\n")
	far := gitRepo(t)
	claudeHome(t, []regRow{{pidOrc, sidM, "MAIN-LETS", root}, {pidW, sidW, "W", far}})
	res := who(t, WhoOptions{Cwd: root, Orc: "MAIN-LETS", Session: sidM, Probe: StandingProbe{FromName: "W", FromAddr: sock(pidW)}})
	checkStanding(t, "cross-repo sender", standingEnv(t, res), false, "not_bound_here")
	if res.Self == nil || res.Self.Session != sidM {
		t.Errorf("--session still names self: %+v", res.Self)
	}
}

// The worker / lead side: `orchestrator --from-name --from-addr` answers whether its
// bound orchestrator sent the message under the grant. Every reason, and the happy path.
func TestStanding_WorkerSide(t *testing.T) {
	for _, c := range []struct {
		name     string
		records  string
		orcCwd   func(root, far string) string // where MAIN-LETS runs (default: root)
		extra    func(root, far string) []regRow
		unknown  []int
		unbound  bool // ask from U's checkout (feature/y, no orc:)
		fromName string
		fromAddr string
		ok       bool
		reason   string
	}{
		{name: "bound orchestrator, its own process", fromName: "MAIN-LETS", fromAddr: sock(pidOrc), ok: true},
		{name: "addr unparseable", fromName: "MAIN-LETS", fromAddr: "MAIN-LETS", reason: "from_addr_unparseable"},
		{name: "degraded registry", unknown: []int{77}, fromName: "MAIN-LETS", fromAddr: sock(pidOrc), reason: "liveness_unknown"},
		{name: "sender gone", fromName: "GHOST", fromAddr: sock(pidOrc), reason: "sender_not_live"},
		{name: "orchestrator name twice", extra: func(_, far string) []regRow { return []regRow{{106, sidO, "MAIN-LETS", far}} }, fromName: "MAIN-LETS", fromAddr: sock(pidOrc), reason: "name_not_unique"},
		{name: "another process claims the name", fromName: "MAIN-LETS", fromAddr: sock(pidU), reason: "pid_mismatch"},
		{name: "grant off", records: "orchestrator: MAIN-LETS standing=off\n", fromName: "MAIN-LETS", fromAddr: sock(pidOrc), reason: "standing_off"},
		{name: "never asked", records: "orchestrator: MAIN-LETS\n", fromName: "MAIN-LETS", fromAddr: sock(pidOrc), reason: "standing_off"},
		{name: "not bound", unbound: true, fromName: "MAIN-LETS", fromAddr: sock(pidOrc), reason: "not_bound"},
		{name: "not the bound orchestrator", fromName: "U", fromAddr: sock(pidU), reason: "not_bound_target"},
		{name: "bound orchestrator in another repo", orcCwd: func(_, far string) string { return far }, fromName: "MAIN-LETS", fromAddr: sock(pidOrc), reason: "cross_repo"},
	} {
		t.Run(c.name, func(t *testing.T) {
			records := c.records
			if records == "" {
				records = "orchestrator: MAIN-LETS standing=on\n"
			}
			root, wtX, wtY := standingRepo(t, records)
			far := gitRepo(t)
			orcCwd := root
			if c.orcCwd != nil {
				orcCwd = c.orcCwd(root, far)
			}
			rows := []regRow{{pidOrc, sidM, "MAIN-LETS", orcCwd}, {pidW, sidW, "W", wtX}, {pidU, sidN, "U", wtY}}
			if c.extra != nil {
				rows = append(rows, c.extra(root, far)...)
			}
			claudeHome(t, rows, c.unknown...)
			cwd, session := wtX, sidW
			if c.unbound {
				cwd, session = wtY, sidN
			}
			res, err := OrchestratorWith(context.Background(), cwd, session, StandingProbe{FromName: c.fromName, FromAddr: c.fromAddr})
			if err != nil || !res.OK {
				t.Fatalf("orchestrator: %+v %v", res, err)
			}
			checkStanding(t, c.name, standingEnv(t, res), c.ok, c.reason)
		})
	}
}

// Every reason the decision functions return is exercised above (keep the tables and
// standingReasons in step).
func TestStanding_ReasonsCovered(t *testing.T) {
	want := map[string]bool{
		"from_addr_unparseable": true, "liveness_unknown": true, "sender_not_live": true, "name_not_unique": true, "pid_mismatch": true,
		"no_self_record": true, "standing_off": true, "self_name_not_unique": true, "not_own_name": true, "not_bound_here": true,
		"not_bound": true, "not_bound_target": true, "cross_repo": true,
	}
	for _, r := range standingReasons {
		if !want[r] {
			t.Errorf("standingReasons names %q, which no test covers", r)
		}
		delete(want, r)
	}
	for r := range want {
		t.Errorf("a tested reason %q is missing from standingReasons", r)
	}
}

// Without the probe the envelopes carry no standing keys; one half alone, or the
// probe on `who` without --orc (or with --orca-repos), is a usage error.
func TestStanding_FlagsAndUsage(t *testing.T) {
	root, wtX, _ := standingRepo(t, "orchestrator: MAIN-LETS standing=on\n")
	claudeHome(t, []regRow{{pidOrc, sidM, "MAIN-LETS", root}, {pidW, sidW, "W", wtX}})
	env := standingEnv(t, who(t, WhoOptions{Cwd: root, Orc: "MAIN-LETS", Session: sidM}))
	if _, present := env["standing_ok"]; present {
		t.Errorf("who without the probe: %v", env["standing_ok"])
	}
	o, _ := OrchestratorWith(context.Background(), wtX, sidW, StandingProbe{})
	if _, present := standingEnv(t, o)["standing_ok"]; present {
		t.Error("orchestrator without the probe carries standing_ok")
	}
	for _, o := range []WhoOptions{
		{Cwd: root, Orc: "MAIN-LETS", Probe: StandingProbe{FromName: "W"}},
		{Cwd: root, Orc: "MAIN-LETS", Probe: StandingProbe{FromAddr: sock(pidW)}},
		{Cwd: root, Probe: StandingProbe{FromName: "W", FromAddr: sock(pidW)}},
		{Cwd: root, Orc: "MAIN-LETS", OrcaRepos: true, Probe: StandingProbe{FromName: "W", FromAddr: sock(pidW)}},
	} {
		res, err := Who(context.Background(), o)
		if e, ok := err.(*Error); !ok || e.Code != ExitUsage || res.Error == nil || res.Error.Kind != "usage" {
			t.Errorf("%+v: err=%v", o.Probe, err)
		}
	}
	for _, p := range []StandingProbe{{FromName: "MAIN-LETS"}, {FromAddr: sock(pidOrc)}} {
		res, err := OrchestratorWith(context.Background(), wtX, sidW, p)
		if e, ok := err.(*Error); !ok || e.Code != ExitUsage || res.Error == nil || res.Error.Kind != "usage" {
			t.Errorf("%+v: err=%v", p, err)
		}
		checkStanding(t, "orchestrator usage", standingEnv(t, res), false, "usage")
	}
}

// Once the probe was passed, an envelope that never reached the decision still says
// standing_ok=false, with the error kind as the reason.
func TestStanding_ErrorEnvelopesAnswerFalse(t *testing.T) {
	probe := StandingProbe{FromName: "W", FromAddr: sock(pidW)}
	res, err := Who(context.Background(), WhoOptions{Cwd: root0(t), Orc: "MAIN-LETS", Probe: probe})
	if err == nil {
		t.Fatal("who outside a repo must fail")
	}
	checkStanding(t, "who not_in_repo", standingEnv(t, res), false, "not_in_repo")
	res, _ = Who(context.Background(), WhoOptions{Orc: "MAIN-LETS", Probe: StandingProbe{FromName: "W"}})
	checkStanding(t, "who usage", standingEnv(t, res), false, "usage")
	o, err := OrchestratorWith(context.Background(), root0(t), sidW, probe)
	if err == nil {
		t.Fatal("orchestrator outside a repo must fail")
	}
	checkStanding(t, "orchestrator not_in_repo", standingEnv(t, o), false, "not_in_repo")
}

// root0 is a directory outside any git repository.
func root0(t *testing.T) string { return t.TempDir() }

// A reused orchestrator name (two live sessions called MAIN-LETS) never inherits the
// record: self_record is null for both, and degraded[] says why.
func TestResolveByName_SelfRecordNeedsUniqueName(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "main")
	recordOrchestrators(t, root, "orchestrator: MAIN-LETS standing=on\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, "MAIN-LETS", gitRepo(t)}})
	res, env := orcEnv(t, root, sidM)
	if sr, present := env["self_record"]; !present || sr != nil {
		t.Errorf("a reused name inherits the record: %v", sr)
	}
	found := false
	for _, d := range res.Degraded {
		found = found || (d.Reason == "self_name_not_unique" && d.Detail == "MAIN-LETS")
	}
	if !found {
		t.Errorf("degraded must name self_name_not_unique: %+v", res.Degraded)
	}
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}})
	if res, _ := orcEnv(t, root, sidM); res.SelfRecord == nil || res.SelfRecord.Standing != "on" {
		t.Errorf("a unique name keeps its record: %+v", res.SelfRecord)
	}
}

// checkSender asserts the sender envelope: sender_ok and in_repo present as JSON
// bools, sender_reason set exactly when sender_ok is false.
func checkSender(t *testing.T, label string, env map[string]any, wantOK bool, wantReason string, wantInRepo bool) {
	t.Helper()
	ok, present := env["sender_ok"].(bool)
	inRepo, inPresent := env["in_repo"].(bool)
	reason, _ := env["sender_reason"].(string)
	if !present || !inPresent || ok != wantOK || reason != wantReason || inRepo != wantInRepo {
		t.Errorf("%s: sender_ok=%v (present=%v) sender_reason=%q in_repo=%v (present=%v), want %v %q %v", label, env["sender_ok"], present, reason, env["in_repo"], inPresent, wantOK, wantReason, wantInRepo)
	}
}

// `lets peers sender`: provenance only, any repo - the happy paths (a session of this
// repo, a live session of another repo) and every false reason of standingSender.
// No grant, binding or record is consulted: an unbound worker is a valid sender.
func TestSender(t *testing.T) {
	for _, c := range []struct {
		name     string
		extra    func(root, far string) []regRow
		unknown  []int
		fromName string
		fromAddr string
		ok       bool
		reason   string
		inRepo   bool
		wantName string
		wantCwd  func(root, wtX, wtY, far string) string
	}{
		{name: "bound worker in this repo", fromName: "W", fromAddr: sock(pidW), ok: true, inRepo: true, wantName: "W", wantCwd: func(_, wtX, _, _ string) string { return wtX }},
		{name: "unbound worker in this repo", fromName: "U", fromAddr: sock(pidU), ok: true, inRepo: true, wantName: "U", wantCwd: func(_, _, wtY, _ string) string { return wtY }},
		{name: "live session of another repo", extra: func(_, far string) []regRow { return []regRow{{107, sidO, "FAR", far}} }, fromName: "FAR", fromAddr: sock(107), ok: true, inRepo: false, wantName: "FAR", wantCwd: func(_, _, _, far string) string { return far }},
		{name: "addr not a socket", fromName: "W", fromAddr: "tcp:127.0.0.1:3", reason: "from_addr_unparseable"},
		{name: "degraded registry", unknown: []int{77}, fromName: "W", fromAddr: sock(pidW), reason: "liveness_unknown"},
		{name: "sender gone", fromName: "GHOST", fromAddr: sock(pidW), reason: "sender_not_live"},
		{name: "sender name twice", extra: func(_, far string) []regRow { return []regRow{{105, sidO, "W", far}} }, fromName: "W", fromAddr: sock(pidW), reason: "name_not_unique"},
		{name: "another process claims the name", fromName: "W", fromAddr: sock(pidU), reason: "pid_mismatch"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, wtX, wtY := standingRepo(t, "orchestrator: MAIN-LETS\n")
			far := gitRepo(t)
			rows := []regRow{{pidOrc, sidM, "MAIN-LETS", root}, {pidW, sidW, "W", wtX}, {pidU, sidN, "U", wtY}}
			if c.extra != nil {
				rows = append(rows, c.extra(root, far)...)
			}
			claudeHome(t, rows, c.unknown...)
			res, err := Sender(context.Background(), root, StandingProbe{FromName: c.fromName, FromAddr: c.fromAddr})
			if err != nil || !res.OK {
				t.Fatalf("sender: %+v %v", res, err)
			}
			env := standingEnv(t, res)
			checkSender(t, c.name, env, c.ok, c.reason, c.inRepo)
			if !c.ok {
				if res.Name != "" || res.Session6 != "" || res.Cwd != "" {
					t.Errorf("a false answer names no session: %+v", res)
				}
				return
			}
			if res.Name != c.wantName || len(res.Session6) != 6 || res.Cwd != c.wantCwd(root, wtX, wtY, far) {
				t.Errorf("sender identity: name=%q session6=%q cwd=%q", res.Name, res.Session6, res.Cwd)
			}
		})
	}
}

// Half a probe (or none) is a usage error that still answers sender_ok=false; outside
// a repo the answer is false with the error kind - fail closed.
func TestSender_FailsClosed(t *testing.T) {
	for _, p := range []StandingProbe{{}, {FromName: "W"}, {FromAddr: sock(pidW)}} {
		res, err := Sender(context.Background(), t.TempDir(), p)
		if err == nil || res.OK || res.Error == nil || res.Error.Kind != "usage" {
			t.Errorf("%+v: err=%v res=%+v", p, err, res)
		}
		checkSender(t, "usage", standingEnv(t, res), false, "usage", false)
	}
	res, err := Sender(context.Background(), t.TempDir(), StandingProbe{FromName: "W", FromAddr: sock(pidW)})
	if err == nil || res.Error == nil {
		t.Fatalf("outside a repo: err=%v res=%+v", err, res)
	}
	checkSender(t, "outside a repo", standingEnv(t, res), false, res.Error.Kind, false)
}
