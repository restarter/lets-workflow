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
	fmt.Fprintf(w, "cmux unavailable (%s) - open the worktree manually:\n\n    %s\n",
		res.Launch.Reason, res.Launch.FallbackCommand)
}
