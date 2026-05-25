//go:build unix

package worktreecmd

import "regexp"

// credURLRE matches https://user:token@host or http://user:token@host
// commonly produced by git's stderr when a remote URL contains creds.
var credURLRE = regexp.MustCompile(`https?://[^/\s:]+:[^@\s]+@`)

// redactCreds replaces inline credentials in git output with ***:*** before
// the output is stored in Error.Message or any JSON envelope field.
func redactCreds(s string) string {
	return credURLRE.ReplaceAllString(s, "https://***:***@")
}
