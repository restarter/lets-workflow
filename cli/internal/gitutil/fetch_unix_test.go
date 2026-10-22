//go:build unix

package gitutil

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A remote helper that never answers and leaves a descendant holding git's stderr
// (the ssh / remote-helper case; the ext:: transport runs `sh -c "sleep 30"`, which
// inherits it). Only the process-group kill closes that pipe at the deadline: with
// git alone killed, Run would wait the whole WaitDelay (Codex round 1). A fetch that
// finishes is never followed by a group kill - read fetchBounded, there is no kill
// on its success path.
func TestFetchBounded_KillsTheProcessGroup(t *testing.T) {
	_, clone := remoteSetup(t)
	start := time.Now()
	err := fetchBounded(context.Background(), clone, time.Second, "-c", "protocol.ext.allow=always", "fetch", "--no-tags", "ext::sh -c sleep% 30", "main")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d >= time.Second+fetchWaitDelay {
		t.Errorf("returned after %s: a descendant kept stderr open until WaitDelay - the group was not killed", d)
	}
}

// A failing fetch whose stderr carries a credential-bearing URL: the error text -
// which every envelope's stale warning and refusal message embed - is redacted
// (Codex round 2). The ext:: helper prints the URL to git's stderr and fails.
func TestFetchBounded_RedactsCredentials(t *testing.T) {
	_, clone := remoteSetup(t)
	err := fetchBounded(context.Background(), clone, 5*time.Second, "-c", "protocol.ext.allow=always", "fetch", "--no-tags", "ext::sh -c echo% https://u:s3cret@host/x% >&2;% exit% 1", "main")
	if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("err = %v, want the credential redacted", err)
	}
}
