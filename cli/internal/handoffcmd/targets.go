//go:build unix

package handoffcmd

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// TargetsOptions configures Targets.
type TargetsOptions struct {
	Root  string // this checkout
	Self  string // this session's own terminal ($ORCA_TERMINAL_HANDLE): never a target
	Match string // optional: a term_ handle (exact) or a title fragment
}

// Targets lists the agent terminals a brief can go to, newest output first. Orca
// absent or not running is ok=true, available=false with the reason.
func Targets(ctx context.Context, o TargetsOptions) (*TargetsResult, error) {
	res := &TargetsResult{Envelope: newEnvelope("targets")}
	info := &TargetsInfo{Terminals: []Target{}}
	res.Targets, res.OK = info, true
	ops, f := newOps(ctx)
	if f != nil {
		info.Reason = f.Reason
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "orca unavailable: " + f.Error()})
		return res, nil
	}
	terms, f := ops.Terminals(ctx)
	if f != nil {
		info.Reason = f.Reason
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "terminal list: " + f.Error()})
		return res, nil
	}
	info.Available = true
	for _, t := range terms {
		if ineligible(t, o.Root, o.Self) == "" && (o.Match == "" || matches(t, o.Match)) {
			info.Terminals = append(info.Terminals, Target{Handle: t.Handle, Title: t.Title, Agent: t.AgentType, State: t.State, LastOutputAt: t.LastOutputAt})
		}
	}
	sort.SliceStable(info.Terminals, func(i, j int) bool { return info.Terminals[i].LastOutputAt > info.Terminals[j].LastOutputAt })
	return res, nil
}

// ineligible names why a terminal is not a target ("" when it is): this session's
// own, another checkout, not writable or disconnected, or no agent in it - a shell
// cannot act on a brief, and the pointer must never be typed into one.
func ineligible(t orcacmd.Terminal, root, self string) string {
	switch {
	case t.Handle == self:
		return "terminal_is_self"
	case !fsutil.SameDir(t.Path, root):
		return "terminal_other_checkout"
	case !t.Writable || !t.Connected:
		return "terminal_not_writable"
	case t.AgentType == "":
		return "not_an_agent"
	}
	return ""
}

// matches: a term_ handle matches exactly; anything else is a case-insensitive
// fragment of the title with Orca's leading status glyphs ("✳ ", "◑ ") ignored.
func matches(t orcacmd.Terminal, q string) bool {
	if strings.HasPrefix(q, "term_") {
		return t.Handle == q
	}
	return strings.Contains(normTitle(t.Title), normTitle(q))
}

func normTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimLeftFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
