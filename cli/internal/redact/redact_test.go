package redact

import (
	"strings"
	"testing"
)

func TestText(t *testing.T) {
	cases := map[string]string{
		"token ghp_abcdefghijklmnopqrstuvwxyz0123":                             "[redacted:token]",
		"pat github_pat_11ABCDEFG0123456789_abcdefghij":                        "[redacted:token]",
		"key sk-ant-api03-abcdefghijklmnopqrstuvwx":                            "[redacted:token]",
		"key sk-abcdefghijklmnopqrstuvwx":                                      "[redacted:token]",
		"slack xoxb-1234567890-abcdef":                                         "[redacted:token]",
		"aws AKIAABCDEFGHIJKLMNOP":                                             "[redacted:token]",
		"Authorization: Bearer abc.def.ghi":                                    "Authorization: Bearer [redacted]",
		"DB_PASSWORD=hunter2 next":                                             "DB_PASSWORD=[redacted] next",
		"api_key: 12345":                                                       "api_key=[redacted]",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----": "[redacted:private-key]",
	}
	for in, want := range cases {
		if got := Text(in); !strings.Contains(got, want) {
			t.Errorf("Text(%q) = %q, want it to contain %q", in, got, want)
		}
	}
	for _, benign := range []string{"all tests passed", "the token budget is fine", "see sk-short"} {
		if got := Text(benign); got != benign {
			t.Errorf("benign %q changed to %q", benign, got)
		}
	}
}

func TestCredsAndControl(t *testing.T) {
	if got := Creds("fatal: https://user:pass@github.com/x.git"); got != "fatal: https://<redacted>@github.com/x.git" {
		t.Errorf("Creds = %q", got)
	}
	if got := Control("a\x1b[31mred\x07\tok\nline\u009b"); got != "a [31mred \tok\nline " {
		t.Errorf("Control = %q", got)
	}
}
