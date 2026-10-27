//go:build unix

package worktreecmd

import (
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// ErrBaseRefusal maps every gitutil refusal kind to its own exit and kind, and names
// --base in the remediation only for a verb that takes it, where the kind offers one
// (c2-F3: a failing `git remote` is a git failure, never 31).
func TestErrBaseRefusal(t *testing.T) {
	for _, c := range []struct {
		kind     string
		code     int
		baseHint bool // the remediation names --base when hasBase
	}{
		{gitutil.BaseNoRemoteBase, ExitNoRemoteBase, true},
		{gitutil.BaseRemoteAmbiguous, ExitNoRemoteBase, true},
		{gitutil.BaseRefMissing, ExitBranchConflict, true},
		{gitutil.BaseRemoteListFailed, ExitGitFailed, false},
	} {
		for _, hasBase := range []bool{true, false} {
			r := &gitutil.BaseRefusal{Kind: c.kind, Remote: "origin", Message: "msg " + c.kind}
			e := ErrBaseRefusal(r, "main", hasBase)
			if e.Code != c.code || e.Kind != c.kind || e.Message != r.Message || e.Remediation == "" {
				t.Errorf("%s (hasBase=%v) -> %d/%s/%q/%q, want %d/%s", c.kind, hasBase, e.Code, e.Kind, e.Message, e.Remediation, c.code, c.kind)
			}
			if named, want := strings.Contains(e.Remediation, "--base"), hasBase && c.baseHint; named != want {
				t.Errorf("%s (hasBase=%v): remediation %q names --base = %v, want %v", c.kind, hasBase, e.Remediation, named, want)
			}
		}
	}
}
