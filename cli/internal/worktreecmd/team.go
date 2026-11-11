//go:build unix

package worktreecmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/peername"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// leadName picks callsign c's lead session name (lets-rpufs): the short
// <callsign>-lead, or <repo>-<callsign>-lead when a live session already holds
// the short one - session names are machine-wide, so another repo's lead counts.
// ok=false when neither name is free and valid: the callsign has no usable lead
// name (callsign_live). why is the warn step for the long form.
func leadName(mainRoot, c string, holder func(name string) string) (name, why string, ok bool) {
	short := c + "-lead"
	// The real cause, never a guess: a callsign like `-frog` passes
	// teamfile.ValidName but `-frog-lead` is no session name.
	cause := "is not a valid session name"
	if peername.Valid(short) {
		if cause = holder(short); cause == "" {
			return short, "", true
		}
	}
	repo := peername.RepoSlug(mainRoot)
	if long := peername.Lead(repo, c); repo != "" && peername.Valid(long) && holder(long) == "" {
		return long, fmt.Sprintf("the lead name %s %s; this team's lead is %s", short, cause, long), true
	}
	return short, cause, false
}

// recordedLeads maps every lead_name a team file in teamsDir records to its team
// (an old file without lead_name records <team>-lead, as lets worktree info reads
// it). An unreadable file is a warning: its name cannot be checked.
func recordedLeads(teamsDir string) (map[string]string, []string) {
	out, warns := map[string]string{}, []string(nil)
	paths, _ := filepath.Glob(filepath.Join(teamsDir, "*.md"))
	for _, p := range paths {
		team := strings.TrimSuffix(filepath.Base(p), ".md")
		if !teamfile.ValidName(team) {
			continue
		}
		n, err := teamfile.LeadName(teamsDir, team)
		if err != nil {
			warns = append(warns, "team file "+p+" is unreadable ("+err.Error()+"): its lead name is not checked")
			continue
		}
		if n == "" {
			n = team + "-lead"
		}
		out[n] = team
	}
	return out, warns
}
