//go:build !unix

package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var errPeersUnsupported = errors.New("lets peers is not yet supported on Windows")

// NewPeersCmd returns the non-unix stub: who / tail / orchestrator emit a parseable
// ok=true reason=not_supported envelope (orient and the touchpoints parse them);
// the verbs that change or send something hard-error.
func NewPeersCmd() *cobra.Command {
	root := &cobra.Command{Use: "peers", Short: "Peer messaging (not yet supported on Windows)", SilenceUsage: true, SilenceErrors: true}
	for _, name := range []string{"who", "tail", "orchestrator"} {
		sub := name
		c := &cobra.Command{Use: sub, SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), `{"schema_version":1,"ok":true,"subcommand":%q,"reason":"not_supported","degraded":[]}`+"\n", sub)
			return nil
		}}
		c.Flags().SetInterspersed(true)
		c.FParseErrWhitelist.UnknownFlags = true
		root.AddCommand(c)
	}
	for _, name := range []string{"frame", "tell", "wait", "role", "ask-ro"} {
		c := &cobra.Command{Use: name, SilenceUsage: true, SilenceErrors: true, RunE: func(*cobra.Command, []string) error { return errPeersUnsupported }}
		c.FParseErrWhitelist.UnknownFlags = true
		root.AddCommand(c)
	}
	return root
}
