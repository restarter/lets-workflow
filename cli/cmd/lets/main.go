package main

import (
	"fmt"
	"os"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
