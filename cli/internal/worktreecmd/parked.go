//go:build unix

package worktreecmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// ParkedEntry is one parked branch: its task, the park commit and the branch tip
// now (a tip that moved past the park is still listed - Switch keeps such a park).
type ParkedEntry struct {
	Task    string `json:"task"`
	Branch  string `json:"branch"`
	ParkSha string `json:"park_sha"`
	Tip     string `json:"tip"`
}

// ParkedResult is `lets worktree parked --team <c>`: the branches that team parked
// (Owned), and parks that name no team (Unowned). Another team's parks are left out.
type ParkedResult struct {
	Envelope
	Team    string        `json:"team"`
	Owned   []ParkedEntry `json:"owned"`
	Unowned []ParkedEntry `json:"unowned"`
}

// Parked is the Go owner of the park inventory: every task-state file with a
// `park:` key whose branch still exists locally, split by `park_team:`.
func Parked(ctx context.Context, dir, team string) (*ParkedResult, error) {
	res := &ParkedResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "parked", Steps: []Step{}}, Team: team, Owned: []ParkedEntry{}, Unowned: []ParkedEntry{}}
	fail := func(e *Error) (*ParkedResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if !teamfile.ValidName(team) {
		return fail(&Error{Code: ExitUsage, Kind: "team_invalid", Message: fmt.Sprintf("--team %q is not a team name", team)})
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = mainRoot
	letsDir := filepath.Join(mainRoot, ".lets")
	slugs, err := taskstate.Slugs(letsDir)
	if err != nil {
		return fail(&Error{Code: ExitFilesystem, Kind: "task_state_unreadable", Message: err.Error(), Cause: err})
	}
	refs, err := forEachRef(ctx, mainRoot)
	if err != nil {
		return fail(&Error{Code: ExitGitFailed, Kind: "git_failed", Message: "for-each-ref: " + err.Error(), Cause: err})
	}
	bySlug := map[string]string{}
	for _, b := range strings.Fields(string(refs)) {
		if s, ok := taskstate.Slug(b); ok {
			bySlug[s] = b
		}
	}
	for _, slug := range slugs {
		st, err := taskstate.Read(letsDir, slug)
		if err != nil || st.Park == "" {
			continue
		}
		branch, ok := bySlug[slug]
		if !ok {
			res.Steps = append(res.Steps, Step{Status: StepWarn, Message: fmt.Sprintf("%s records park %s, but no local branch has that slug", taskstate.Path(letsDir, slug), st.Park)})
			continue
		}
		e := ParkedEntry{Task: st.Task, Branch: branch, ParkSha: st.Park, Tip: revParse(ctx, mainRoot, "refs/heads/"+branch)}
		switch st.ParkTeam {
		case team:
			res.Owned = append(res.Owned, e)
		case "":
			res.Unowned = append(res.Unowned, e)
		}
	}
	sort.Slice(res.Owned, func(i, j int) bool { return res.Owned[i].Branch < res.Owned[j].Branch })
	sort.Slice(res.Unowned, func(i, j int) bool { return res.Unowned[i].Branch < res.Unowned[j].Branch })
	res.OK = true
	return res, nil
}
