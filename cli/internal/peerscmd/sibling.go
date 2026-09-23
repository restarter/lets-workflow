//go:build unix

package peerscmd

import (
	"context"
	"sort"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// listOrcaRepos is orcacmd.ListRepos, a seam so tests supply Orca's repo list.
var listOrcaRepos = orcacmd.ListRepos

// repoByIndex resolves an index through the same seam `who --orca-repos` numbered with.
func repoByIndex(ctx context.Context, idx int) (string, *orcacmd.Failure) {
	info, f := listOrcaRepos(ctx)
	if f != nil {
		return "", f
	}
	for _, r := range info.Repos {
		if r.Index == idx {
			return r.Path, nil
		}
	}
	return "", &orcacmd.Failure{Reason: "repo_index_unknown", Verb: "repo list"}
}

// errBudget marks a sibling lookup that ran out of the resolution budget; the
// caller reports budget_exhausted, never a configuration verdict.
const errBudget = "budget_exhausted"

// siblingOrchestrator is the reachability rule for a BOUND name that this repo
// cannot address (lets-rules Boundaries, "Carve-out - bound sibling sessions"):
// only under Orca, only sessions whose cwd lies in a checkout Orca lists, and only
// when exactly one live orchestrator there carries the name - by the same two
// passes as the in-repo bound resolution. It reads each sibling's peers dir and
// registry rows and writes nothing there: role moves are applied in memory only,
// and no heal / prune / lock runs on a sibling.
// local: this repo's role file for the name whose session sits outside (shape A),
// or nil. ok=false: nothing across the repos holds the name and there is no local
// role file - the caller keeps orchestrator_not_registered.
func siblingOrchestrator(ctx context.Context, rc *repoContext, name string, local *roleFile) (p *Peer, ref *Refused, ok bool) {
	ref = &Refused{Name: name, Reason: "target_in_other_repo", Sibling: true}
	if local != nil {
		ref.Scope, ref.Session6 = local.Scope, session6(local.Session)
	}
	hasLocal := local != nil
	budget := func() (*Peer, *Refused, bool) {
		ref.Reason, ref.Detail = errBudget, ""
		return nil, ref, true
	}
	if !orcaSelected(rc.root, false) {
		ref.Detail = "orca_not_selected"
		return nil, ref, hasLocal
	}
	info, f := listOrcaRepos(ctx)
	if ctx.Err() != nil {
		return budget()
	}
	if f != nil {
		ref.Detail = "orca_unavailable"
		return nil, ref, hasLocal
	}
	type cand struct {
		idx        int
		path       string
		f          roleFile
		live, regd bool
	}
	// ownerOf: the Orca repo (never this one) whose main checkout holds the session's live cwd.
	ownerOf := func(sid string) (int, string, bool) {
		e, found := rc.snap.Find(sid)
		if !found {
			return 0, "", false
		}
		_, main := gitutil.DetectInsideWorktreeAt(e.Cwd)
		if main == "" || fsutil.SameDir(main, rc.mainRoot) {
			return 0, "", false
		}
		for _, r := range info.Repos {
			if fsutil.SameDir(main, r.Path) {
				return r.Index, r.Path, true
			}
		}
		return 0, "", false
	}
	bySession := map[string]cand{}
	add := func(rf roleFile) {
		if rf.Role != "orchestrator" || rc.snap.Liveness(rf.Session, rf.Pid) != ccregistry.Alive {
			return
		}
		live, regd := liveName(rc.snap, rf) == name, rf.Name == name
		if !live && !regd {
			return
		}
		idx, path, owned := ownerOf(rf.Session)
		if !owned {
			return
		}
		if prev, seen := bySession[rf.Session]; seen { // one session, one candidate
			live, regd = live || prev.live, regd || prev.regd
		}
		bySession[rf.Session] = cand{idx, path, rf, live, regd}
	}
	for _, r := range info.Repos { // shape B: the role file lives in the sibling
		if fsutil.SameDir(r.Path, rc.mainRoot) {
			continue
		}
		roles, _ := loadRoles(r.Path)
		// A role whose session was re-minted (/clear, resume) is found under its live
		// id, as loadRepoFor would - in memory only; persistMoves never runs here.
		applyMoves(roles, reconcileRoles(roles, rc.snap))
		for _, rf := range roles {
			add(rf)
		}
		if ctx.Err() != nil {
			return budget()
		}
	}
	if local != nil { // shape A: the role file is here, the session there
		add(*local)
	}
	if ctx.Err() != nil { // ownerOf runs git; an expiry there is not "repo not in Orca"
		return budget()
	}
	var lives, regs []cand
	for _, c := range bySession {
		if c.live {
			lives = append(lives, c)
		} else {
			regs = append(regs, c)
		}
	}
	picked := lives
	if len(picked) == 0 {
		picked = regs
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].f.Session < picked[j].f.Session })
	switch len(picked) {
	case 0:
		if !hasLocal {
			return nil, nil, false
		}
		ref.Detail = "repo_not_in_orca"
		return nil, ref, true
	case 1:
	default:
		ref.Reason, ref.Detail = "bound_ambiguous", "sibling_repos"
		return nil, ref, true
	}
	c := picked[0]
	prc, err := loadRepoFor(ctx, c.path, callerSelectsOrca(rc.root, false))
	if ctx.Err() != nil {
		return budget()
	}
	if err != nil {
		ref.Detail = "repo_not_in_orca"
		return nil, ref, true
	}
	peer := findPeer(prc, ctx, c.f.Session)
	if ctx.Err() != nil {
		return budget()
	}
	if peer == nil || peer.Send == "none" || peer.Send == "" {
		ref.Reason, ref.Detail = "target_unsendable", ""
		if peer != nil {
			ref.Detail = peer.Reason
		}
		return nil, ref, true
	}
	idx := c.idx
	out := *peer
	out.Role, out.Scope, out.RepoIndex = "orchestrator", c.f.Scope, &idx
	return &out, nil, true
}
