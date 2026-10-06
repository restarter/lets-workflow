//go:build unix

package worktreecmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// conventionOf loads the active tracker convention of a main checkout with its
// diagnosis. callerRoot (the caller's checkout, "" = mainRoot) adds one warning
// when a board file there differs from the main checkout's - LETS reads only the
// main checkout's .claude/rules. pluginRoot "" reads $CLAUDE_PLUGIN_ROOT.
func conventionOf(callerRoot, mainRoot, pluginRoot string) (trackeradapter.Convention, trackeradapter.Diagnosis) {
	home, _ := os.UserHomeDir()
	tracker := letsconfig.ResolvedEnv(mainRoot, home, nil)["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	pr, err := initcmd.DetectPluginRoot(pluginRoot)
	if err != nil {
		pr = ""
	}
	c, d := trackeradapter.LoadConventionDiagnosed(mainRoot, tracker, pr)
	if callerRoot != "" && callerRoot != mainRoot && trackeradapter.NameRe.MatchString(tracker) {
		rel := filepath.Join(".claude", "rules", "tracker-"+tracker+".board.md")
		mine, err := os.ReadFile(filepath.Join(callerRoot, rel))
		if err == nil {
			theirs, terr := os.ReadFile(filepath.Join(mainRoot, rel))
			if terr != nil || !bytes.Equal(mine, theirs) {
				d.Warnings = append(d.Warnings, "board_not_in_main_checkout: "+rel+" in this worktree differs from the main checkout's - LETS reads naming only from "+filepath.Join(mainRoot, rel)+". Copy or commit it there")
			}
		}
	}
	return c, d
}

// loadConvention loads the active tracker convention for a main checkout.
func loadConvention(mainRoot string) trackeradapter.Convention {
	c, _ := conventionOf("", mainRoot, "")
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
	if !conv.Declared {
		// Templates without id: still name new branches; say so only when sweep's
		// legacy prefixes cannot see them (those branches accumulate).
		for _, k := range []string{"branch", "worktree-branch"} {
			tmpl := conv.Branch
			if k == "worktree-branch" {
				tmpl = conv.WorktreeBranch
			}
			p := templatePrefix(tmpl)
			if conv.Source[k] == trackeradapter.SourceDefault || strings.HasPrefix(p, "feature/") || strings.HasPrefix(p, "worktree-") {
				continue
			}
			res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "convention_templates_without_id: no id: is declared, so sweep only looks at feature/ and worktree- branches - branches named by " + tmpl + " are never swept. Declare id: to sweep them"})
		}
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
