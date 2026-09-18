//go:build unix

package orcacmd

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// Terminal is one live Orca terminal joined to its `worktree ps` agent row by pane
// key (`<tabId>:<leafId>`). AgentType prefers the ps row and falls back to the
// terminal's agentIdentity; State is empty when Orca tracks no agent in the pane.
// PaneKey outlives a handle: a stale handle is re-joined by it, never by the title.
type Terminal struct {
	Handle, Path, Title, AgentType, State, PaneKey string
	LastOutputAt                                   int64 // epoch ms
	Writable, Connected                            bool
}

// Frame is one `terminal read --screen` result. Only Source "screen" is a rendered
// frame; "stream" and "screen-unavailable" are accumulated output. Draft is text
// Orca holds for the input line but has not submitted - for a Claude pane typed text
// lives there and never reaches the screen (2026-09-18) - and "" when there is none.
type Frame struct {
	Source string
	Lines  []string
	Draft  string
}

// startupWait bounds a startup wait; Run's 15 s default would cut it short.
const startupWait = 120 * time.Second

// Terminals lists live Orca terminals. peerscmd keeps its own copy of this join
// until lets-cbmg7 moves the peers path onto this method.
func (c *Client) Terminals(ctx context.Context) ([]Terminal, *Failure) {
	out, f := c.Run(ctx, "terminal list", "terminal", "list", "--json")
	if f != nil {
		return nil, f
	}
	var env struct {
		Result *struct {
			Terminals []struct {
				Handle        string `json:"handle"`
				WorktreePath  string `json:"worktreePath"`
				Title         string `json:"title"`
				TabID         string `json:"tabId"`
				LeafID        string `json:"leafId"`
				AgentIdentity string `json:"agentIdentity"`
				LastOutputAt  int64  `json:"lastOutputAt"`
				Writable      bool   `json:"writable"`
				Connected     bool   `json:"connected"`
			} `json:"terminals"`
		} `json:"result"`
	}
	if f := DecodeJSON("terminal list", out, &env); f != nil {
		return nil, f
	}
	if env.Result == nil {
		return nil, &Failure{Reason: ReasonOutputUnrecognized, Verb: "terminal list"}
	}
	rows, f := c.Ps(ctx)
	if f != nil {
		return nil, f
	}
	state := map[string]PsAgent{}
	for _, r := range rows {
		for _, a := range r.Agents {
			state[a.PaneKey] = a
		}
	}
	var terms []Terminal
	for _, t := range env.Result.Terminals {
		if !ValidHandle(t.Handle) {
			continue
		}
		key := t.TabID + ":" + t.LeafID
		a := state[key]
		agent := a.AgentType
		if agent == "" {
			agent = t.AgentIdentity
		}
		terms = append(terms, Terminal{
			Handle: t.Handle, Path: t.WorktreePath, Title: redact.Control(t.Title),
			AgentType: agent, State: a.State, PaneKey: key, LastOutputAt: t.LastOutputAt,
			Writable: t.Writable, Connected: t.Connected,
		})
	}
	return terms, nil
}

// Screen reads a terminal's rendered frame and the input-line draft Orca holds.
func (c *Client) Screen(ctx context.Context, handle string) (Frame, *Failure) {
	out, f := c.Run(ctx, "terminal read", "terminal", "read", "--terminal", handle, "--screen", "--json")
	if f != nil {
		return Frame{}, f
	}
	var env struct {
		Result *struct {
			Terminal *struct {
				Source string   `json:"source"`
				Tail   []string `json:"tail"`
				Draft  string   `json:"draft"`
			} `json:"terminal"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &env) != nil || env.Result == nil || env.Result.Terminal == nil {
		return Frame{}, &Failure{Reason: ReasonOutputUnrecognized, Verb: "terminal read"}
	}
	t := env.Result.Terminal
	return Frame{Source: t.Source, Lines: t.Tail, Draft: t.Draft}, nil
}

// SendText types one line plus Enter and observes the submission for up to 10 s.
// It never resends: a timed-out observation returns the receipt it has.
func (c *Client) SendText(ctx context.Context, handle, text string) (Receipt, *Failure) {
	out, f := c.Run(ctx, "terminal send", "terminal", "send", "--terminal", handle, "--text", text, "--enter", "--wait-submit", "10", "--json")
	if f != nil {
		return Receipt{}, f
	}
	return ParseReceipt(out)
}

// CreateTerminal opens a terminal in the checkout at path running command and
// returns its handle. When Orca echoes the worktree id (`<repoId>::<path>`), its
// path part must be path: a selector that resolved elsewhere would put the agent in
// another checkout.
func (c *Client) CreateTerminal(ctx context.Context, path, title, command string) (string, *Failure) {
	out, f := c.Run(ctx, "terminal create", "terminal", "create", "--worktree", "path:"+path, "--title", title, "--command", command, "--json")
	if f != nil {
		return "", f
	}
	var env struct {
		Result *struct {
			Terminal *struct {
				Handle     string `json:"handle"`
				WorktreeID string `json:"worktreeId"`
			} `json:"terminal"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &env) != nil || env.Result == nil || env.Result.Terminal == nil || !ValidHandle(env.Result.Terminal.Handle) {
		return "", &Failure{Reason: ReasonOutputUnrecognized, Verb: "terminal create"}
	}
	if id := env.Result.Terminal.WorktreeID; id != "" {
		if i := strings.LastIndex(id, "::"); i < 0 || !fsutil.SameDir(id[i+2:], path) {
			return "", &Failure{Reason: ReasonEnvMismatch, Verb: "terminal create", Detail: detail("created in " + id)}
		}
	}
	return env.Result.Terminal.Handle, nil
}

// WaitStartup waits once for a new terminal's TUI to go idle - a startup signal
// only: tui-idle also fires mid-turn, so no reply is ever judged by it. A stale
// handle is re-joined once by the terminal's title in path (callers give new
// terminals a unique title). It returns the handle in use and whether the wait was
// satisfied (an agent held at an update or trust prompt is not).
func (c *Client) WaitStartup(ctx context.Context, handle, path, title string) (string, bool, *Failure) {
	wctx, cancel := context.WithTimeout(ctx, startupWait+5*time.Second)
	defer cancel()
	satisfied := false
	relist := func() (string, error) {
		out, f := c.Run(wctx, "terminal list", "terminal", "list", "--worktree", "path:"+path, "--json")
		if f != nil {
			return "", f
		}
		var l struct {
			Result *struct {
				Terminals []struct {
					Handle string `json:"handle"`
					Title  string `json:"title"`
				} `json:"terminals"`
			} `json:"result"`
		}
		if json.Unmarshal(out, &l) != nil || l.Result == nil {
			return "", &Failure{Reason: ReasonOutputUnrecognized, Verb: "terminal list"}
		}
		found := ""
		for _, term := range l.Result.Terminals {
			if term.Title == title && ValidHandle(term.Handle) {
				if found != "" {
					return "", &Failure{Reason: ReasonHandleStale, Verb: "terminal list", Detail: "several terminals carry this title"}
				}
				found = term.Handle
			}
		}
		if found == "" {
			return "", &Failure{Reason: ReasonHandleStale, Verb: "terminal list"}
		}
		handle = found
		return found, nil
	}
	f := WithHandleRetry(wctx, handle, relist, func(h string) *Failure {
		out, f := c.Run(wctx, "terminal wait", "terminal", "wait", "--terminal", h, "--for", "tui-idle", "--timeout-ms", "120000", "--json")
		if f != nil {
			return f
		}
		var w struct {
			Result *struct {
				Wait *struct {
					Satisfied bool `json:"satisfied"`
				} `json:"wait"`
			} `json:"result"`
		}
		if json.Unmarshal(out, &w) == nil && w.Result != nil && w.Result.Wait != nil {
			satisfied = w.Result.Wait.Satisfied
		}
		return nil
	})
	return handle, satisfied, f
}
