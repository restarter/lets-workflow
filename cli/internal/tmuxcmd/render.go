//go:build unix

package tmuxcmd

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
		if res.Launch.InExistingSession {
			fmt.Fprintf(w, "Opened tmux window %s at %s\n", res.Launch.Target, res.Launch.Path)
			return
		}
		fmt.Fprintf(w, "Created detached tmux session %q (%s) at %s\nAttach with:\n\n    %s\n",
			res.Launch.WorkspaceName, res.Launch.Target, res.Launch.Path, res.Launch.AttachCommand)
		return
	}
	if res.Launch.Reason == "already_open" {
		fmt.Fprintf(w, "A tmux pane (%s %q) already lives at %s - not opening a duplicate.\nUse --force to override, or open the worktree manually:\n\n    %s\n",
			res.Launch.ExistingTarget, res.Launch.ExistingTitle, res.Launch.Path, res.Launch.FallbackCommand)
		return
	}
	fmt.Fprintf(w, "tmux unavailable (%s) - open the worktree manually:\n\n    %s\n",
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
		fmt.Fprintf(w, "Renamed tmux window %s -> %q\n", res.Rename.Target, res.Rename.Title)
		return
	}
	fmt.Fprintf(w, "tmux window not renamed (%s)\n", res.Rename.Reason)
}

// RenderNotify writes a human-readable summary of a NotifyResult. Notified=true
// means the message was displayed on the status line, not confirmed seen.
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
		fmt.Fprintf(w, "Displayed tmux notification on %d client(s): %q\n", res.Notify.Clients, res.Notify.Title)
		return
	}
	fmt.Fprintf(w, "tmux notification not sent (%s)\n", res.Notify.Reason)
}
