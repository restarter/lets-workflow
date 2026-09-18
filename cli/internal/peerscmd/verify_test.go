//go:build unix

package peerscmd

import (
	"encoding/json"
	"testing"
)

// TestResult_SchemaContract pins the envelope keys the orc skill and orient parse.
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; a bump breaks skills/orc and orient - update them and cli/README.md first", SchemaVersion)
	}
	cases := map[string]struct {
		v    any
		keys []string
	}{
		"who":          {&WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{{Alive: "alive", Via: []string{"claude"}, Send: "claude"}}}, []string{"schema_version", "ok", "subcommand", "degraded", "peers"}},
		"tail":         {&TailResult{Envelope: newEnvelope("tail"), Turns: []Turn{}}, []string{"turns", "degraded"}},
		"frame":        {&FrameResult{Envelope: newEnvelope("frame")}, []string{"header", "msgid", "sent_at", "handoff_path"}},
		"tell":         {&TellResult{Envelope: newEnvelope("tell")}, []string{"delivered", "route", "observed"}},
		"wait":         {&WaitResult{Envelope: newEnvelope("wait")}, []string{"satisfied"}},
		"ask-ro":       {&AskROResult{Envelope: newEnvelope("ask-ro")}, []string{"answered", "degraded"}},
		"role":         {&RoleResult{Envelope: newEnvelope("role"), Role: &RoleInfo{}}, []string{"role"}},
		"orchestrator": {&OrchestratorResult{Envelope: newEnvelope("orchestrator"), Candidates: []Candidate{}}, []string{"source", "scope", "candidates"}},
	}
	for name, c := range cases {
		b, _ := json.Marshal(c.v)
		var got map[string]any
		_ = json.Unmarshal(b, &got)
		for _, k := range c.keys {
			if _, ok := got[k]; !ok {
				t.Errorf("%s: missing key %q in %s", name, k, b)
			}
		}
	}
	b, _ := json.Marshal(Peer{Alive: "unknown", Via: []string{}, Send: "none"})
	var p map[string]any
	_ = json.Unmarshal(b, &p)
	for _, k := range []string{"alive", "via", "send"} {
		if _, ok := p[k]; !ok {
			t.Errorf("peer missing %q", k)
		}
	}
}
