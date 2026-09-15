// Package redact strips secrets and terminal control bytes from text before it
// reaches a JSON envelope, an Orca card, or another session's context.
//
// Leaf package: standard library only.
package redact

import (
	"regexp"
	"strings"
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
	{regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-ant-[A-Za-z0-9_-]{20,}|sk-[A-Za-z0-9]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16})\b`), "[redacted:token]"},
	{regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)\S+`), "${1}[redacted]"},
	{regexp.MustCompile(`(?i)\b([A-Z0-9_]*(PASSWORD|PASSWD|SECRET|TOKEN|API_?KEY|PRIVATE_KEY)[A-Z0-9_]*)\s*[=:]\s*\S+`), "${1}=[redacted]"},
}

// Text redacts private keys, well-known token shapes, bearer headers and
// secret-looking assignments. Pattern-based: callers also cap length, which is the
// real control against a secret no pattern knows.
func Text(s string) string {
	for _, r := range textRules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}
