//go:build unix

package orcacmd

import (
	"bytes"
	"strings"
	"testing"
)

// A fallback that drops an explicit --base-branch must say so in the human
// output too, not only in --json (Codex round 1, lets-l9st6).
func TestRenderOpen_ShowsWarnings(t *testing.T) {
	var b bytes.Buffer
	RenderOpen(&b, &OpenResult{
		Envelope: Envelope{OK: true, Steps: []Step{
			{Status: StepOK, Message: "an ok step"},
			{Status: StepWarn, Message: "--base-branch origin/dev is not carried by the fallback"},
		}},
		Launch: &LaunchInfo{WorkspaceName: "n", Reason: "orca_app_not_running", FallbackCommand: "/lets:worktree create n --no-orca"},
	})
	out := b.String()
	if !strings.Contains(out, "[warn] --base-branch origin/dev is not carried by the fallback\n") {
		t.Errorf("warning missing from human output:\n%s", out)
	}
	if strings.Contains(out, "an ok step") {
		t.Errorf("only warnings are rendered, got:\n%s", out)
	}
}
