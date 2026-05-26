//go:build unix

package worktreecmd

import (
	"fmt"
	"io"
	"strings"
)

// RenderCreate writes the human-readable summary of a CreateResult.
// Format spec (consistent across all 4 renderers in this package):
//   - first line: "Worktree created: <path>" or "Error: <message>"
//   - on success: "Branch: <name> (<mode>)", "Symlinks: lets=… beads=…",
//     "Next: cd <path> && claude"
//   - on error: optional "Hint: <remediation>" line; optional "Residual
//     paths (clean up manually): …" line when rollback left state behind.
//
// Moved out of cli/internal/cli/worktree.go in review S-8 so the domain
// package owns presentation alongside the envelope shape — mirrors the
// updatecmd.PrintReport precedent.
func RenderCreate(w io.Writer, res *CreateResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		if res.Error.Remediation != "" {
			fmt.Fprintf(w, "Hint: %s\n", res.Error.Remediation)
		}
		if res.Rollback != nil && len(res.Rollback.Residual) > 0 {
			fmt.Fprintf(w, "Residual paths (clean up manually): %s\n", strings.Join(res.Rollback.Residual, ", "))
		}
		return
	}
	if res.Worktree == nil {
		return
	}
	fmt.Fprintf(w, "Worktree created: %s\n", res.Worktree.Path)
	fmt.Fprintf(w, "Branch: %s (%s)\n", res.Worktree.Branch, res.Worktree.BranchMode)
	fmt.Fprintf(w, "Symlinks: lets=%v beads=%v\n", res.Worktree.LetsSymlinked, res.Worktree.BeadsSymlinked)
	fmt.Fprintf(w, "Next: cd %s && claude\n", res.Worktree.Path)
}

// RenderRemove writes the human-readable summary of a RemoveResult.
// On --branch-only success (Path empty) headlines "Branch deleted: …"
// rather than the misleading "Worktree removed: <blank>".
func RenderRemove(w io.Writer, res *RemoveResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		if res.Error.Remediation != "" {
			fmt.Fprintf(w, "Hint: %s\n", res.Error.Remediation)
		}
		return
	}
	if res.Removed == nil {
		return
	}
	if res.Removed.Path == "" {
		fmt.Fprintf(w, "Branch deleted: %s\n", res.Removed.Branch)
		return
	}
	fmt.Fprintf(w, "Worktree removed: %s\n", res.Removed.Path)
	branchStatus := "kept"
	if res.Removed.BranchDeleted {
		branchStatus = "deleted"
	}
	fmt.Fprintf(w, "Branch: %s (%s)\n", res.Removed.Branch, branchStatus)
	if res.Removed.Forced {
		fmt.Fprintln(w, "Forced: true")
	}
}

// RenderList writes a fixed-width text table of all worktrees plus a
// "<N> worktrees (main: <branch>)" footer.
func RenderList(w io.Writer, res *ListResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		return
	}
	fmt.Fprintf(w, "%-12s %-22s %-12s %-7s %-7s %-12s %s\n",
		"NAME", "BRANCH", "KIND", "LETS", "BEADS", "CHANGES", "PATH")
	for _, wt := range res.Worktrees {
		changes := "clean"
		if !wt.ChangesClean {
			changes = fmt.Sprintf("%dm/%du", wt.ChangesModified, wt.ChangesUntracked)
		}
		fmt.Fprintf(w, "%-12s %-22s %-12s %-7v %-7v %-12s %s\n",
			wt.Name, wt.Branch, wt.Kind, wt.LetsSymlinked, wt.BeadsSymlinked, changes, wt.Path)
	}
	mainBranch := ""
	if res.Main != nil {
		mainBranch = res.Main.Branch
	}
	fmt.Fprintf(w, "\n%d worktrees (main: %s)\n", len(res.Worktrees), mainBranch)
}

// RenderInfo writes the human-readable summary of an InfoResult — a
// key:value block whose contents differ depending on `in_worktree`.
func RenderInfo(w io.Writer, res *InfoResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		return
	}
	fmt.Fprintf(w, "In worktree: %v\n", res.InWorktree)
	if res.Worktree != nil {
		fmt.Fprintf(w, "Path:        %s\n", res.Worktree.Path)
		if res.InWorktree {
			fmt.Fprintf(w, "Main repo:   %s\n", res.MainRoot)
		}
		fmt.Fprintf(w, "Branch:      %s\n", res.Worktree.Branch)
		if res.InWorktree {
			lets := "local"
			if res.Worktree.LetsSymlinked {
				lets = "symlinked"
			}
			beads := "local"
			if res.Worktree.BeadsSymlinked {
				beads = "shared"
			}
			fmt.Fprintf(w, "LETS:        %s\n", lets)
			fmt.Fprintf(w, "Beads:       %s\n", beads)
		}
		changes := "clean"
		if !res.Worktree.ChangesClean {
			changes = fmt.Sprintf("%d modified, %d untracked",
				res.Worktree.ChangesModified, res.Worktree.ChangesUntracked)
		}
		fmt.Fprintf(w, "Changes:     %s\n", changes)
	}
}
