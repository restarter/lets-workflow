//go:build unix

package orcacmd

// Exit codes for `lets orca`. Orca unavailability is NOT an error (graceful
// fallback, exit 0). Only bad usage and an invalid --repo are hard errors.
const (
	ExitOK          = 0
	ExitGeneric     = 1
	ExitUsage       = 2
	ExitRepoInvalid = 10 // --repo is not a directory or not a main checkout
)
