// Package redact strips secrets and terminal control bytes from text before it
// reaches a JSON envelope, an Orca card, or another session's context.
//
// Leaf package: standard library only.
package redact

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// credURLRE matches the `scheme://[user[:password]]@` prefix of an HTTP(S)
// URL — any scheme://...@ form, regardless of whether the credentials look
// like user:password, :token-only, single-token, or contain @ in the
// password. Anchoring on the trailing @ and stopping at /+whitespace is the
// simplest pattern that covers every cred shape git emits to stderr when a
// remote URL contains auth.
//
// Captured group 1 is the scheme (preserved in the redaction output so the
// user can still locate the offending remote in their git config). The
// non-captured middle ([^\s/]*@) is the part replaced wholesale.
//
// Trade-off (review post-S-1 followup #4): RE2's greedy [^\s/]* stops only
// at `/` or whitespace, so it can over-absorb when the input contains
// stray `@` symbols AFTER the host but before any `/` (e.g.
// `https://u:p@h?q=x@y` matches `https://u:p@h?q=x@` and redacts to
// `https://<redacted>@y`). This is acceptable for git stderr where URLs
// are followed immediately by `/path` or whitespace, but worth knowing if
// this helper is ever reused outside the git-stderr context.
var credURLRE = regexp.MustCompile(`(https?://)[^\s/]*@`)

// Creds replaces inline credentials in git output with <redacted>
// before the output is stored in Error.Message or any JSON envelope field.
// Preserves the original scheme (security review B-2: scheme-flip can
// mislead users debugging transport security).
//
// Covers all four shapes git can emit:
//
//	https://user:password@host    -> https://<redacted>@host
//	https://:token@host           -> https://<redacted>@host  (token-only)
//	https://token@host            -> https://<redacted>@host  (single-token,
//	                                                          gh auth setup-git form)
//	https://user:p@ssword@host    -> https://<redacted>@host  (password contains @)
//
// Leaves bare-SSH URLs (`git@github.com:user/repo`) untouched — no
// scheme:// prefix means no match, which is correct (the user part of SSH
// URLs is not a secret).
func Creds(s string) string {
	return credURLRE.ReplaceAllString(s, "${1}<redacted>@")
}

// Control replaces C0 control bytes (except tab and newline), DEL and C1 controls
// with a space, so terminal escape sequences cannot restyle, hide or rewrite what a
// reader sees.
func Control(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n':
			return r
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return ' '
		}
		return r
	}, s)
}

var textRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), "[redacted:private-key]"},
	{regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-ant-[A-Za-z0-9_-]{20,}|sk-proj-[A-Za-z0-9_-]{20,}|sk-[A-Za-z0-9]{20,}|sk_live_[A-Za-z0-9]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35})`), "[redacted:token]"},
	{regexp.MustCompile(`\beyJ[\w-]+\.[\w-]+\.[\w-]+`), "[redacted:jwt]"},
	{regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)\S+`), "${1}[redacted]"},
	{regexp.MustCompile(`(?im)^([\w-]*(api-?key|token|secret)[\w-]*):[ \t]*\S+`), "${1}: [redacted]"},
	{regexp.MustCompile(`(?i)\b([A-Z0-9_]*(PASSWORD|PASSWD|SECRET|TOKEN|API_?KEY|PRIVATE_KEY)[A-Z0-9_]*)\s*[=:]\s*\S+`), "${1}=[redacted]"},
}

// keyWordLine finds a line naming a key-like word; highEntropy is a long run of
// token characters. Together they catch a secret no shape rule knows.
var (
	keyWordLine = regexp.MustCompile(`(?i)(key|token|secret|passw|credential|auth)`)
	highEntropy = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)
)

// highEntropyOnKeyLine redacts a 32+ character token-shaped run that mixes letters
// and digits, on a line that also names a key-like word.
func highEntropyOnKeyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if !keyWordLine.MatchString(l) {
			continue
		}
		lines[i] = highEntropy.ReplaceAllStringFunc(l, func(m string) string {
			if strings.ContainsAny(m, "0123456789") && strings.IndexFunc(m, func(r rune) bool { return r >= 'A' && r <= 'z' }) >= 0 {
				return "[redacted:high-entropy]"
			}
			return m
		})
	}
	return strings.Join(lines, "\n")
}

// Text redacts private keys, well-known token shapes, bearer headers and
// secret-looking assignments. Pattern-based: callers also cap length, which is the
// real control against a secret no pattern knows.
func Text(s string) string {
	for _, r := range textRules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return highEntropyOnKeyLine(s)
}

var (
	// envSource: a command or path that reads an env-like or credential file.
	envSource = regexp.MustCompile(`(?i)(^|[\s/'"])(\.env(\.[\w-]+)?|[\w.-]*\.pem|id_(rsa|ed25519|ecdsa)|\.netrc|credentials(\.json)?)(\s|$|['"])`)
	// envCommand: a Bash command that prints the whole environment.
	envCommand = regexp.MustCompile(`^\s*(env|printenv|export -p|declare -p|declare -x|set)\s*$`)
	// envLine: one `KEY=` assignment line; three of them make a dump.
	envLine = regexp.MustCompile(`(?m)^\s*(export\s+|declare\s+-x\s+)?[A-Z_][A-Z0-9_]*=`)
)

// ToolResult redacts a tool result another session is about to read. It takes the
// tool's STRUCTURED input (for Bash the `command` field, for Read / Edit / Write the
// `file_path`): a result whose subject is an env-like source, or that looks like an
// environment dump, is withheld whole; anything else goes through Creds, Text and
// Control and is capped at capBytes (cut on a rune boundary, with a marker).
func ToolResult(tool string, input map[string]any, output string, capBytes int) string {
	subject := inputSubject(tool, input)
	if envSource.MatchString(subject) || (tool == "Bash" && envCommand.MatchString(subject)) {
		return "[redacted:env-like source]"
	}
	if len(envLine.FindAllStringIndex(output, 3)) >= 3 {
		return "[redacted:env dump]"
	}
	return Cap(Control(Text(Creds(output))), capBytes)
}

// Cap cuts s to at most capBytes on a rune boundary and says how much it dropped.
func Cap(s string, capBytes int) string {
	if capBytes <= 0 || len(s) <= capBytes {
		return s
	}
	cut := capBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf(" …[truncated %d bytes]", len(s)-cut)
}

func inputSubject(tool string, input map[string]any) string {
	key := ""
	switch tool {
	case "Bash":
		key = "command"
	case "Read", "Edit", "Write", "NotebookEdit":
		key = "file_path"
	}
	if v, ok := input[key].(string); ok && key != "" {
		return v
	}
	b, _ := json.Marshal(input)
	return string(b)
}
