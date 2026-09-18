//go:build unix

package worktreecmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// loadConvention loads the active tracker convention for a main checkout.
func loadConvention(mainRoot string) trackeradapter.Convention {
	home, _ := os.UserHomeDir()
	tracker := letsconfig.ResolvedEnv(mainRoot, home, nil)["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	pluginRoot, err := initcmd.DetectPluginRoot("")
	if err != nil {
		pluginRoot = ""
	}
	c, _ := trackeradapter.LoadConvention(mainRoot, tracker, pluginRoot)
	return c
}

// templatePrefix is the literal text before {id} (a pre-filter only).
func templatePrefix(tmpl string) string {
	if i := strings.Index(tmpl, "{id}"); i >= 0 {
		return tmpl[:i]
	}
	return tmpl
}

// Sweep finds local task branches already merged into origin/<merge> (or <merge>).
// Candidates are only the branches the convention's created shapes accept - a
// template's literal prefix is merely a pre-filter, so a template that starts with
// {id} never widens the sweep to every branch. Dry-run by default; apply deletes
// the merged ones through deleteBranch. Unmerged branches (a squash merge is not
// detectable) are listed, never deleted.
func Sweep(ctx context.Context, dir string, apply bool) (*SweepResult, error) {
	res := &SweepResult{
		Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "sweep", Steps: []Step{}},
		Merged:   []string{}, Unmerged: []string{}, Deleted: []string{}, Applied: apply,
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	res.ProjectRoot = mainRoot
	merge := mergeBranch(mainRoot)
	conv := loadConvention(mainRoot)

	prefixes := []string{"feature/", "worktree-"}
	accept := func(string) bool { return true }
	if conv.Declared && conv.ID != nil {
		prefixes = []string{templatePrefix(conv.Branch), templatePrefix(conv.WorktreeBranch)}
		accept = func(b string) bool { _, _, ok := conv.ParseBranch(b, trackeradapter.Created); return ok }
	} else if conv.Declared {
		res.OK = true
		res.Steps = append(res.Steps, Step{Status: StepSkip, Message: "the convention declares id: nothing - no branch is a task branch"})
		return res, nil
	}
	merged, unmerged := SweepCandidates(ctx, mainRoot, merge, prefixes)
	for _, b := range merged {
		if accept(b) {
			res.Merged = append(res.Merged, b)
		}
	}
	for _, b := range unmerged {
		if accept(b) {
			res.Unmerged = append(res.Unmerged, b)
		}
	}
	for _, b := range res.Unmerged {
		res.Steps = append(res.Steps, Step{Status: StepSkip, Message: fmt.Sprintf("%s: unmerged (maybe squashed) - kept", b)})
	}
	if apply {
		for _, b := range res.Merged {
			msg, e := deleteBranch(ctx, mainRoot, b, false, merge)
			if e != nil {
				res.Steps = append(res.Steps, Step{Status: StepWarn, Message: fmt.Sprintf("%s: %s", b, e.Message)})
				continue
			}
			res.Deleted = append(res.Deleted, b)
			res.Steps = append(res.Steps, Step{Status: StepOK, Message: msg})
		}
	}
	res.OK = true
	return res, nil
}

// conventionCandidate returns the local branch the convention ties to a worktree
// name: a branch in a created shape whose id equals the id the name carries (in a
// created or accepted shape). "" when the convention is undeclared or nothing matches.
func conventionCandidate(ctx context.Context, mainRoot, name string) string {
	conv := loadConvention(mainRoot)
	id, _, ok := conv.ParseBranch(name, trackeradapter.CreatedAndAccepted)
	if !ok {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "-C", mainRoot, "for-each-ref", "--format=%(refname:short)", "refs/heads/").Output()
	if err != nil {
		return ""
	}
	for _, b := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if bid, _, ok := conv.ParseBranch(b, trackeradapter.Created); ok && bid == id {
			return b
		}
	}
	return ""
}
