//go:build unix

package orcacmd

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
	l := res.Launch
	switch {
	case l == nil:
	case l.Launched:
		fmt.Fprintf(w, "Orca opened worktree %s at %s (branch %s)\n", l.WorkspaceName, l.Path, l.Branch)
	case l.Reason == "already_open":
		fmt.Fprintf(w, "An Orca worktree named %s is already open at %s - see its card.\n", l.WorkspaceName, l.Path)
	default:
		fmt.Fprintf(w, "Orca unavailable (%s) - fall back with:\n\n    %s\n", l.Reason, l.FallbackCommand)
	}
}

// RenderNotify writes a human-readable summary of a NotifyResult.
func RenderNotify(w io.Writer, res *NotifyResult) {
	if !res.OK || res.Notify == nil {
		if res.Error != nil {
			fmt.Fprintf(w, "Error: %s: %s\n", res.Error.Kind, res.Error.Message)
		}
		return
	}
	if res.Notify.Notified {
		fmt.Fprintf(w, "Orca card comment set on %s\n", res.Notify.Target)
		return
	}
	fmt.Fprintf(w, "Orca notification not sent (%s)\n", res.Notify.Reason)
}

// RenderStatus writes a human-readable summary of a StatusResult.
func RenderStatus(w io.Writer, res *StatusResult) {
	s := res.Status
	if s == nil {
		return
	}
	if s.Running {
		fmt.Fprintf(w, "Orca %s running (%s)\n", s.Version, s.Bin)
		return
	}
	fmt.Fprintf(w, "Orca not usable: %s\n", s.Reason)
}
