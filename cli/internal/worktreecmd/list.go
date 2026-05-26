//go:build unix

package worktreecmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// List enumerates git worktrees under projectRoot, annotating each row with
// LETS-specific state (interactive vs agent vs other, symlink presence,
// uncommitted-changes counts).
func List(ctx context.Context, projectRoot string) (*ListResult, error) {
	res := &ListResult{
		Envelope: Envelope{
			SchemaVersion: SchemaVersion,
			Subcommand:    "list",
			ProjectRoot:   projectRoot,
			Steps:         []Step{},
		},
	}
	out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		res.OK = false
		res.Error = &ErrorInfo{Kind: "git_worktree_list_failed", Message: redactCreds(err.Error())}
		return res, &Error{Code: ExitGitFailed, Kind: "git_worktree_list_failed", Cause: err}
	}
	entries := parsePorcelain(string(out))
	for _, e := range entries {
		wt := annotateWorktree(ctx, projectRoot, e)
		if wt.IsMain {
			tmp := wt
			res.Main = &tmp
		} else {
			res.Worktrees = append(res.Worktrees, wt)
		}
	}
	res.OK = true
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("listed %d worktrees", len(res.Worktrees))})
	return res, nil
}

type porcelainEntry struct {
	Path     string
	HEAD     string
	Branch   string
	Bare     bool
	Detached bool
	Locked   bool
	Prunable bool
}

func parsePorcelain(s string) []porcelainEntry {
	var entries []porcelainEntry
	var cur porcelainEntry
	flush := func() {
		if cur.Path != "" {
			entries = append(entries, cur)
		}
		cur = porcelainEntry{}
	}
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			cur.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "bare":
			cur.Bare = true
		case line == "detached":
			cur.Detached = true
		case strings.HasPrefix(line, "locked"):
			cur.Locked = true
		case strings.HasPrefix(line, "prunable"):
			cur.Prunable = true
		}
	}
	flush()
	return entries
}

func annotateWorktree(ctx context.Context, projectRoot string, e porcelainEntry) WorktreeInfo {
	wt := WorktreeInfo{
		Path:     e.Path,
		Branch:   e.Branch,
		Head:     shortSHA(e.HEAD),
		IsMain:   sameDir(e.Path, projectRoot),
		Locked:   e.Locked,
		Prunable: e.Prunable,
		Detached: e.Detached,
	}
	if !wt.IsMain {
		wt.Name = filepath.Base(e.Path)
	}
	rel, err := filepath.Rel(projectRoot, e.Path)
	switch {
	case wt.IsMain:
		wt.Kind = ""
	case err == nil && strings.HasPrefix(rel, ".worktrees"+string(filepath.Separator)):
		wt.Kind = "interactive"
	case err == nil && strings.HasPrefix(rel, filepath.Join(".claude", "worktrees")+string(filepath.Separator)):
		wt.Kind = "agent"
	default:
		wt.Kind = "other"
	}
	if fi, err := os.Lstat(filepath.Join(e.Path, ".lets")); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		wt.LetsSymlinked = true
	}
	if fi, err := os.Lstat(filepath.Join(e.Path, ".beads", ".env")); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		wt.BeadsSymlinked = true
	}
	statusOut, _ := exec.CommandContext(ctx, "git", "-C", e.Path, "status", "--porcelain").Output()
	modified, untracked := 0, 0
	for _, l := range strings.Split(string(statusOut), "\n") {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "??") {
			untracked++
		} else {
			modified++
		}
	}
	wt.ChangesModified = modified
	wt.ChangesUntracked = untracked
	wt.ChangesClean = modified == 0 && untracked == 0
	return wt
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// sameDir compares two paths after symlink resolution + Abs normalization.
// macOS /var/folders vs /private/var/folders mismatch (initRepo uses
// realTempDir to dodge this, but porcelain output is raw git data).
func sameDir(a, b string) bool {
	resolve := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return real
		}
		return p
	}
	return resolve(a) == resolve(b)
}
