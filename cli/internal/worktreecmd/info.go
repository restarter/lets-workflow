//go:build unix

package worktreecmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
)

// Info classifies dir relative to git worktrees:
//   - in main repo: in_worktree=false, worktree=main row, main_root=project root
//   - in worktree: in_worktree=true, worktree=this worktree's row, main_root=main repo
//   - outside any repo: ok=false, error.kind=not_in_repo, exit=ExitNotInRepo
//
// dir is typically the current working directory but the signature lets
// callers (and tests) target a specific path.
func Info(ctx context.Context, dir string) (*InfoResult, error) {
	_ = ctx // worktreecmd-context discipline; detector uses its own timeout
	res := &InfoResult{
		Envelope: Envelope{
			SchemaVersion: SchemaVersion,
			Subcommand:    "info",
			Steps:         []Step{},
		},
	}
	// Use the consolidated initcmd detector (path-anchored variant).
	inWt, mainRoot := initcmd.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo"}
	}

	res.ProjectRoot = mainRoot
	res.MainRoot = mainRoot
	res.InWorktree = inWt

	branch := ""
	if out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	headOut, _ := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	e := porcelainEntry{Path: dir, Branch: branch, HEAD: strings.TrimSpace(string(headOut))}
	wt := annotateWorktree(ctx, mainRoot, e)
	if inWt {
		wt.Name = filepath.Base(dir)
	}
	res.Worktree = &wt
	res.OK = true
	return res, nil
}
