//go:build unix

package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// Compile-time pin: worktreecmd.Error must satisfy the exitCoder interface.
// Catches accidental signature drift if someone renames ExitCode() or changes
// its return type.
var _ exitCoder = (*worktreecmd.Error)(nil)

// Runtime pin: errors.As must reach through fmt.Errorf("...%w", err) wrapping
// to find the typed exit code. Without this, main() would fall through to
// os.Exit(1) on every typed subcommand error and scripts that branch on
// $? would lose their failure-class signal.
func TestExitCoder_AsMatchesWorktreeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "bare",
			err:  &worktreecmd.Error{Code: worktreecmd.ExitDirtyWorktree, Kind: "dirty_worktree"},
			want: worktreecmd.ExitDirtyWorktree,
		},
		{
			name: "wrapped",
			err:  fmt.Errorf("wrap: %w", &worktreecmd.Error{Code: worktreecmd.ExitBranchUnmerged, Kind: "branch_unmerged"}),
			want: worktreecmd.ExitBranchUnmerged,
		},
		{
			name: "zero-code-defaults-to-generic",
			err:  &worktreecmd.Error{Kind: "broken"},
			want: worktreecmd.ExitGeneric,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ec exitCoder
			if !errors.As(c.err, &ec) {
				t.Fatalf("errors.As failed to find exitCoder in %v", c.err)
			}
			if got := ec.ExitCode(); got != c.want {
				t.Errorf("ExitCode() = %d, want %d", got, c.want)
			}
		})
	}
}

// Untyped errors must NOT match — main() falls back to os.Exit(1).
func TestExitCoder_AsRejectsUntyped(t *testing.T) {
	var ec exitCoder
	if errors.As(errors.New("plain"), &ec) {
		t.Error("plain error matched exitCoder; main() would surface the wrong exit code")
	}
}
