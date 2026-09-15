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

// A key the edge of the input cuts (a terminal screen, a scrolled view) is still a
// key: its visible part is redacted, and the text around it is kept.
func TestText_PartialPrivateKeys(t *testing.T) {
	body1 := "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj"
	body2 := "MzEfYyjiWA4R4/M2bS1GB4t7NXp98C3SC6dVMvDuictGeurT8jNbvJZHtCSuYEvu"
	cases := map[string]string{
		"complete, indented":          "$ cat k.pem\n  -----BEGIN PRIVATE KEY-----\n  " + body1 + "\n  " + body2 + "\n  -----END PRIVATE KEY-----\nok",
		"BEGIN only (END scrolled)":   "$ cat k.pem\n-----BEGIN PRIVATE KEY-----\n" + body1 + "\n" + body2,
		"END only (BEGIN scrolled)":   body1 + "\n" + body2 + "\nNNNN==\n-----END EC PRIVATE KEY-----\n❯",
		"body only (both off-screen)": "⏺ reading\n    " + body1 + "\n    " + body2 + "\n❯",
	}
	for name, in := range cases {
		got := Text(in)
		if strings.Contains(got, body1) || strings.Contains(got, body2) || strings.Contains(got, "NNNN==") || !strings.Contains(got, "[redacted:private-key]") {
			t.Errorf("%s: key material left in %q", name, got)
		}
	}
	if got := Text("$ cat k.pem\n-----BEGIN PRIVATE KEY-----\n" + body1 + "\nnext command"); !strings.HasPrefix(got, "$ cat k.pem\n") || !strings.HasSuffix(got, "\nnext command") {
		t.Errorf("text around a cut key is kept: %q", got)
	}
	for _, benign := range []string{"one line of sixty-four base64 characters is not a key:\n" + body1, "short\nlines\nstay"} {
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

func TestText_MoreTokenShapes(t *testing.T) {
	cases := map[string]string{
		"openai sk-proj-abcdefghijklmnopqrstuvwxyz_123":             "[redacted:token]",
		"stripe sk_live_abcdefghijklmnopqrstuvwx":                   "[redacted:token]",
		"google AIzaSyA1234567890abcdefghijklmnopqrstuv":            "[redacted:token]",
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcDEF123_-x":     "[redacted:jwt]",
		"x-api-key: abc123def456":                                   "x-api-key: [redacted]",
		"deploy key for prod: Zm9vYmFyYmF6cXV4MTIzNDU2Nzg5MGFiY2Rl": "[redacted:high-entropy]",
	}
	for in, want := range cases {
		if got := Text(in); !strings.Contains(got, want) {
			t.Errorf("Text(%q) = %q, want it to contain %q", in, got, want)
		}
	}
	// a long hash on a line with no key-like word is left alone
	commit := "commit 4f1c2a9b8e7d6c5b4a3928171615141312111009 merged"
	if got := Text(commit); got != commit {
		t.Errorf("benign hash changed: %q", got)
	}
}

func TestToolResult(t *testing.T) {
	bash := func(cmd string) map[string]any { return map[string]any{"command": cmd} }
	read := func(p string) map[string]any { return map[string]any{"file_path": p} }
	for _, c := range []struct {
		tool  string
		input map[string]any
	}{
		{"Bash", bash("env")},
		{"Bash", bash("declare -p")},
		{"Bash", bash("cat .env")},
		{"Bash", bash("cat config/.env.local")},
		{"Read", read("/repo/.beads/.env")},
		{"Read", read("/home/u/.ssh/id_ed25519")},
		{"Bash", bash("cat ~/.netrc")},
	} {
		if got := ToolResult(c.tool, c.input, "SECRET=1", 400); got != "[redacted:env-like source]" {
			t.Errorf("%s %v: %q", c.tool, c.input, got)
		}
	}
	dump := "declare -x HOME=/h\ndeclare -x PATH=/bin\ndeclare -x TOKENISH=x\n"
	if got := ToolResult("Bash", bash("some-script"), dump, 400); got != "[redacted:env dump]" {
		t.Errorf("dump: %q", got)
	}
	if got := ToolResult("Bash", bash("echo hi"), "token ghp_abcdefghijklmnopqrstuvwxyz0123", 400); !strings.Contains(got, "[redacted:token]") {
		t.Errorf("token in result: %q", got)
	}
	if got := ToolResult("Bash", bash("ls"), "a\x1b[31mred", 400); strings.ContainsRune(got, 0x1b) {
		t.Errorf("control byte kept: %q", got)
	}
	long := strings.Repeat("ж", 300) // 600 bytes
	got := ToolResult("Bash", bash("cat notes.txt"), long, 401)
	if !strings.Contains(got, "…[truncated 200 bytes]") {
		t.Errorf("cap marker: %q", got[len(got)-40:])
	}
	if cut := got[:strings.Index(got, " …[truncated")]; !utf8ValidAndEven(cut) {
		t.Errorf("cut mid-rune: %d bytes", len(cut))
	}
	if got := ToolResult("Grep", map[string]any{"pattern": "x"}, "fine", 400); got != "fine" {
		t.Errorf("other tool: %q", got)
	}
}

func utf8ValidAndEven(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return len(s)%2 == 0
}
