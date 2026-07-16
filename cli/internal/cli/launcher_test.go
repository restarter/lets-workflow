package cli

import (
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

// TestShippedLaunchers_MatchSubcommands pins letsconfig.ShippedLaunchers to the
// subcommands actually registered on the root command, in BOTH directions. This
// is the launcher analogue of initcmd's TestShippedTrackers_MatchOnDisk: a
// tracker adapter is a file on disk, a launcher is a cobra subcommand.
//
//   - whitelist entry with no subcommand -> `lets init --launcher=X` would accept
//     a value the binary cannot serve.
//   - launcher subcommand missing from the whitelist -> `lets init --launcher=X`
//     would reject a launcher that actually works.
//
// "terminal" is exempt: it is the no-subcommand launcher (print a cd command).
func TestShippedLaunchers_MatchSubcommands(t *testing.T) {
	const noSubcommand = "terminal"

	registered := map[string]bool{}
	for _, c := range NewRootCmd().Commands() {
		registered[c.Name()] = true
	}

	whitelisted := map[string]bool{}
	for _, l := range letsconfig.ShippedLaunchers {
		whitelisted[l] = true
		if l == noSubcommand {
			continue
		}
		if !registered[l] {
			t.Errorf("ShippedLaunchers lists %q but no `lets %s` subcommand is registered (init would accept an unservable value)", l, l)
		}
	}

	// Reverse direction: every launcher-shaped subcommand must be whitelisted.
	// Hand-maintained because cobra cannot tell a launcher subcommand from any
	// other (init, hook, …). Adding a launcher means adding it here AND to
	// ShippedLaunchers - the test then enforces the subcommand exists.
	for _, name := range []string{"cmux", "tmux"} {
		if registered[name] && !whitelisted[name] {
			t.Errorf("`lets %s` is registered but %q is missing from ShippedLaunchers (init would reject a working launcher)", name, name)
		}
	}
}
