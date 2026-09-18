//go:build unix

package handoffcmd

import (
	"context"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// orcaOps is the Orca surface handoff uses; *orcacmd.Client implements it and tests
// fake it. There is deliberately no interrupt: a busy or half-typed input line is
// refused, never cleared.
type orcaOps interface {
	Terminals(ctx context.Context) ([]orcacmd.Terminal, *orcacmd.Failure)
	Screen(ctx context.Context, handle string) (orcacmd.Frame, *orcacmd.Failure)
	SendText(ctx context.Context, handle, text string) (orcacmd.Receipt, *orcacmd.Failure)
	CreateTerminal(ctx context.Context, path, title, command string) (string, *orcacmd.Failure)
	WaitStartup(ctx context.Context, handle, path, title string) (string, bool, *orcacmd.Failure)
}

// newOps resolves Orca and checks the app runs (a seam: tests replace it).
var newOps = func(ctx context.Context) (orcaOps, *orcacmd.Failure) {
	c, f := orcacmd.NewClient()
	if f != nil {
		return nil, f
	}
	if _, f := c.Status(ctx); f != nil {
		return nil, f
	}
	return c, nil
}
