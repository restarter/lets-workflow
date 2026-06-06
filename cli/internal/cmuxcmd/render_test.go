//go:build unix

package cmuxcmd

import (
	"strings"
	"testing"
)

func TestRenderOpen_Launched(t *testing.T) {
	var b strings.Builder
	RenderOpen(&b, &OpenResult{
		Envelope: Envelope{SchemaVersion: SchemaVersion, OK: true, Subcommand: "open"},
		Launch:   &LaunchInfo{Launched: true, WorkspaceName: "cmux-launcher", Path: "/tmp/wt"},
	})
	out := b.String()
	if !strings.Contains(out, "cmux-launcher") || !strings.Contains(out, "/tmp/wt") {
		t.Fatalf("launched output missing workspace/path: %q", out)
	}
}

func TestRenderOpen_Fallback(t *testing.T) {
	var b strings.Builder
	RenderOpen(&b, &OpenResult{
		Envelope: Envelope{SchemaVersion: SchemaVersion, OK: true, Subcommand: "open"},
		Launch:   &LaunchInfo{Launched: false, Path: "/tmp/wt", Reason: "cmux_not_found", FallbackCommand: "cd '/tmp/wt' && claude"},
	})
	out := b.String()
	if !strings.Contains(out, "cmux_not_found") || !strings.Contains(out, "cd '/tmp/wt' && claude") {
		t.Fatalf("fallback output missing reason/command: %q", out)
	}
}
