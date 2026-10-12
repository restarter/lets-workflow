//go:build unix

package peerscmd

import (
	"fmt"
	"io"
	"strings"
)

// RenderWho prints the peer table for a human.
func RenderWho(w io.Writer, res *WhoResult) {
	if len(res.Peers) == 0 {
		fmt.Fprintln(w, "no live peers in this repo")
	}
	for _, p := range res.Peers {
		kind := "-"
		if p.Orchestrator {
			kind = "orchestrator"
		}
		var marks []string
		if p.Scope != "" {
			marks = append(marks, "scope="+p.Scope)
		}
		if p.Standing {
			marks = append(marks, "standing")
		}
		if p.Orc != "" {
			marks = append(marks, "orc="+p.Orc)
		}
		if p.Duplicate {
			marks = append(marks, "duplicate_name")
		}
		fmt.Fprintf(w, "%-12s %-24s %-8s %-8s %s\n", kind, nonEmpty(p.Name, "(no name)"), p.Session6, p.Alive, strings.Join(nonEmptyList(append([]string{p.Task, p.Branch}, marks...)...), " "))
	}
	for _, d := range res.Degraded {
		fmt.Fprintf(w, "degraded %s: %s %s\n", d.Source, d.Reason, d.Detail)
	}
}

func nonEmptyList(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
