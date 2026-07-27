//go:build unix

package tmuxcmd

import (
	"strings"
	"testing"
)

func TestRenderOpen_DetachedShowsAttach(t *testing.T) {
	var b strings.Builder
	RenderOpen(&b, &OpenResult{
		Envelope: Envelope{OK: true},
		Launch:   &LaunchInfo{Launched: true, WorkspaceName: "demo", Target: "demo:0", Path: "/tmp", AttachCommand: "tmux attach -t 'demo'"},
	})
	if !strings.Contains(b.String(), "Attach with:") {
		t.Errorf("detached launch must show attach instructions: %q", b.String())
	}
}

func TestRenderOpen_InSessionNoAttach(t *testing.T) {
	var b strings.Builder
	RenderOpen(&b, &OpenResult{
		Envelope: Envelope{OK: true},
		Launch:   &LaunchInfo{Launched: true, Target: "cur:3", Path: "/tmp", InExistingSession: true},
	})
	if strings.Contains(b.String(), "Attach with:") {
		t.Errorf("in-session launch must NOT show attach instructions: %q", b.String())
	}
}

func TestRenderOpen_AlreadyOpen(t *testing.T) {
	var b strings.Builder
	RenderOpen(&b, &OpenResult{
		Envelope: Envelope{OK: true},
		Launch:   &LaunchInfo{Launched: false, Reason: "already_open", ExistingTarget: "w:1", ExistingTitle: "x", Path: "/tmp", FallbackCommand: "cd /tmp && claude"},
	})
	if !strings.Contains(b.String(), "already lives at") {
		t.Errorf("want already-open message: %q", b.String())
	}
}

func TestRenderNotify_NotSent(t *testing.T) {
	var b strings.Builder
	RenderNotify(&b, &NotifyResult{
		Envelope: Envelope{OK: true},
		Notify:   &NotifyInfo{Notified: false, Reason: "no_client"},
	})
	if !strings.Contains(b.String(), "not sent (no_client)") {
		t.Errorf("want not-sent message: %q", b.String())
	}
}

func TestRenderRename_NotRenamed(t *testing.T) {
	var b strings.Builder
	RenderRename(&b, &RenameResult{
		Envelope: Envelope{OK: true},
		Rename:   &RenameInfo{Renamed: false, Reason: "pane_not_found"},
	})
	if !strings.Contains(b.String(), "not renamed (pane_not_found)") {
		t.Errorf("want not-renamed message: %q", b.String())
	}
}
