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
		role := p.Role
		if role == "" {
			role = "-"
		}
		fmt.Fprintf(w, "%-12s %-24s %-8s %-8s send=%-6s %s\n", role, nonEmpty(p.Name, "(no name)"), p.Session6, p.Alive, p.Send, strings.Join(nonEmptyList(p.Task, p.Branch, p.Reason), " "))
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
