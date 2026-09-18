//go:build unix

package handoffcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/agentrun"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// Delivery outcomes.
const (
	DeliveryProven   = "proven"   // Orca observed the agent start its turn
	DeliveryUnproven = "unproven" // the input was accepted; no turn start was observed
	DeliveryFailed   = "failed"   // the one send attempt failed; never retried
	DeliverySkipped  = "skipped"  // nothing was typed
)

// newCodexCommand starts the Codex TUI read-only, like the headless path.
const newCodexCommand = "codex --sandbox read-only"

// screenLines caps a read-back.
const screenLines = 40

// Seams (tests replace them).
var (
	sendLockDir = func() string {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".lets", "locks")
	}
	sendLockWait = 15 * time.Second
	clock        = time.Now
	fingerprint  = agentrun.Fingerprint
)

// SendOptions configures Send. Exactly one of Terminal and New.
type SendOptions struct {
	Root, Brief string
	Self        string // this session's own terminal: never a target
	Terminal    string // an existing agent terminal's handle (from Targets)
	New         string // "codex": open a Codex tab in Root and send there
}

// Send types the one-line pointer to the brief into an agent terminal of this
// checkout, once. It refuses only on evidence - an agent Orca or the screen shows
// busy, or text already typed (gate) - and types into an agent whose input line it
// cannot read, saying so; it never clears a line, never types into a shell and
// never resends after a delivery attempt. A stale handle, which Orca rejects before
// typing, is re-joined by pane key and sent to once. A receipt without
// turn_started is "unproven": the screen is read back for the caller. The lock
// serializes handoff senders only - peer sends keep their own until lets-cbmg7
// shares one.
func Send(ctx context.Context, o SendOptions) (*SendResult, error) {
	res := &SendResult{Envelope: newEnvelope("send")}
	if err := CheckBrief(o.Root, o.Brief); err != nil {
		return res, briefInvalid(&res.Envelope, err)
	}
	if (o.Terminal == "") == (o.New == "") || (o.New != "" && o.New != "codex") {
		return res, usage(&res.Envelope, "pass exactly one of --terminal <handle> and --new codex")
	}
	if o.Terminal != "" && !orcacmd.ValidHandle(o.Terminal) {
		return res, usage(&res.Envelope, "--terminal is not an Orca terminal handle")
	}
	info := &SendInfo{Delivery: DeliverySkipped}
	res.Send, res.OK = info, true
	ops, f := newOps(ctx)
	if f != nil {
		info.Reason = f.Reason
		return res, nil
	}
	fresh := o.New == "codex"
	target := orcacmd.Terminal{Handle: o.Terminal}
	if fresh {
		title := "handoff " + filepath.Base(OutBase(o.Brief))
		h, f := ops.CreateTerminal(ctx, o.Root, title, newCodexCommand)
		if f != nil {
			info.Reason = f.Reason
			return res, nil
		}
		info.Created = true
		h, up, f := ops.WaitStartup(ctx, h, o.Root, title)
		target = orcacmd.Terminal{Handle: h, Title: title, AgentType: "codex"}
		info.Handle, info.Title, info.Agent = h, title, "codex"
		if f != nil || !up {
			info.Reason = "startup_not_idle"
			return res, nil
		}
		// Learn the new tab's pane key, so a stale handle can be re-joined.
		if terms, f := ops.Terminals(ctx); f == nil {
			for _, t := range terms {
				if t.Handle == h {
					target.PaneKey = t.PaneKey
				}
			}
		}
	}
	unlock, reason := lockTarget(target.Handle)
	if reason != "" {
		info.Reason = reason
		return res, nil
	}
	defer unlock()
	if !fresh {
		terms, f := ops.Terminals(ctx)
		if f != nil {
			info.Reason = f.Reason
			return res, nil
		}
		found := false
		for _, t := range terms {
			if t.Handle == o.Terminal {
				target, found = t, true
			}
		}
		if !found {
			info.Reason = "terminal_not_found"
			return res, nil
		}
		info.Handle, info.Title, info.Agent = target.Handle, target.Title, target.AgentType
		if why := ineligible(target, o.Root, o.Self); why != "" {
			info.Reason = why
			return res, nil
		}
	}
	if why := gate(ctx, ops, target, info); why != "" {
		info.Reason = why
		return res, nil
	}
	info.Fingerprint = fingerprint(o.Root)
	info.SentAt = clock().UTC().Format(time.RFC3339Nano)
	rcpt, f := ops.SendText(ctx, target.Handle, Pointer(o.Brief))
	if f != nil && f.Reason == orcacmd.ReasonHandleStale {
		// Orca rejected the handle before typing anything: re-join the same pane and
		// send once to its replacement - never to both.
		next, why := rejoin(ctx, ops, target)
		if why == "" && fresh && next.AgentType == "" {
			next.AgentType = "codex" // this tab was opened with codex; Orca may not have named it yet
		}
		if why == "" {
			why = ineligible(next, o.Root, o.Self)
		}
		if why != "" {
			info.Delivery, info.Reason = DeliveryFailed, why
			return res, nil
		}
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "stale handle " + target.Handle + ": re-joined the same pane as " + next.Handle})
		target, info.Handle = next, next.Handle
		if why := gate(ctx, ops, target, info); why != "" {
			info.Reason = why
			return res, nil
		}
		info.SentAt = clock().UTC().Format(time.RFC3339Nano)
		rcpt, f = ops.SendText(ctx, target.Handle, Pointer(o.Brief))
	}
	switch {
	case f != nil:
		info.Delivery, info.Reason = DeliveryFailed, f.Reason // never resent
	case rcpt.TurnStarted:
		info.Delivery = DeliveryProven
	case rcpt.InputAccepted:
		info.Delivery, info.Reason = DeliveryUnproven, "delivery_unconfirmed"
		info.ScreenTail = readBack(ctx, ops, target.Handle, res)
	default:
		info.Delivery, info.Reason = DeliveryFailed, "input_not_accepted"
	}
	return res, nil
}

// gate reads the target and returns why nothing may be typed ("" to go on). Only
// positive evidence refuses: Orca's agent state; a draft Orca holds for the input
// line (any agent - in a Claude pane typed text never reaches the screen, and a
// send would submit draft and pointer together); or a known recognizer seeing the
// agent busy or text already typed. An input line LETS cannot read (no recognizer,
// no frame) goes on and is recorded as unknown.
func gate(ctx context.Context, ops orcaOps, t orcacmd.Terminal, info *SendInfo) string {
	if t.State == "working" || t.State == "waiting" {
		info.InputLine = lineBusy
		return "target_busy"
	}
	info.InputLine = lineUnknown
	fr, f := ops.Screen(ctx, t.Handle)
	switch {
	case f != nil:
	case strings.TrimSpace(fr.Draft) != "":
		info.InputLine = lineNotClear
	case fr.Source == "screen":
		info.InputLine = inputLine(t.AgentType, fr.Lines)
	}
	switch info.InputLine {
	case lineBusy:
		return "target_busy"
	case lineNotClear:
		return "input_line_not_clear" // the owner clears it; LETS never does
	}
	return ""
}

// rejoin finds the terminal that now holds the same pane (tabId:leafId) - never by
// title - after Orca called its handle stale.
func rejoin(ctx context.Context, ops orcaOps, old orcacmd.Terminal) (orcacmd.Terminal, string) {
	if old.PaneKey == "" {
		return old, orcacmd.ReasonHandleStale
	}
	terms, f := ops.Terminals(ctx)
	if f != nil {
		return old, f.Reason
	}
	for _, t := range terms {
		if t.PaneKey == old.PaneKey && t.Handle != old.Handle {
			return t, ""
		}
	}
	return old, orcacmd.ReasonHandleStale
}

// lockTarget holds a machine-wide per-terminal lock from the readiness check through
// the send: two handoff senders typing into one terminal interleave their text. It
// is keyed by the first handle and held across a stale-handle re-join.
func lockTarget(handle string) (func(), string) {
	if err := os.MkdirAll(sendLockDir(), 0o700); err != nil {
		return nil, "send_lock_failed"
	}
	lf, err := os.OpenFile(filepath.Join(sendLockDir(), "handoff-send-"+handle+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, "send_lock_failed"
	}
	if err := fsutil.TryLockFile(lf, time.Now().Add(sendLockWait)); err != nil {
		_ = lf.Close()
		if errors.Is(err, fsutil.ErrLockBusy) {
			return nil, "another_send_in_progress"
		}
		return nil, "send_lock_failed"
	}
	return func() { _ = fsutil.UnlockFile(lf); _ = lf.Close() }, ""
}

// readBack returns the terminal's current frame for the caller to show as untrusted
// data: URL credentials and secrets redacted over the whole text (a secret can span
// lines), control bytes replaced, the last 40 lines kept. Only source=screen is a
// frame; its lines are bounded by the terminal width.
func readBack(ctx context.Context, ops orcaOps, handle string, res *SendResult) []string {
	fr, f := ops.Screen(ctx, handle)
	if f != nil || fr.Source != "screen" {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "read-back unavailable (source=" + redact.Control(fr.Source) + ")"})
		return nil
	}
	lines := strings.Split(redact.Text(redact.Creds(strings.Join(fr.Lines, "\n"))), "\n")
	if len(lines) > screenLines {
		lines = lines[len(lines)-screenLines:]
	}
	for i, l := range lines {
		lines[i] = redact.Control(l)
	}
	return lines
}
