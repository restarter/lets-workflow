//go:build unix

package worktreecmd

// Test-only exports. Allows _test.go in package worktreecmd_test to drive
// internal helpers without making them part of the public API.

var PerformRollbackForTesting = rollback

var RedactCredsForTesting = redactCreds
