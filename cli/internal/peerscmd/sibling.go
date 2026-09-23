//go:build unix

package peerscmd

import (
	"context"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// listOrcaRepos is orcacmd.ListRepos, a seam so tests supply Orca's repo list.
var listOrcaRepos = orcacmd.ListRepos

// repoByIndex resolves an index through the same seam `who --orca-repos` numbered with.
func repoByIndex(ctx context.Context, idx int) (string, *orcacmd.Failure) {
	info, f := listOrcaRepos(ctx)
	if f != nil {
		return "", f
	}
	for _, r := range info.Repos {
		if r.Index == idx {
			return r.Path, nil
		}
	}
	return "", &orcacmd.Failure{Reason: "repo_index_unknown", Verb: "repo list"}
}
