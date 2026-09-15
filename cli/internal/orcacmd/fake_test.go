//go:build unix

package orcacmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeOrca replaces the binary lookup and the exec seam for one test. handler gets
// the argv and returns stdout, stderr and whether the call fails.
type fakeCall struct{ args []string }

func useFakeOrca(t *testing.T, handler func(args []string) (string, string, bool)) *[]fakeCall {
	t.Helper()
	calls := &[]fakeCall{}
	oldLook, oldRun := lookOrca, runOrca
	lookOrca = func() (string, bool) { return "/fake/orca", true }
	runOrca = func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
		*calls = append(*calls, fakeCall{args: args})
		out, errOut, fail := handler(args)
		if fail {
			return []byte(out), []byte(errOut), errors.New("exit status 1")
		}
		return []byte(out), []byte(errOut), nil
	}
	t.Cleanup(func() { lookOrca, runOrca = oldLook, oldRun })
	return calls
}

func joined(args []string) string { return strings.Join(args, " ") }

const statusRunning = `{"ok":true,"result":{"app":{"running":true},"runtime":{"appVersion":"1.4.203"}}}`
