package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// exitCoder is implemented by typed subcommand errors that want a specific
// process exit code (e.g. worktreecmd.Error returns 10..20 for typed
// failure classes so scripts can branch on failure kind without parsing
// prose). Errors that don't implement this fall through to ExitGeneric (1).
type exitCoder interface {
	ExitCode() int
}

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var ec exitCoder
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
}
