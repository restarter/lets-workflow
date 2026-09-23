//go:build unix

package peerscmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

const handoffMax = 8 << 10

// FrameOptions configures Frame.
type FrameOptions struct {
	Cwd       string
	Session   string // the caller
	ToSession string
	Kind      string
	ToName    string // ask-ro: the display name of an orchestrator that is not running (from who --repo)
}

// newMsgID is 16 hex chars from crypto/rand.
var newMsgID = func() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// peerMsgDir creates <root>/.lets/cache/peer-msg through os.OpenRoot and accepts it
// only as a real directory with mode 0700 (a planted symlink is refused).
func peerMsgDir(root string) (string, error) {
	cache := filepath.Join(root, ".lets", "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	r, err := os.OpenRoot(cache)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	if err := r.Mkdir("peer-msg", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	fi, err := r.Lstat("peer-msg")
	if err != nil {
		return "", err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s/peer-msg is not a real directory", cache)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := r.Chmod("peer-msg", 0o700); err != nil {
			return "", err
		}
	}
	return filepath.Join(cache, "peer-msg"), nil
}

func findPeer(rc *repoContext, ctx context.Context, sid string) *Peer {
	for _, p := range rc.peers(ctx) {
		if p.Session == sid {
			pp := p
			return &pp
		}
	}
	return nil
}

// Frame issues a msgid, the header that must open the message, and the handoff file
// path the skill writes the whole message to (the Write tool; never a shell).
func Frame(ctx context.Context, o FrameOptions) (*FrameResult, error) {
	res := &FrameResult{Envelope: newEnvelope("frame")}
	fail := func(e *Error) (*FrameResult, error) {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if !ccregistry.ValidSession(o.Session) || !ccregistry.ValidSession(o.ToSession) || !ValidKind(o.Kind) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "frame needs --session and --to-session (session ids) and --kind ask|ping|tell|ask-ro"})
	}
	if o.Session == o.ToSession {
		return fail(&Error{Code: ExitUsage, Kind: "self_send", Message: "a session cannot send to itself", Remediation: "address another session, or say it in this chat"})
	}
	rc, err := loadRepo(ctx, o.Cwd, false)
	if err != nil {
		return fail(err.(*Error))
	}
	res.Degraded = append(rc.degraded, HealSelf(rc, o.Session, rc.root, branchOf(ctx, o.Cwd))...)
	toName := ""
	if to, ok := rc.snap.Find(o.ToSession); ok && to.NameOK {
		toName = to.Name
	} else if o.Kind == "ask-ro" && ccregistry.ValidName(o.ToName) {
		toName = o.ToName // ask-ro targets a session that is NOT running
	} else {
		return fail(&Error{Code: ExitGeneric, Kind: "peer_not_found", Message: "no live, named session " + session6(o.ToSession)})
	}
	// The sender's name is a label on a claim (from_sid is a claim too); only the
	// target has to resolve. A session with no name - or none the registry shows -
	// still sends, labelled by its session6.
	label := session6(o.Session)
	if from, ok := rc.snap.Find(o.Session); ok && from.NameOK {
		label = from.Name
	}
	role := "peer"
	if f, ok := rc.roles[o.Session]; ok {
		role = f.Role
	}
	id, err := newMsgID()
	if err != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "msgid_failed", Message: err.Error()})
	}
	dir, err := peerMsgDir(rc.root)
	if err != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "handoff_dir_refused", Message: err.Error()})
	}
	pruneHandoffs(dir) // a handoff kept for retry (Tell never deletes an untyped one) must not accumulate
	res.MsgID = id
	res.SentAt = time.Now().UTC().Format(time.RFC3339Nano)
	res.Header = fmt.Sprintf(`[lets-peer id=%s kind=%s from_sid=%s to_sid=%s from="%s/%s" to="%s"]`, id, o.Kind, o.Session, o.ToSession, role, label, toName)
	res.HandoffPath = filepath.Join(dir, id+".txt")
	if err := os.WriteFile(filepath.Join(dir, id+".issued"), []byte(res.Header), 0o600); err != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "handoff_dir_refused", Message: err.Error()})
	}
	res.OK = true
	return res, nil
}

// readHandoff opens <peer-msg>/<msgid>.txt without following a symlink and requires
// a regular file of at most 8 KiB that starts with the header Frame issued. It never
// deletes: the caller decides, via consumeHandoff, once it knows whether anything was
// typed.
func readHandoff(root, msgid string) (string, *Error) {
	if !msgIDRe.MatchString(msgid) {
		return "", &Error{Code: ExitUsage, Kind: "usage", Message: "--msgid is not a msgid"}
	}
	dir, err := peerMsgDir(root)
	if err != nil {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: err.Error()}
	}
	issued, err := os.ReadFile(filepath.Join(dir, msgid+".issued"))
	if err != nil {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "no header was issued for this msgid"}
	}
	path := filepath.Join(dir, msgid+".txt")
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "handoff unreadable or a symlink"}
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > handoffMax {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "handoff is not a regular file of at most 8 KiB"}
	}
	b, err := io.ReadAll(io.LimitReader(f, handoffMax+1))
	if err != nil || len(b) > handoffMax {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "handoff over 8 KiB"}
	}
	text := string(b)
	if !strings.HasPrefix(text, string(issued)) {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "handoff does not start with the issued header"}
	}
	return text, nil
}

// retryHint is the Note printed on a kept (never-typed) handoff, pointing at the
// SAME msgid. It carries --repo-index / --repo when the target was looked up
// outside this checkout (the hub) - the retry must land in the same repo the
// target was resolved in, or it fails peer_unreachable against the wrong one.
func retryHint(toSession, msgid string, repoIndex *int, repo string) string {
	hint := "handoff kept - retry with the same msgid: lets peers tell --to-session " + toSession + " --msgid " + msgid
	switch {
	case repoIndex != nil:
		hint += fmt.Sprintf(" --repo-index %d", *repoIndex)
	case repo != "":
		hint += " --repo '" + repo + "'" // a path is data, never interpolated unquoted
	}
	return hint
}

// consumeHandoff removes a handoff and its issued header. Called only once the
// message was delivered, handed to the skill, or typed into the peer: a message
// that was never typed keeps its file so the SAME msgid can be retried.
func consumeHandoff(root, msgid string) {
	dir, err := peerMsgDir(root)
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(dir, msgid+".txt"))
	_ = os.Remove(filepath.Join(dir, msgid+".issued"))
}

// handoffMaxAge is how long a kept handoff (nothing was ever typed) survives before
// Frame prunes it: a refused or abandoned send must not accumulate files forever.
const handoffMaxAge = 24 * time.Hour

// pruneHandoffs removes *.txt / *.issued older than handoffMaxAge, ignoring errors -
// best-effort housekeeping, never a reason to fail a frame.
func pruneHandoffs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-handoffMaxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") && !strings.HasSuffix(name, ".issued") {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// TellOptions configures Tell.
type TellOptions struct {
	Cwd       string // this checkout: holds the handoff
	ToSession string
	MsgID     string
	ProbeOrca bool
	Repo      string // the target's project, when it is not this one (the hub), or
	RepoIndex *int   // an index from `lets orca repos` (nil = unset)
}

// Tell delivers a framed message. Go sends only over Orca, and only when the target
// is send-safe; a Claude-routed peer gets the framed text back for the skill's
// SendMessage, so Go never sends there.
func Tell(ctx context.Context, o TellOptions) (*TellResult, error) {
	res := &TellResult{Envelope: newEnvelope("tell"), Route: "none"}
	fail := func(e *Error) (*TellResult, error) {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if !ccregistry.ValidSession(o.ToSession) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--to-session is not a session id"})
	}
	rc, err := loadRepo(ctx, o.Cwd, o.ProbeOrca)
	if err != nil {
		return fail(err.(*Error))
	}
	// The handoff stays in this checkout; the target is looked up in its own repo.
	prc, e := peerRepo(ctx, rc, o.Repo, o.RepoIndex, o.ProbeOrca)
	if e != nil {
		return fail(e)
	}
	res.Degraded = prc.degraded
	text, e := readHandoff(rc.root, o.MsgID)
	if e != nil {
		if e.Kind == "handoff_refused" { // a malformed handoff is not retryable; usage errors touch nothing
			consumeHandoff(rc.root, o.MsgID)
		}
		return fail(e)
	}
	if h, ok := leadingHeader(text); !ok || h.ToSID != o.ToSession || h.ID != o.MsgID {
		consumeHandoff(rc.root, o.MsgID)
		return fail(&Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "the header does not address this session and msgid"})
	}
	peer := findPeer(prc, ctx, o.ToSession)
	res.OK = true
	attempted := false
	switch {
	case peer == nil || peer.Send == "none":
		res.Reason = "peer_unreachable"
		res.State = "not_a_live_peer_of_this_repo"
		if peer != nil {
			res.State = peer.Reason
		}
	case peer.Send == "claude":
		res.Route, res.Reason, res.Text = "claude", "claude_transport_model_send", text
	case peer.Send == "orca":
		res.Route = "orca"
		if path, d := LocateTranscript(ccregistry.HomeDir(), peer.Cwd, peer.Session); d != nil {
			res.Reason, res.State = "peer_not_ready", d.Reason
		} else {
			out := orcaTell(ctx, prc.ops, *peer, path, o.MsgID, text)
			res.Delivered, res.Reason, res.State, res.SentAt, res.Observed = out.Delivered, out.Reason, out.State, out.SentAt, out.Observed
			attempted = out.Attempted
			if out.Reason == "peer_not_ready" && peer.Name != "" {
				n := 0
				for _, e := range prc.snap.Entries {
					if e.NameOK && e.Name == peer.Name {
						n++
					}
				}
				if n == 1 {
					res.ClaudeFallbackAllowed = true
					res.Text = text
				}
			}
			if out.Receipt.InputAccepted || out.Receipt.TurnStarted {
				res.Receipt = &ReceiptInfo{InputAccepted: out.Receipt.InputAccepted, TurnStarted: out.Receipt.TurnStarted}
			}
		}
	}
	// res.Text != "" covers both the claude route and the peer_not_ready fallback: the
	// skill has the text, so the file must not be retried.
	consumed := res.Delivered || res.Text != "" || attempted
	if consumed {
		consumeHandoff(rc.root, o.MsgID)
	} else {
		res.Note = retryHint(o.ToSession, o.MsgID, o.RepoIndex, o.Repo)
	}
	if !res.Delivered && res.Text == "" {
		return res, &Error{Code: ExitNotDelivered, Kind: "not_delivered", Message: nonEmpty(res.Reason, "not delivered"), Remediation: res.Note}
	}
	return res, nil
}

// WaitOptions configures Wait.
type WaitOptions struct {
	Cwd          string
	ToSession    string
	SinceMessage string
	SentAt       string
	TimeoutMs    int
}

// Wait blocks until the target's transcript shows an end of turn after the INBOUND
// record of the message, or the timeout.
func Wait(ctx context.Context, o WaitOptions) (*WaitResult, error) {
	res := &WaitResult{Envelope: newEnvelope("wait")}
	fail := func(e *Error) (*WaitResult, error) {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if !ccregistry.ValidSession(o.ToSession) || !msgIDRe.MatchString(o.SinceMessage) || o.SentAt == "" {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "wait needs --to-session, --since-message and --sent-at"})
	}
	if _, err := time.Parse(time.RFC3339Nano, o.SentAt); err != nil {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--sent-at is not an RFC 3339 time"})
	}
	if o.TimeoutMs <= 0 {
		o.TimeoutMs = 60000
	}
	rc, err := loadRepo(ctx, o.Cwd, false)
	if err != nil {
		return fail(err.(*Error))
	}
	res.Degraded = rc.degraded
	res.OK = true
	e, ok := rc.snap.Find(o.ToSession)
	if !ok {
		res.Reason = "peer_not_found"
		return res, nil
	}
	path, d := LocateTranscript(ccregistry.HomeDir(), e.Cwd, e.SessionID)
	if d != nil {
		res.Reason = "completion_unverifiable"
		res.Degraded = append(res.Degraded, *d)
		return res, nil
	}
	timeout := time.Duration(o.TimeoutMs) * time.Millisecond
	wctx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	res.Satisfied, res.Reason = waitReply(wctx, path, o.SinceMessage, o.SentAt, timeout)
	return res, nil
}
