//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// tellFrom frames and sends from `from` to `to` in root, the way the orc skill does.
func tellFrom(t *testing.T, root, from, to string) *TellResult {
	t.Helper()
	fr, err := Frame(context.Background(), FrameOptions{Cwd: root, Session: from, ToSession: to, Kind: "tell"})
	if err != nil {
		t.Fatalf("Frame: %v", err)
	}
	if err := os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nhello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ := Tell(context.Background(), TellOptions{Cwd: root, ToSession: to, MsgID: fr.MsgID})
	return res
}

// agree: a resolved target is sendable by tell; a refused one is unreachable by tell
// too, and both name a reason (symmetric - plan review finding 3).
func agree(t *testing.T, r *Resolution, tr *TellResult) {
	t.Helper()
	if r.Target != nil && tr.Reason == "peer_unreachable" {
		t.Fatalf("resolution returned a target (%+v) that tell cannot reach (%+v)", r.Target, tr)
	}
	if r.Target == nil {
		if len(r.Refused) == 0 || r.Refused[0].Reason == "" {
			t.Fatalf("a refusal must name its reason: %+v", r)
		}
		if tr.Reason != "peer_unreachable" || tr.State == "" {
			t.Fatalf("resolution refused but tell did not (or gave no state): %+v", tr)
		}
	}
}

// F1: `claude -r` beside the still-running original - one session id, two live pids.
func TestResume_SameSessionTwoLivePids(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {5, sidM, "MAIN", root}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	tr := tellFrom(t, root, sidW, sidM)
	agree(t, r, tr)
	if r.Target != nil || r.Refused[0].Detail != "session_duplicated" || tr.State != "session_duplicated" {
		t.Errorf("duplicated session: resolution %+v, tell %+v", r, tr)
	}
}

// F1b: the duplicate pid sits outside this repo under another name - still one session.
func TestResume_DuplicateOutsideRepo(t *testing.T) {
	root := repoWithLets(t, "")
	elsewhere := t.TempDir()
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {5, sidM, "MAIN-R", filepath.Clean(elsewhere)}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	tr := tellFrom(t, root, sidW, sidM)
	agree(t, r, tr)
	if r.Target != nil || tr.State != "session_duplicated" {
		t.Errorf("a duplicate outside the repo still duplicates: resolution %+v, tell %+v", r, tr)
	}
}

// F2: resumed under a new pid with a new registry name; the old pid is dead.
func TestResume_NewPidNewName(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	claudeHome(t, []regRow{{7, sidM, "MAIN-2", root}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	tr := tellFrom(t, root, sidW, sidM)
	agree(t, r, tr)
	if r.Source != "bound" || r.Target == nil || r.Target.Session != sidM || tr.Reason != "claude_transport_model_send" {
		t.Errorf("renamed resume: resolution %+v, tell %+v", r, tr)
	}
}

// F3: resumed in a new Orca pane - the terminal handle in the role file is stale.
func TestResume_NewTerminal(t *testing.T) {
	root := repoWithLets(t, "orca")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\norca_terminal: term_old\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	useOrca(t, &fakeOps{terms: []orcaTerm{{Handle: "term_new", Path: root, AgentType: "claude"}}})
	claudeHome(t, []regRow{{7, sidM, "MAIN", root}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	tr := tellFrom(t, root, sidW, sidM)
	agree(t, r, tr)
	if r.Target == nil || r.Target.Send != "claude" {
		t.Errorf("a stale handle must fall back to the claude route: %+v", r)
	}
}

// F4: resumed from a directory outside the repo.
func TestResume_CwdOutsideRepo(t *testing.T) {
	root := repoWithLets(t, "")
	elsewhere := t.TempDir()
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	claudeHome(t, []regRow{{7, sidM, "MAIN", filepath.Clean(elsewhere)}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	tr := tellFrom(t, root, sidW, sidM)
	agree(t, r, tr)
	if r.Target != nil || r.Refused[0].Reason != "target_in_other_repo" || tr.State != "not_a_live_peer_of_this_repo" {
		t.Errorf("foreign cwd: resolution %+v, tell %+v", r, tr)
	}
}

// The old name was taken by another orchestrator: the live holder is the address.
func TestResume_OldNameTakenGoesToLiveHolder(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n") // A, resumed as MAIN-2
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN\npid: 8\nset: x\n") // B, took MAIN
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	claudeHome(t, []regRow{{7, sidM, "MAIN-2", root}, {8, sidN, "MAIN", root}, {3, sidW, "W", root}})
	if r := resolveIn(t, root, sidW); r.Target == nil || r.Target.Session != sidN {
		t.Errorf("the live holder of the bound name is the target: %+v", r)
	}
}

// Nobody holds the name live and two role files registered it: refuse, never pick.
// agree() does not apply: both sessions are reachable by id, so tell succeeds - the
// refusal is about WHICH session the binding means, which only resolution can answer.
func TestResume_RegisteredNameAmbiguousRefuses(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN\npid: 2\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	claudeHome(t, []regRow{{7, sidM, "MAIN-2", root}, {8, sidN, "MAIN-3", root}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	if r.Target != nil || r.Reason != "bound_ambiguous" || len(r.Refused) != 2 {
		t.Errorf("an ambiguous fallback must refuse with both candidates: %+v", r)
	}
}
