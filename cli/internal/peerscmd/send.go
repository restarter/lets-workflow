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

// Frame issues a msgid, the header that must open the message, and the handoff file
// path the skill writes the whole message to (the Write tool; never a shell).
// Deprecated: a v0.10.0 skill's first step; the message itself goes through
// SendMessage (Tell hands the text back). No peer lookup: the target is labelled
// with its registry name when the registry shows one.
func Frame(ctx context.Context, o FrameOptions) (*FrameResult, error) {
	res := &FrameResult{Envelope: newEnvelope("frame")}
	res.Deprecated = deprecatedText
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
	root, e := rootOf(o.Cwd)
	if e != nil {
		return fail(e)
	}
	snap := ccregistry.Read(ccregistry.HomeDir())
	toName := session6(o.ToSession)
	if ccregistry.ValidName(o.ToName) {
		toName = o.ToName
	} else if to, ok := snap.Find(o.ToSession); ok && to.NameOK {
		toName = to.Name
	}
	// The sender's name is a label on a claim (from_sid is a claim too).
	label := session6(o.Session)
	if from, ok := snap.Find(o.Session); ok && from.NameOK {
		label = from.Name
	}
	id, err := newMsgID()
	if err != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "msgid_failed", Message: err.Error()})
	}
	dir, err := peerMsgDir(root)
	if err != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "handoff_dir_refused", Message: err.Error()})
	}
	pruneHandoffs(dir) // an abandoned handoff must not accumulate
	res.MsgID = id
	res.SentAt = time.Now().UTC().Format(time.RFC3339Nano)
	res.Header = fmt.Sprintf(`[lets-peer id=%s kind=%s from_sid=%s to_sid=%s from="peer/%s" to="%s"]`, id, o.Kind, o.Session, o.ToSession, label, toName)
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
	if errors.Is(err, os.ErrNotExist) {
		// consumed by an earlier tell, or never issued: never a resend
		return "", &Error{Code: ExitGeneric, Kind: "handoff_missing", Message: "no handoff for this msgid - it was already sent, or never framed; nothing is sent twice"}
	}
	if err != nil {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "the issued header is unreadable"}
	}
	path := filepath.Join(dir, msgid+".txt")
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return "", &Error{Code: ExitGeneric, Kind: "handoff_missing", Message: "the handoff file was never written"}
	}
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
}

// Tell is the deprecated v0.10.0 send step. It never types anything: it reads the
// handoff Frame issued, consumes it, and hands the FULL text (header line included)
// back as route claude / reason claude_transport_model_send - the row a v0.10.0 orc
// skill answers with SendMessage, whose reply read correlates on that header's msgid.
// A second tell on the same msgid finds no handoff (handoff_missing): never a resend.
func Tell(ctx context.Context, o TellOptions) (*TellResult, error) {
	res := &TellResult{Envelope: newEnvelope("tell"), Route: "none"}
	res.Deprecated = deprecatedText
	fail := func(e *Error) (*TellResult, error) {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if !ccregistry.ValidSession(o.ToSession) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--to-session is not a session id"})
	}
	root, e := rootOf(o.Cwd)
	if e != nil {
		return fail(e)
	}
	text, e := readHandoff(root, o.MsgID)
	if e != nil {
		if e.Kind == "handoff_refused" { // a malformed handoff is not retryable; usage errors touch nothing
			consumeHandoff(root, o.MsgID)
		}
		return fail(e)
	}
	consumeHandoff(root, o.MsgID)
	if h, ok := leadingHeader(text); !ok || h.ToSID != o.ToSession || h.ID != o.MsgID {
		return fail(&Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "the header does not address this session and msgid"})
	}
	res.OK = true
	res.Route, res.Reason, res.Text = "claude", "claude_transport_model_send", text
	return res, nil
}

// WaitOptions configures Wait (the flags of a v0.10.0 caller; nothing reads them).
type WaitOptions struct {
	Cwd          string
	ToSession    string
	SinceMessage string
	SentAt       string
	TimeoutMs    int
}

// Wait is the deprecated v0.10.0 completion wait: it answers completion_unverifiable
// at once (the v0.10.0 skill then offers /lets:orc read). A native ask's answer
// arrives as the peer's own cross-session message.
func Wait(_ context.Context, _ WaitOptions) (*WaitResult, error) {
	res := &WaitResult{Envelope: newEnvelope("wait")}
	res.Deprecated = deprecatedText
	res.OK, res.Reason = true, "completion_unverifiable"
	return res, nil
}
