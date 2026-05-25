//go:build unix

package worktreecmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// Info classifies dir relative to git worktrees:
//   - in main repo: in_worktree=false, worktree=main row, main_root=project root
//   - in worktree: in_worktree=true, worktree=this worktree's row, main_root=main repo
//   - outside any repo: ok=false, error.kind=not_in_repo, exit=ExitNotInRepo
//
// dir is typically the current working directory but the signature lets
// callers (and tests) target a specific path.
func Info(ctx context.Context, dir string) (*InfoResult, error) {
	res := &InfoResult{
		Envelope: Envelope{
			SchemaVersion: SchemaVersion,
			Subcommand:    "info",
			Steps:         []Step{},
		},
	}
	// Probe dir directly via `git -C` rather than relying on initcmd's
	// cwd-based DetectInsideWorktreeWithRoot — caller may have passed an
	// arbitrary path while the orchestrator's cwd is elsewhere.
	gitDirOut, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-dir").Output()
	if err != nil {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo"}
	}
	commonDirOut, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "not_in_repo", Message: "not inside a git repository"}
		return res, &Error{Code: ExitNotInRepo, Kind: "not_in_repo"}
	}
	resolve := func(p, base string) string {
		s := strings.TrimSpace(p)
		if !filepath.IsAbs(s) {
			if abs, err := filepath.Abs(filepath.Join(base, s)); err == nil {
				s = abs
			}
		}
		if real, err := filepath.EvalSymlinks(s); err == nil {
			s = real
		}
		return s
	}
	gd := resolve(string(gitDirOut), dir)
	cd := resolve(string(commonDirOut), dir)
	inWt := gd != cd
	mainRoot := filepath.Dir(cd) // <main>/.git → <main>

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
