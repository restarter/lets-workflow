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
	fmt.Fprintf(w, "Symlinks: lets=%v store=%v\n", res.Worktree.LetsSymlinked, res.Worktree.StoreLinked)
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
		"NAME", "BRANCH", "KIND", "LETS", "STORE", "CHANGES", "PATH")
	for _, wt := range res.Worktrees {
		changes := "clean"
		if !wt.ChangesClean {
			changes = fmt.Sprintf("%dm/%du", wt.ChangesModified, wt.ChangesUntracked)
		}
		fmt.Fprintf(w, "%-12s %-22s %-12s %-7v %-7v %-12s %s\n",
			wt.Name, wt.Branch, wt.Kind, wt.LetsSymlinked, wt.StoreLinked, changes, wt.Path)
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
			store := "local"
			if res.Worktree.StoreLinked {
				store = "shared"
			}
			fmt.Fprintf(w, "LETS:        %s\n", lets)
			fmt.Fprintf(w, "Store:       %s\n", store)
		}
		changes := "clean"
		if !res.Worktree.ChangesClean {
			changes = fmt.Sprintf("%d modified, %d untracked",
				res.Worktree.ChangesModified, res.Worktree.ChangesUntracked)
		}
		fmt.Fprintf(w, "Changes:     %s\n", changes)
	}
}

// RenderSteps writes an envelope's steps, then its error, one line each.
func RenderSteps(w io.Writer, env Envelope) {
	for _, s := range env.Steps {
		fmt.Fprintf(w, "[%s] %s\n", s.Status, s.Message)
	}
	if env.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", env.Error.Message)
		if env.Error.Remediation != "" {
			fmt.Fprintf(w, "Hint: %s\n", env.Error.Remediation)
		}
	}
}

// RenderTaskState writes a task-state result as key: value lines.
func RenderTaskState(w io.Writer, res *TaskStateResult) {
	if res.TaskState == nil {
		RenderSteps(w, res.Envelope)
		return
	}
	ts := res.TaskState
	fmt.Fprintf(w, "path: %s\nwritten: %v\n", ts.Path, ts.Written)
	if ts.Reason != "" {
		fmt.Fprintf(w, "reason: %s\n", ts.Reason)
	}
	for _, kv := range [][2]string{{"task", ts.Task}, {"start", ts.Start}, {"session", ts.Session}, {"origin", ts.Origin}, {"orc", ts.Orc}} {
		if kv[1] != "" {
			fmt.Fprintf(w, "%s: %s\n", kv[0], kv[1])
		}
	}
	if ts.Rebound != nil {
		fmt.Fprintf(w, "rebound from: %s\n", ts.Rebound.From)
	}
}

// RenderSweep writes merged / unmerged / deleted branches.
func RenderSweep(w io.Writer, res *SweepResult) {
	if !res.OK && res.Error != nil {
		fmt.Fprintf(w, "Error: %s\n", res.Error.Message)
		return
	}
	for _, b := range res.Merged {
		state := "merged"
		for _, d := range res.Deleted {
			if d == b {
				state = "deleted"
			}
		}
		fmt.Fprintf(w, "%-10s %s\n", state, b)
	}
	for _, b := range res.Unmerged {
		fmt.Fprintf(w, "%-10s %s (maybe squashed)\n", "unmerged", b)
	}
	if !res.Applied && len(res.Merged) > 0 {
		fmt.Fprintln(w, "\nDry run - pass --apply to delete the merged branches.")
	}
}
