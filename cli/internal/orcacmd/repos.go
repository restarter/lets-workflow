//go:build unix

package orcacmd

import (
	"context"
	"os"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// RepoInfo is one repository Orca knows, validated as a main checkout.
type RepoInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

// ReposInfo lists the usable repos and names what was dropped.
type ReposInfo struct {
	Repos   []RepoInfo `json:"repos"`
	Dropped []string   `json:"dropped,omitempty"` // display names (never raw paths) that were not main checkouts
	Reason  string     `json:"reason,omitempty"`  // orca_unavailable | repo_list_unrecognized
}

// ReposResult is the repos-subcommand envelope.
type ReposResult struct {
	Envelope
	Repos *ReposInfo `json:"repos,omitempty"`
}

// ListRepos decodes `repo list --json` in Go. Each path must be a directory that git
// reports as a main checkout; anything else is dropped with a note, so no caller ever
// types an Orca-supplied path into a shell. Indexes are the order Orca returned.
func ListRepos(ctx context.Context) (*ReposInfo, *Failure) {
	info := &ReposInfo{Repos: []RepoInfo{}}
	c, f := NewClient()
	if f != nil {
		info.Reason = "orca_unavailable"
		return info, f
	}
	if _, f := c.Status(ctx); f != nil {
		info.Reason = "orca_unavailable"
		return info, f
	}
	out, f := c.Run(ctx, "repo list", "repo", "list", "--json")
	if f != nil {
		info.Reason = "orca_unavailable"
		return info, f
	}
	var env struct {
		Result *struct {
			Repos []struct {
				Path        string `json:"path"`
				DisplayName string `json:"displayName"`
			} `json:"repos"`
		} `json:"result"`
	}
	if DecodeJSON("repo list", out, &env) != nil || env.Result == nil {
		info.Reason = "repo_list_unrecognized"
		return info, &Failure{Reason: "repo_list_unrecognized", Verb: "repo list"}
	}
	for i, r := range env.Result.Repos {
		name := Truncate(redact.Control(r.DisplayName), 80)
		fi, err := os.Stat(r.Path)
		inWt, main := gitutil.DetectInsideWorktreeAt(r.Path)
		if r.Path == "" || err != nil || !fi.IsDir() || inWt || main == "" || !fsutil.SameDir(main, r.Path) {
			info.Dropped = append(info.Dropped, name)
			continue
		}
		info.Repos = append(info.Repos, RepoInfo{Index: i, Name: name, Path: r.Path})
	}
	return info, nil
}

// RepoByIndex resolves a --repo-index against a fresh repo list.
func RepoByIndex(ctx context.Context, idx int) (string, *Failure) {
	info, f := ListRepos(ctx)
	if f != nil {
		return "", f
	}
	for _, r := range info.Repos {
		if r.Index == idx {
			return r.Path, nil
		}
	}
	return "", &Failure{Reason: "repo_index_unknown", Verb: "repo list"}
}

// Repos is `lets orca repos`.
func Repos(ctx context.Context) (*ReposResult, error) {
	res := &ReposResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "repos", Steps: []Step{}}}
	info, _ := ListRepos(ctx)
	res.Repos, res.OK = info, true
	return res, nil
}
