//go:build unix

package cmuxcmd

import (
	"fmt"
	"io"
)

// RenderOpen writes a human-readable summary of an OpenResult.
func RenderOpen(w io.Writer, res *OpenResult) {
	if !res.OK {
		if res.Error != nil {
			fmt.Fprintf(w, "Error: %s: %s\n", res.Error.Kind, res.Error.Message)
		}
		return
	}
	if res.Launch == nil {
		return
	}
	if res.Launch.Launched {
		fmt.Fprintf(w, "Opened cmux workspace %q at %s\n", res.Launch.WorkspaceName, res.Launch.Path)
		return
	}
	if res.Launch.Reason == "already_open" {
		fmt.Fprintf(w, "A cmux workspace (%s %q) already targets %s - not opening a duplicate.\nUse --force to override, or open the worktree manually:\n\n    %s\n",
			res.Launch.ExistingRef, res.Launch.ExistingTitle, res.Launch.Path, res.Launch.FallbackCommand)
		return
	}
	fmt.Fprintf(w, "cmux unavailable (%s) - open the worktree manually:\n\n    %s\n",
		res.Launch.Reason, res.Launch.FallbackCommand)
}

// RenderRename writes a human-readable summary of a RenameResult.
func RenderRename(w io.Writer, res *RenameResult) {
	if !res.OK {
		if res.Error != nil {
			fmt.Fprintf(w, "Error: %s: %s\n", res.Error.Kind, res.Error.Message)
		}
		return
	}
	if res.Rename == nil {
		return
	}
	if res.Rename.Renamed {
		fmt.Fprintf(w, "Renamed cmux workspace %s → %q\n", res.Rename.Ref, res.Rename.Title)
		return
	}
	fmt.Fprintf(w, "cmux workspace not renamed (%s)\n", res.Rename.Reason)
}

// RenderNotify writes a human-readable summary of a NotifyResult. Notified=true
// means the notification was ENQUEUED in cmux, not confirmed seen by a human.
func RenderNotify(w io.Writer, res *NotifyResult) {
	if !res.OK {
		if res.Error != nil {
			fmt.Fprintf(w, "Error: %s: %s\n", res.Error.Kind, res.Error.Message)
		}
		return
	}
	if res.Notify == nil {
		return
	}
	if res.Notify.Notified {
		fmt.Fprintf(w, "Enqueued cmux notification to %s: %q\n", res.Notify.Ref, res.Notify.Title)
		return
	}
	fmt.Fprintf(w, "cmux notification not sent (%s)\n", res.Notify.Reason)
}
