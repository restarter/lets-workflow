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
	"github.com/restarter/lets-workflow/cli/internal/taskid"
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
	conv, _ := trackeradapter.LoadConvention(mainRoot, tracker, pluginRoot)
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
	res.Branch, res.Slug, res.Template, res.Source = name, slug, tmpl, source
	res.OK = true
	return res, nil
}
