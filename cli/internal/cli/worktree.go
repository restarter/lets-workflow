package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNotImplemented is a placeholder returned by every subcommand stub
// in this scaffold. Task 9 replaces these RunE bodies with the real
// orchestration that calls into worktreecmd.{Create,Remove,List,Info}.
var errNotImplemented = errors.New("not implemented yet (lets-rqep4 scaffold)")

// NewWorktreeCmd builds `lets worktree` with its 4 subcommand stubs.
// Subcommands inherit SilenceUsage + SilenceErrors so cobra doesn't
// double-print the error after a JSON envelope is emitted (Task 9
// wires JSON output via cmd.OutOrStdout / cmd.ErrOrStderr).
func NewWorktreeCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "worktree",
		Short:         "Manage interactive git worktrees with LETS-managed symlinks",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	for _, sub := range []*cobra.Command{
		newWorktreeCreateCmd(),
		newWorktreeRemoveCmd(),
		newWorktreeListCmd(),
		newWorktreeInfoCmd(),
	} {
		sub.SilenceUsage = true
		sub.SilenceErrors = true
		root.AddCommand(sub)
	}
	return root
}

// newWorktreeCreateCmd returns the create subcommand stub. Flags + body
// wired in Task 9. No --launch flag (cut per Decision 13 in the plan).
func newWorktreeCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree or attach an existing branch",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newWorktreeRemoveCmd returns the remove subcommand stub.
func newWorktreeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a worktree and clean up symlinks",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newWorktreeListCmd returns the list subcommand stub.
func newWorktreeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List worktrees with LETS annotations",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newWorktreeInfoCmd returns the info subcommand stub.
func newWorktreeInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show worktree status for the current directory",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}
