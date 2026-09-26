package gitutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Remote-containment states of RemoteContainsAny.
const (
	RemotePushed     = "pushed"
	RemoteNotPushed  = "not_pushed"
	RemoteUnverified = "unverified"
)

// DefaultRemoteTimeout bounds each network step of RemoteContainsAny.
const DefaultRemoteTimeout = 20 * time.Second

// maxRemoteHeads caps the live head list; a remote with more is not checked.
const maxRemoteHeads = 200

// ErrTooManyHeads: the remote has more heads than RemoteContainsAny checks.
var ErrTooManyHeads = errors.New("too_many_heads")

// runGit runs git in dir; a seam so tests can see every argv.
var runGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// RemoteContainsAny answers whether sha is on ANY branch of remote - the one Go
// owner of "is this commit pushed", asked before a history rewrite. It errs only
// towards pushed or unverified, both of which refuse the rewrite:
//
//  1. a local remote-tracking ref (refs/remotes/<remote>/) containing sha -> pushed,
//     no network (a stale tracking ref can only say pushed);
//  2. `git ls-remote --heads` live; more than 200 heads -> unverified (ErrTooManyHeads);
//  3. the tips whose objects are missing are fetched in ONE `git fetch --no-tags
//     --refmap=` by ref NAME, with no destination - FETCH_HEAD only, no ref of this
//     repository changes (a bare sha may be refused by the server);
//  4. `merge-base --is-ancestor` per head: one contains sha -> pushed, that head
//     named; none -> not_pushed.
//
// A timeout, a failed git call or a tip still missing after the fetch is unverified,
// with err saying why. head names the containing branch (without refs/heads/).
func RemoteContainsAny(ctx context.Context, dir, remote, sha string, timeout time.Duration) (state, head string, err error) {
	if timeout <= 0 {
		timeout = DefaultRemoteTimeout
	}
	if _, err := runGit(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", sha+"^{commit}"); err != nil {
		return RemoteUnverified, "", fmt.Errorf("commit %s is not in this repository: %w", sha, err)
	}
	out, err := runGit(ctx, dir, "for-each-ref", "--contains", sha, "--format=%(refname)", "refs/remotes/"+remote+"/")
	if err != nil {
		return RemoteUnverified, "", err
	}
	for _, ref := range strings.Fields(string(out)) {
		if name := strings.TrimPrefix(ref, "refs/remotes/"+remote+"/"); name != "HEAD" {
			return RemotePushed, name, nil
		}
	}

	lsCtx, cancel := context.WithTimeout(ctx, timeout)
	out, err = runGit(lsCtx, dir, "ls-remote", "--heads", remote)
	cancel()
	if err != nil {
		return RemoteUnverified, "", err
	}
	type tip struct{ sha, ref string }
	var tips []tip
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.HasPrefix(f[1], "refs/heads/") {
			tips = append(tips, tip{f[0], f[1]})
		}
	}
	if len(tips) > maxRemoteHeads {
		return RemoteUnverified, "", fmt.Errorf("%w: %s has %d heads (limit %d)", ErrTooManyHeads, remote, len(tips), maxRemoteHeads)
	}
	present := func(s string) bool {
		_, err := runGit(ctx, dir, "cat-file", "-e", s+"^{commit}")
		return err == nil
	}
	var missing []string
	for _, t := range tips {
		if !present(t.sha) {
			missing = append(missing, t.ref)
		}
	}
	if len(missing) > 0 {
		fetchCtx, cancel := context.WithTimeout(ctx, timeout)
		_, err := runGit(fetchCtx, dir, append([]string{"fetch", "--no-tags", "--no-write-fetch-head", "--refmap=", remote}, missing...)...)
		cancel()
		if err != nil {
			return RemoteUnverified, "", err
		}
		for _, t := range tips {
			if !present(t.sha) {
				return RemoteUnverified, "", fmt.Errorf("the tip of %s is still missing after the fetch", t.ref)
			}
		}
	}
	for _, t := range tips {
		_, err := runGit(ctx, dir, "merge-base", "--is-ancestor", sha, t.sha)
		if err == nil {
			return RemotePushed, strings.TrimPrefix(t.ref, "refs/heads/"), nil
		}
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 1 {
			return RemoteUnverified, "", err
		}
	}
	return RemoteNotPushed, "", nil
}

// RemotesContainAny runs RemoteContainsAny against EVERY configured remote (`git
// remote`): a commit on any branch of any remote - a fork included - is pushed,
// and remote names where. Any remote unverified (with none pushed) is unverified;
// no remote at all is not_pushed.
func RemotesContainAny(ctx context.Context, dir, sha string, timeout time.Duration) (state, remote, head string, err error) {
	out, err := runGit(ctx, dir, "remote")
	if err != nil {
		return RemoteUnverified, "", "", err
	}
	var unverified error
	for _, r := range strings.Fields(string(out)) {
		st, h, e := RemoteContainsAny(ctx, dir, r, sha, timeout)
		switch st {
		case RemotePushed:
			return RemotePushed, r, h, nil
		case RemoteUnverified:
			if unverified == nil {
				unverified = fmt.Errorf("remote %s: %w", r, e)
			}
		}
	}
	if unverified != nil {
		return RemoteUnverified, "", "", unverified
	}
	return RemoteNotPushed, "", "", nil
}
