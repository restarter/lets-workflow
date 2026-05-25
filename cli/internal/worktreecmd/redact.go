//go:build unix

package worktreecmd

import "regexp"

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
var credURLRE = regexp.MustCompile(`(https?://)[^\s/]*@`)

// redactCreds replaces inline credentials in git output with <redacted>
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
func redactCreds(s string) string {
	return credURLRE.ReplaceAllString(s, "${1}<redacted>@")
}
