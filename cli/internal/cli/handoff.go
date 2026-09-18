//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restarter/lets-workflow/cli/internal/agentrun"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/handoffcmd"
)

// NewHandoffCmd builds `lets handoff` (targets / send / codex / await): delivering a
// hand-off brief to an agent and bringing its final report back. Orca and Codex are
// optional; their absence is a named reason, not an error.
func NewHandoffCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "handoff",
		Short:         "Deliver a hand-off brief to an agent and bring its report back",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newHandoffTargetsCmd(), newHandoffSendCmd(), newHandoffCodexCmd(), newHandoffAwaitCmd())
	return root
}

// handoffRoot is this checkout's toplevel.
func handoffRoot() string {
	cwd, _ := os.Getwd()
	return gitutil.ProjectRoot(cwd, 2*time.Second)
}

func printHandoff(cmd *cobra.Command, jsonOut bool, res any, human func()) {
	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return
	}
	human()
}

func newHandoffTargetsCmd() *cobra.Command {
	var (
		match   string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:           "targets",
		Short:         "List the agent terminals of this checkout a brief can go to",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := handoffcmd.Targets(cmd.Context(), handoffcmd.TargetsOptions{Root: handoffRoot(), Self: os.Getenv("ORCA_TERMINAL_HANDLE"), Match: match})
			printHandoff(cmd, jsonOut, res, func() { handoffcmd.RenderTargets(cmd.OutOrStdout(), res) })
			return err
		},
	}
	cmd.Flags().StringVar(&match, "match", "", "A term_ handle, an agent name (codex, antigravity), or a fragment of the tab title")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newHandoffSendCmd() *cobra.Command {
	var (
		brief, terminal, newAgent string
		jsonOut                   bool
	)
	cmd := &cobra.Command{
		Use:           "send",
		Short:         "Type a one-line pointer to the brief into an agent terminal of this checkout",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := handoffcmd.Send(cmd.Context(), handoffcmd.SendOptions{Root: handoffRoot(), Brief: brief, Self: os.Getenv("ORCA_TERMINAL_HANDLE"), Terminal: terminal, New: newAgent})
			printHandoff(cmd, jsonOut, res, func() { handoffcmd.RenderSend(cmd.OutOrStdout(), res) })
			return err
		},
	}
	cmd.Flags().StringVar(&brief, "brief", "", "Absolute path of the brief (in <root>/.lets/handoffs/)")
	cmd.Flags().StringVar(&terminal, "terminal", "", "Handle of the target terminal (from lets handoff targets)")
	cmd.Flags().StringVar(&newAgent, "new", "", "Open a new tab running this agent and send there (codex)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newHandoffCodexCmd() *cobra.Command {
	var (
		brief   string
		timeout time.Duration
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:           "codex",
		Short:         "Run the brief through Codex headless (read-only) and write its final report",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := handoffcmd.Codex(cmd.Context(), handoffcmd.RunOptions{Root: handoffRoot(), Brief: brief, Timeout: timeout})
			printHandoff(cmd, jsonOut, res, func() { handoffcmd.RenderRun(cmd.OutOrStdout(), res) })
			return err
		},
	}
	cmd.Flags().StringVar(&brief, "brief", "", "Absolute path of the brief (in <root>/.lets/handoffs/)")
	cmd.Flags().DurationVar(&timeout, "timeout", agentrun.DefaultTimeout, "Give up after this long")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}

func newHandoffAwaitCmd() *cobra.Command {
	var (
		brief, agent, since, fp string
		timeout                 time.Duration
		jsonOut                 bool
	)
	cmd := &cobra.Command{
		Use:           "await",
		Short:         "Wait for the final report of a brief sent to an agent's tab",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, perr := time.Parse(time.RFC3339Nano, since)
			if perr != nil {
				const msg = "--since must be an RFC 3339 time (send.sent_at)"
				printHandoff(cmd, jsonOut, handoffcmd.NewErrorEnvelope("await", "usage", msg), func() {})
				return &handoffcmd.Error{Code: handoffcmd.ExitUsage, Kind: "usage", Message: msg}
			}
			res, err := handoffcmd.Await(cmd.Context(), handoffcmd.AwaitOptions{Root: handoffRoot(), Brief: brief, Agent: agent, Since: t, Fingerprint: fp, Timeout: timeout})
			printHandoff(cmd, jsonOut, res, func() { handoffcmd.RenderRun(cmd.OutOrStdout(), res) })
			return err
		},
	}
	cmd.Flags().StringVar(&brief, "brief", "", "Absolute path of the brief that was sent")
	cmd.Flags().StringVar(&agent, "agent", "", "The agent in the target tab (send.agent)")
	cmd.Flags().StringVar(&since, "since", "", "When the brief was sent (send.sent_at)")
	cmd.Flags().StringVar(&fp, "fingerprint", "", "The checkout fingerprint at send time (send.fingerprint)")
	cmd.Flags().DurationVar(&timeout, "timeout", agentrun.DefaultTimeout, "Give up after this long")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON envelope")
	return cmd
}
