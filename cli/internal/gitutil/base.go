package gitutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// BaseResolution.Source values.
const (
	BaseFromRemote   = "remote"
	BaseFromLocal    = "local"
	BaseFromExplicit = "explicit"
)

// How ResolveRemote chose the remote (Remote.By, BaseResolution.RemoteBy).
const (
	RemoteByUpstream = "upstream"
	RemoteBySingle   = "single"
	RemoteByOrigin   = "origin"
	RemoteByNone     = "none"
)

// BaseRefusal kinds; worktreecmd maps them to exits (31, 31, 13), orcacmd degrades.
const (
	BaseNoRemoteBase    = "no_remote_base"
	BaseRemoteAmbiguous = "remote_ambiguous"
	BaseRefMissing      = "base_ref_missing"
)

// fetchWaitDelay bounds the wait for the pipes once the fetch's process group was killed.
const fetchWaitDelay = 2 * time.Second

// Remote is the remote a new branch's base comes from; Name is "" when By is none.
type Remote struct{ Name, By string }

// BaseRefusal is why no base could be resolved.
type BaseRefusal struct {
	Kind    string
	Remote  string
	Remotes []string
	Message string
}

func (r *BaseRefusal) Error() string { return r.Message }

// BaseResolution is how a new branch's base was chosen - the one shape every
// envelope embeds as base_resolution beside its own base_ref (lets-qjt3h: the ref
// stays a string, how it was resolved sits next to it). Ref is the human ref
// (<remote>/<merge>, or <merge> with no remote) the caller copies into base_ref;
// Sha the commit cut from; Notes the warn lines the caller turns into steps.
type BaseResolution struct {
	Ref         string   `json:"-"`
	Source      string   `json:"source"`
	Remote      string   `json:"remote,omitempty"`
	RemoteBy    string   `json:"remote_by,omitempty"`
	Sha         string   `json:"sha"`
	Fetched     bool     `json:"fetched"`
	Stale       bool     `json:"stale,omitempty"`
	FetchAge    string   `json:"fetch_age,omitempty"`
	LocalAhead  int      `json:"local_ahead,omitempty"`
	LocalBehind int      `json:"local_behind,omitempty"`
	Notes       []string `json:"-"`
}

// BaseOptions: FetchTimeout 0 is DefaultRemoteTimeout.
type BaseOptions struct{ FetchTimeout time.Duration }

// ResolveRemote picks the remote for merge from local config only - no network:
// merge's upstream remote (branch.<merge>.remote, when it names a configured
// remote; "." is a local upstream and falls through), else the only remote, else
// "origin". No remote at all is By=none; several, none of those, is
// remote_ambiguous (lets-57lb9, MAIN-LETS Q4).
func ResolveRemote(ctx context.Context, dir, merge string) (Remote, *BaseRefusal) {
	out, err := runGit(ctx, dir, "remote")
	if err != nil {
		return Remote{}, &BaseRefusal{Kind: BaseRemoteAmbiguous, Message: "cannot list the remotes: " + err.Error()}
	}
	remotes := strings.Fields(string(out))
	if len(remotes) == 0 {
		return Remote{By: RemoteByNone}, nil
	}
	if up, err := runGit(ctx, dir, "config", "--get", "branch."+merge+".remote"); err == nil {
		if name := strings.TrimSpace(string(up)); name != "." && slices.Contains(remotes, name) {
			return Remote{Name: name, By: RemoteByUpstream}, nil
		}
	}
	switch {
	case len(remotes) == 1:
		return Remote{Name: remotes[0], By: RemoteBySingle}, nil
	case slices.Contains(remotes, "origin"):
		return Remote{Name: "origin", By: RemoteByOrigin}, nil
	}
	return Remote{}, &BaseRefusal{Kind: BaseRemoteAmbiguous, Remotes: remotes,
		Message: fmt.Sprintf("%d remotes (%s): none is %s's upstream and none is named origin", len(remotes), strings.Join(remotes, ", "), merge)}
}

// ResolveBase resolves the commit a new branch is cut from - the one base policy
// of every LETS cut (lets-qjt3h, lets-57lb9):
//   - a remote R: `git fetch --no-tags R +refs/heads/<merge>:refs/remotes/R/<merge>`
//     bounded by FetchTimeout - an explicit destination, because `git fetch R <merge>`
//     updates the tracking ref only when remote.R.fetch maps it, and a narrow refspec
//     would leave it stale or absent - then refs/remotes/R/<merge>; absent ->
//     no_remote_base, never the local <merge>
//     while a remote is configured; present after a failed fetch -> Stale, with
//     the age of FETCH_HEAD; the local <merge> ahead of it -> a note, never a refusal;
//   - no remote: the local <merge> with a note; no local <merge> -> base_ref_missing.
//
// The ahead / behind counts are best effort: a failure (a shallow boundary) leaves
// them 0 and refuses nothing.
func ResolveBase(ctx context.Context, dir, merge string, o BaseOptions) (*BaseResolution, *BaseRefusal) {
	r, refusal := ResolveRemote(ctx, dir, merge)
	if refusal != nil {
		return nil, refusal
	}
	local := commitOf(ctx, dir, "refs/heads/"+merge)
	if r.By == RemoteByNone {
		if local == "" {
			return nil, &BaseRefusal{Kind: BaseRefMissing, Message: fmt.Sprintf("no remote is configured and there is no local %s", merge)}
		}
		return &BaseResolution{Ref: merge, Source: BaseFromLocal, RemoteBy: RemoteByNone, Sha: local,
			Notes: []string{"no remote configured - cutting from the local " + merge}}, nil
	}
	timeout := o.FetchTimeout
	if timeout <= 0 {
		timeout = DefaultRemoteTimeout
	}
	ref := r.Name + "/" + merge
	ferr := fetchBounded(ctx, dir, timeout, "fetch", "--no-tags", r.Name, "+refs/heads/"+merge+":refs/remotes/"+ref)
	sha := commitOf(ctx, dir, "refs/remotes/"+ref)
	if sha == "" {
		msg := ref + " is missing"
		if ferr != nil {
			msg += " (fetch failed: " + ferr.Error() + ")"
		}
		return nil, &BaseRefusal{Kind: BaseNoRemoteBase, Remote: r.Name,
			Message: msg + "; a new branch is never cut from the local " + merge + " while a remote is configured"}
	}
	res := &BaseResolution{Ref: ref, Source: BaseFromRemote, Remote: r.Name, RemoteBy: r.By, Sha: sha, Fetched: ferr == nil}
	if ferr != nil {
		res.Stale, res.FetchAge = true, fetchHeadAge(ctx, dir)
		res.Notes = append(res.Notes, fmt.Sprintf("fetch of %s failed (%v); cutting from the local copy of %s, last fetched %s ago", ref, ferr, ref, res.FetchAge))
	}
	if local != "" {
		if out, err := runGit(ctx, dir, "rev-list", "--left-right", "--count", "refs/heads/"+merge+"...refs/remotes/"+ref); err == nil {
			if f := strings.Fields(string(out)); len(f) == 2 {
				res.LocalAhead, _ = strconv.Atoi(f[0])
				res.LocalBehind, _ = strconv.Atoi(f[1])
			}
		}
		if res.LocalAhead > 0 {
			res.Notes = append(res.Notes, fmt.Sprintf("%d local commit(s) on %s are not in the base %s", res.LocalAhead, merge, ref))
		}
	}
	return res, nil
}

// commitOf is ref's commit, "" when it does not resolve.
func commitOf(ctx context.Context, dir, ref string) string {
	out, err := runGit(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// fetchHeadAge is the age of FETCH_HEAD - this checkout's, else the common dir's
// (as switch reported it before) - or "unknown".
func fetchHeadAge(ctx context.Context, dir string) string {
	var paths []string
	if out, err := runGit(ctx, dir, "rev-parse", "--git-path", "FETCH_HEAD"); err == nil {
		p := strings.TrimSpace(string(out))
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		paths = append(paths, p)
	}
	if out, err := runGit(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		paths = append(paths, filepath.Join(strings.TrimSpace(string(out)), "FETCH_HEAD"))
	}
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			return time.Since(fi.ModTime()).Round(time.Minute).String()
		}
	}
	return "unknown"
}
