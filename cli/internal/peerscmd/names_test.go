package peerscmd

import "testing"

func TestValidRole(t *testing.T) {
	for role, want := range map[string]bool{"orchestrator": true, "peer": true, "worker": true, "": false, "Orchestrator": false, "admin": false} {
		if got := ValidRole(role); got != want {
			t.Errorf("ValidRole(%q) = %v, want %v", role, got, want)
		}
	}
}
