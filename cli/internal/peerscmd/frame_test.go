//go:build unix

package peerscmd

import (
	"encoding/json"
	"os"
	"testing"
)

// deliveredLine is the delivered-SendMessage fixture: the one user record a
// cross-session SendMessage leaves in the receiver's transcript (lets-rry3c,
// shape recorded live 2026-09-25T12:15:22Z; ids and paths are fixture values).
func deliveredLine(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(fixturePath("delivered-sendmessage.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func deliveredText(t *testing.T) string {
	t.Helper()
	msg, _ := deliveredLine(t)["message"].(map[string]any)
	text, _ := msg["content"].(string)
	if text == "" {
		t.Fatal("fixture has no user text")
	}
	return text
}

const deliveredID = "0a1b2c3d4e5f6071"

func TestLeadingHeader_ClaudeCodeDeliveryPrefix(t *testing.T) {
	h, ok := leadingHeader(deliveredText(t))
	if !ok || h.ID != deliveredID || h.Kind != "ask" || h.FromSID != sidMain || h.ToSID != sidWork || h.From != "worker/W1" || h.To != "MAIN" {
		t.Errorf("the delivered record must parse to its header: %+v ok=%v", h, ok)
	}
}

func TestLeadingHeader_WrapperOnly(t *testing.T) {
	text := "<cross-session-message from=\"uds:/tmp/cc-socks/1.sock\">\n" + header(msg1, sidWork) + "\nq"
	if h, ok := leadingHeader(text); !ok || h.ID != msg1 {
		t.Errorf("the wrapper-only shape must still parse: %+v ok=%v", h, ok)
	}
	if h, ok := leadingHeader(header(msg1, sidWork) + "\nq"); !ok || h.ID != msg1 {
		t.Errorf("a bare header must still parse: %+v ok=%v", h, ok)
	}
}

func TestLeadingHeader_PrefixWithoutWrapperRejected(t *testing.T) {
	text := "Another Claude session sent a message:\n" + header(msg1, sidWork) + "\nq"
	if h, ok := leadingHeader(text); ok {
		t.Errorf("the prefix without the wrapper is not a header: %+v", h)
	}
}

func TestLeadingHeader_ArbitraryPrefixRejected(t *testing.T) {
	for _, text := range []string{
		"hello <cross-session-message>\n" + header(msg1, sidWork),
		"Another Claude session sent a message: extra <cross-session-message>\n" + header(msg1, sidWork),
		"Some session sent a message:\n<cross-session-message>\n" + header(msg1, sidWork),
	} {
		if h, ok := leadingHeader(text); ok {
			t.Errorf("text before the wrapper is not a header: %q -> %+v", text, h)
		}
	}
}
