//go:build unix

package worktreecmd

import "github.com/restarter/lets-workflow/cli/internal/redact"

// redactCreds replaces inline URL credentials in git output (redact.Creds owns the rule).
func redactCreds(s string) string { return redact.Creds(s) }
