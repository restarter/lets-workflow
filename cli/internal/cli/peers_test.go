//go:build unix

package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

func TestPeersTail_AcceptsPositionalName(t *testing.T) {
	cmd := cli.NewPeersCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"tail", "SOMEONE", "--cwd", t.TempDir()}) // not a repo: tail itself answers
	err := cmd.Execute()
	if err != nil && strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("a positional name must reach tail: %v", err)
	}
	// not_in_repo is only reachable once target validation passed - i.e. once the
	// positional name was copied into TailOptions.Name; a dropped name is `usage`.
	var env struct {
		Subcommand string `json:"subcommand"`
		Error      *struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &env); jerr != nil || env.Subcommand != "tail" || env.Error == nil || env.Error.Kind != "not_in_repo" {
		t.Errorf("the name must pass validation and reach repository loading: %s", out.String())
	}
}
