//go:build unix

package worktreecmd

import (
	"fmt"
	"path/filepath"

	"github.com/restarter/lets-workflow/cli/internal/peername"
)

// leadName is the lead name a NEW team of callsign gets in the repository at
// mainRoot: peername.Lead(peername.RepoSlug(mainRoot), callsign), or callsign+"-lead"
// plus the reason (for the warn step) when no slug derives or the name fails
// peername.Valid. An existing team keeps the lead_name its team file records.
func leadName(mainRoot, callsign string) (name, why string) {
	short := callsign + "-lead"
	repo := peername.RepoSlug(mainRoot)
	if repo == "" {
		return short, fmt.Sprintf("no repo slug derives from %q; this team's lead is %s", filepath.Base(filepath.Clean(mainRoot)), short)
	}
	if n := peername.Lead(repo, callsign); peername.Valid(n) {
		return n, ""
	}
	return short, fmt.Sprintf("the lead name <repo>-%s-lead would exceed the session-name grammar; this team's lead is %s", callsign, short)
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
