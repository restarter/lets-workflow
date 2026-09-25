//go:build unix

package worktreecmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// BranchNameOptions configures BranchName.
type BranchNameOptions struct {
	Task      string // task id (validated with taskid.Valid)
	TitleFile string // file holding the untrusted tracker title (written by the Write tool)
	Worktree  bool   // render worktree-branch: instead of branch:
	// PluginRoot is the adapter fallback when the installed copy predates `## Worktree`
	// ("" reads $CLAUDE_PLUGIN_ROOT, which a Bash tool call does not carry).
	PluginRoot string
}

// maxSlug bounds the derived slug (Convention.Render refuses longer ones).
const maxSlug = 50

// Slugify derives a branch slug from an untrusted title: lowercase ASCII letters
// and digits joined by single `-`, at most 50 characters. An empty result (for
// example a fully non-Latin title) becomes "task".
func Slugify(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
		if b.Len() >= maxSlug {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > maxSlug {
		s = strings.TrimRight(s[:maxSlug], "-")
	}
	if s == "" {
		return "task"
	}
	return s
}

// BranchName renders the branch LETS creates for a task under the active
// convention. It is the ONE renderer: markdown calls it and never assembles a branch
// name itself. An undeclared convention renders the default templates.
func BranchName(ctx context.Context, dir string, o BranchNameOptions) (*BranchNameResult, error) {
	res := &BranchNameResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "branch-name", Steps: []Step{}}}
	fail := func(e *Error) (*BranchNameResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if !taskid.Valid(o.Task) {
		return fail(&Error{Code: ExitUsage, Kind: "task_invalid", Message: fmt.Sprintf("--task %q is not a task id", o.Task)})
	}
	title := ""
	if o.TitleFile != "" {
		data, err := os.ReadFile(o.TitleFile)
		if err != nil {
			return fail(&Error{Code: ExitUsage, Kind: "title_file_unreadable", Message: err.Error(), Cause: err})
		}
		title = string(data)
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = mainRoot
	home, _ := os.UserHomeDir()
	tracker := letsconfig.ResolvedEnv(mainRoot, home, nil)["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	pluginRoot, err := initcmd.DetectPluginRoot(o.PluginRoot)
	if err != nil {
		pluginRoot = ""
	}
	conv, reasons := trackeradapter.LoadConvention(mainRoot, tracker, pluginRoot)
	res.Reasons = reasons
	key, tmpl := "branch", trackeradapter.DefaultBranch
	if o.Worktree {
		key, tmpl = "worktree-branch", trackeradapter.DefaultWorktreeBranch
	}
	source := trackeradapter.SourceDefault
	if conv.Declared {
		if o.Worktree {
			tmpl = conv.WorktreeBranch
		} else {
			tmpl = conv.Branch
		}
		source = conv.Source[key]
	} else {
		conv = trackeradapter.Convention{}
	}
	slug := Slugify(title)
	name, err := conv.Render(tmpl, o.Task, slug)
	if err != nil {
		return fail(&Error{Code: ExitUsage, Kind: "branch_render_failed", Message: err.Error(), Cause: err})
	}
	if exec.CommandContext(ctx, "git", "check-ref-format", "--branch", name).Run() != nil {
		return fail(&Error{Code: ExitUsage, Kind: "branch_name_invalid", Message: fmt.Sprintf("%q is not a valid branch name", name)})
	}
	wtDir := dirName(o.Task, slug)
	var dirErr *Error
	if err := ValidateName(ctx, wtDir); err != nil {
		dirErr = &Error{Code: ExitUsage, Kind: "dir_name_invalid", Message: fmt.Sprintf("task %q renders the worktree dir %q, which is not a valid worktree name", o.Task, wtDir), Cause: err}
	} else if other := dirOccupant(ctx, mainRoot, filepath.Join(mainRoot, ".worktrees", wtDir), conv, name, o.Task); other != "" && other != o.Task {
		dirErr = &Error{Code: ExitWorktreeExists, Kind: "dir_collision",
			Message:     fmt.Sprintf("worktree dir %q is already held by task %q, not by task %q", wtDir, other, o.Task),
			Remediation: "remove or rename the existing worktree, then retry"}
	}
	if dirErr != nil {
		// Only a worktree render creates the dir; a plain render (take-task on a
		// feature branch) keeps its branch and reports the dir problem as a warning.
		if o.Worktree {
			return fail(dirErr)
		}
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: dirErr.Kind + ": " + dirErr.Message})
		wtDir = ""
	}
	res.Branch, res.Dir, res.Slug, res.Template, res.Source = name, wtDir, slug, tmpl, source
	res.OK = true
	return res, nil
}

// maxDirStem bounds the lowered stem of a hashed dir: 51 + `-` + 12 hex = 64,
// nameRE's limit.
const maxDirStem = 51

// dirHash is the suffix of a hashed dir: the first 12 hex (48 bits) of sha256 of
// the original task id. A package var so a test can force two ids onto one suffix.
var dirHash = func(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:12]
}

// dirName derives the worktree dir for a task: `<id>-<slug>` unchanged when it is
// already a valid name for nameRE. Otherwise (an upper-case id, a character outside
// the name class, or longer than 64) the lowered form, every other character mapped
// to `-`, cut to 51 at a `-` where possible, plus `-` and dirHash(id) - so two ids
// that lower to the same text (PROJ-42 and proj-42) still get distinct dirs. The
// branch and the task id keep the original spelling; only the directory changes.
// The caller validates the result.
func dirName(id, slug string) string {
	base := id + "-" + slug
	if nameRE.MatchString(base) {
		return base
	}
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	stem := b.String()
	if len(stem) > maxDirStem {
		stem = stem[:maxDirStem]
		if i := strings.LastIndexByte(stem, '-'); i > 0 {
			stem = stem[:i]
		}
	}
	return strings.TrimRight(stem, "-") + "-" + dirHash(id)
}

// dirOccupant returns the task id held by the worktree already at path: its
// task-state `task:` line, else the id the convention reads off its branch. ""
// when nothing is there, it is not a worktree, or no id is readable (detached
// HEAD, no task-state, a branch outside the convention) - `lets worktree create`
// then refuses the path itself (worktree_path_exists). An occupant on selfBranch,
// the branch this task renders, is selfID: a greedy id pattern can read a longer
// id off that branch (`PROJ-8-fix` from `worktree-PROJ-8-fix-login`).
func dirOccupant(ctx context.Context, mainRoot, path string, conv trackeradapter.Convention, selfBranch, selfID string) string {
	if _, err := os.Lstat(path); err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	top, err1 := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	here, err2 := filepath.EvalSymlinks(path)
	if err1 != nil || err2 != nil || top != here {
		return "" // a plain directory inside the main checkout, not a worktree
	}
	branch := currentBranchOf(ctx, path)
	slug, ok := taskstate.Slug(branch)
	if !ok {
		return ""
	}
	if st, err := taskstate.Read(filepath.Join(mainRoot, ".lets"), slug); err == nil && taskid.Valid(st.Task) {
		return st.Task
	}
	if branch == selfBranch {
		return selfID
	}
	if id, _, ok := conv.ParseBranch(branch, trackeradapter.Created); ok && taskid.Valid(id) {
		return id
	}
	return ""
}
