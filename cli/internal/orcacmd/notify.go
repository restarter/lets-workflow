//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// NotifyOptions configures Notify.
type NotifyOptions struct {
	Title, Body string
	Cwd         string // the worktree to notify; matched against Orca's worktree paths
}

// commentCap bounds a card comment (runes; cut on a rune boundary).
const commentCap = 280

// NotifyFunc is the seam notifycmd calls (tests replace it; an export_test.go would
// be invisible to another package's tests).
var NotifyFunc = Notify

// Notify writes "<title>: <body>" as the Orca workspace card comment for this
// worktree. Never hard-fails except on a missing title.
func Notify(ctx context.Context, o NotifyOptions) (*NotifyResult, error) {
	res := &NotifyResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "notify", Steps: []Step{}}}
	if o.Title == "" {
		res.Error = &ErrorInfo{Kind: "title_missing", Message: "--title is required"}
		return res, &Error{Code: ExitUsage, Kind: "title_missing", Message: "--title is required"}
	}
	info := &NotifyInfo{Title: o.Title}
	res.Notify = info
	res.OK = true
	degrade := func(f *Failure) (*NotifyResult, error) {
		info.Reason = f.Reason
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "orca notify skipped: " + f.Error()})
		return res, nil
	}
	c, f := NewClient()
	if f != nil {
		return degrade(f)
	}
	id, f := c.SelfWorktree(ctx)
	if f != nil {
		rows, pf := c.Ps(ctx)
		if pf != nil {
			return degrade(pf)
		}
		id = ""
		for _, r := range rows {
			if o.Cwd != "" && fsutil.SameDir(r.Path, o.Cwd) {
				id = r.WorktreeID
				break
			}
		}
		if id == "" {
			return degrade(&Failure{Reason: ReasonWorktreeNotFound, Verb: "worktree ps"})
		}
	}
	comment := Truncate(redact.Control(redact.Text(o.Title+": "+o.Body)), commentCap)
	if o.Body == "" {
		comment = Truncate(redact.Control(redact.Text(o.Title)), commentCap)
	}
	if f := c.RunChecked(ctx, "worktree set", "worktree", "set", "--worktree", "id:"+id, "--comment", comment, "--json"); f != nil {
		return degrade(f)
	}
	info.Notified, info.Target = true, id
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("card comment set on %s", id)})
	return res, nil
}

// Truncate cuts s to at most n runes, on a rune boundary.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}
