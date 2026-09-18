//go:build unix

package handoffcmd

import (
	"fmt"
	"io"
)

// RenderTargets prints one agent terminal per line.
func RenderTargets(w io.Writer, res *TargetsResult) {
	if res.Targets == nil {
		return
	}
	if !res.Targets.Available {
		fmt.Fprintf(w, "orca unavailable: %s\n", res.Targets.Reason)
		return
	}
	for _, t := range res.Targets.Terminals {
		fmt.Fprintf(w, "%s  %s  (%s %s)\n", t.Handle, t.Title, t.Agent, t.State)
	}
}

// RenderSend prints the delivery outcome.
func RenderSend(w io.Writer, res *SendResult) {
	if res.Send == nil {
		return
	}
	fmt.Fprintf(w, "delivery: %s %s -> %s\n", res.Send.Delivery, res.Send.Reason, res.Send.Title)
}

// RenderRun prints where the report is, or why there is none.
func RenderRun(w io.Writer, res *RunResult) {
	if res.Run == nil {
		return
	}
	if res.Run.Complete {
		fmt.Fprintf(w, "report: %s\n", res.Run.ReportPath)
		return
	}
	fmt.Fprintf(w, "no report: %s\n", res.Run.Reason)
}
