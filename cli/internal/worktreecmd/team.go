//go:build unix

package worktreecmd

import (
	"github.com/restarter/lets-workflow/cli/internal/peername"
)

// leadName is the lead name a NEW team of callsign gets in the repository at
// mainRoot: peername.Lead(peername.RepoSlug(mainRoot), callsign), or callsign+"-lead"
// with ok=false when no slug derives or the name fails peername.Valid. An existing
// team keeps the lead_name its team file records.
func leadName(mainRoot, callsign string) (name string, ok bool) {
	if repo := peername.RepoSlug(mainRoot); repo != "" {
		if n := peername.Lead(repo, callsign); peername.Valid(n) {
			return n, true
		}
	}
	return callsign + "-lead", false
}

// liveLeads reads the registry once; a callsign's lead runs when a live session is
// named leadName(mainRoot, c). The name embeds the repository, so another repo's
// lead of the same callsign never collides. A degraded registry is a warn step.
func liveLeads(res *TeamInitResult, mainRoot string) func(callsign string) bool {
	live := liveSessionNames(res)
	return func(c string) bool {
		n, _ := leadName(mainRoot, c)
		return live[n]
	}
}
