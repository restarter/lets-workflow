//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// CreateOptions configures the create flow.
type CreateOptions struct {
	Name               string
	Branch             string // explicit branch ref (--branch); may contain '/'. Empty = derive from Name.
	Mode               BranchMode
	Base               string
	NoSymlinkLets      bool
	NoStoreLinks       bool   // skip the tracker adapter's declared store links (--no-store-links; --no-symlink-beads alias)
	PluginRoot         string // plugin root for the adapter fallback (--plugin-root, else CLAUDE_PLUGIN_ROOT)
	SwitchMainIfNeeded bool
}

// minGitVersion is required for `git worktree remove --force` (since 2.17).
const minGitVersion = "2.17"

// Create executes the full create flow. Returns CreateResult and an error
// (nil on success). Caller (cobra wrapper) maps error to exit code via
// ExitCode(err) and emits the JSON envelope.
//
// Task 4a skeleton: validate → guard → resolveBranch → gitignore → git
// worktree add. Task 4b appends post-create symlinks + verify + rollback.
func Create(ctx context.Context, projectRoot string, opts CreateOptions) (*CreateResult, error) {
	result := &CreateResult{
		Envelope: Envelope{
			SchemaVersion: SchemaVersion,
			Subcommand:    "create",
			ProjectRoot:   projectRoot,
			Steps:         []Step{},
		},
	}
	addStep := func(status, msg string) {
		result.Steps = append(result.Steps, Step{Status: status, Message: msg})
	}
	fail := func(e *Error) (*CreateResult, error) {
		result.OK = false
		result.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return result, e
	}
	// restoreMainIfSwitched is called by the pre-rollback failure paths
	// (Steps 6 + 7) to undo an earlier Step 5 --switch-main-if-needed.
	// Without this, a Step 6 gitignore failure (or Step 7 git-worktree-add
	// failure) after Step 5 auto-switched main would leave the user's main
	// on the merge-branch when they expected the original. Steps 8/9/10
	// route through rollback() which handles this itself.
	//
	// When the restore itself fails (transient git issue, foreign uid, etc.)
	// we surface a residual via result.Rollback so the JSON envelope makes
	// the partial state visible alongside the primary error — not buried
	// in Steps[] (review S-3).
	restoreMainIfSwitched := func(prev string) {
		if prev == "" {
			return
		}
		if !restoreMainBranch(ctx, projectRoot, prev, &result.Envelope, "restore: ") {
			if result.Rollback == nil {
				result.Rollback = &RollbackInfo{Attempted: true, Succeeded: false}
			} else {
				result.Rollback.Succeeded = false
			}
			result.Rollback.Residual = append(result.Rollback.Residual,
				fmt.Sprintf("main_repo_on_branch:%s (expected %s)", currentBranchOr(ctx, projectRoot), prev))
		}
	}

	// Pre-flight: minimum git version.
	if err := checkGitVersion(ctx); err != nil {
		var e *Error
		if errors.As(err, &e) {
			return fail(e)
		}
		return fail(&Error{Code: ExitGitFailed, Kind: "git_version", Cause: err})
	}

	// Step 1: validate name.
	if err := ValidateName(ctx, opts.Name); err != nil {
		var e *Error
		if errors.As(err, &e) {
			return fail(e)
		}
		return fail(&Error{Code: ExitUsage, Kind: "name_validation", Cause: err})
	}

	// Step 2: guard not-inside-worktree. Uses projectRoot-anchored check so
	// callers (and tests) that pass a fresh repo while running from inside an
	// unrelated worktree are not falsely rejected.
	if inside, _ := initcmd.DetectInsideWorktreeAt(projectRoot); inside {
		addStep(StepErr, "guard: cannot create worktree from inside a worktree")
		return fail(ErrInsideWorktree())
	}
	addStep(StepOK, "guard: in main repo")

	// Step 3: resolve base ref (LETS_MERGE_BRANCH from .lets/.env, fallback "main").
	base := opts.Base
	if base == "" {
		base = mergeBranch(projectRoot)
	}
	addStep(StepOK, "base ref: "+base)

	// Step 4: resolve branch (attach vs create). opts.Branch, when set, decouples
	// the attached/created ref from the worktree dir name (lets-x5ucf).
	plan, err := ResolveBranch(ctx, projectRoot, opts.Name, opts.Branch, opts.Mode, base)
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return fail(e)
		}
		return fail(&Error{Code: ExitGitFailed, Kind: "resolve_branch", Cause: err})
	}
	addStep(StepOK, fmt.Sprintf("branch: %s (%s)", plan.Branch, plan.Mode))

	// Step 5: attach pre-check — refuse if branch is checked out in main.
	// If --switch-main-if-needed: capture prevBranch BEFORE switch so rollback
	// can restore it. prevMainBranch stored on result so rollback() can read.
	var prevMainBranch string
	if plan.Mode == "attached" {
		cur, _ := currentBranch(ctx, projectRoot)
		if cur == plan.Branch {
			if !opts.SwitchMainIfNeeded {
				return fail(ErrBranchCheckedOutInMain(plan.Branch))
			}
			if err := ensureCleanTree(ctx, projectRoot); err != nil {
				var e *Error
				if errors.As(err, &e) {
					return fail(e)
				}
				return fail(&Error{Code: ExitDirtyWorktree, Kind: "main_repo_dirty", Cause: err})
			}
			prevMainBranch = cur // capture before switch
			mergeBase := mergeBranch(projectRoot)
			if out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "switch", mergeBase).CombinedOutput(); err != nil {
				return fail(&Error{
					Code:    ExitGitFailed,
					Kind:    "main_switch_failed",
					Message: redactCreds(strings.TrimSpace(string(out))),
					Cause:   err,
				})
			}
			addStep(StepWarn, fmt.Sprintf("auto-switched main repo to %s (was on %s); will restore on rollback", mergeBase, prevMainBranch))
		}
	}

	// Step 6: ensure .gitignore entries via shared initcmd helper
	// (race-safe via flock + integrity check after Task 3 hardening).
	if err := initcmd.EnsureGitignore(projectRoot, []string{".worktrees/", ".lets"}); err != nil {
		restoreMainIfSwitched(prevMainBranch)
		return fail(&Error{
			Code:    ExitFilesystem,
			Kind:    "gitignore_update",
			Message: err.Error(),
			Cause:   err,
		})
	}
	addStep(StepOK, ".gitignore ensured (.worktrees/, .lets)")

	// Step 7: git worktree add (no pre-existence Lstat; trust git's atomic registration).
	wtPath := filepath.Join(projectRoot, ".worktrees", opts.Name)
	var gitArgs []string
	if plan.Mode == "attached" {
		gitArgs = []string{"-C", projectRoot, "worktree", "add", wtPath, plan.Branch}
	} else {
		gitArgs = []string{"-C", projectRoot, "worktree", "add", "-b", plan.Branch, wtPath, plan.Base}
	}
	if out, err := exec.CommandContext(ctx, "git", gitArgs...).CombinedOutput(); err != nil {
		msg := redactCreds(strings.TrimSpace(string(out)))
		kind := "git_worktree_add_failed"
		code := ExitGitFailed
		remediation := "if path is stale: lets worktree remove " + opts.Name + " (use --force if dirty)"
		switch {
		// Path collision (e.g. `.worktrees/foo` already exists or `worktree-foo`
		// branch already exists when creating in new-branch mode).
		case strings.Contains(msg, "already exists"):
			kind = "worktree_path_exists"
			code = ExitWorktreeExists
		// Branch already checked out in ANOTHER worktree (review S-2). git
		// emits "fatal: 'feat' is already used by worktree at '...'". Route
		// to ExitBranchConflict so scripts can branch on it without parsing
		// prose; remediation points at the offending worktree.
		case strings.Contains(msg, "is already used by worktree"):
			kind = "branch_in_use_other_worktree"
			code = ExitBranchConflict
			remediation = "the target branch is checked out in another worktree; lets worktree list to find it, then remove or pick a different name"
		}
		restoreMainIfSwitched(prevMainBranch)
		return fail(&Error{
			Code:        code,
			Kind:        kind,
			Message:     msg,
			Remediation: remediation,
			Cause:       err,
		})
	}
	addStep(StepOK, "git worktree add")

	// Stash for Task 4b: wtPath + plan are now the active state. Task 4b
	// installs symlinks, runs verify, and on failure invokes rollback.
	result.Worktree = &WorktreeInfo{
		Name:       opts.Name,
		Path:       wtPath,
		Branch:     plan.Branch,
		BranchMode: plan.Mode,
		BaseRef:    plan.Base,
	}
	result.NextSteps = &NextSteps{AbsolutePath: wtPath}

	// Step 8: symlink .lets/ (a pre-existing real dir can only be the statusline race
	// in a worktree lets just made, so create mode replaces it).
	addSteps := func(steps []Step) { result.Steps = append(result.Steps, steps...) }
	letsSymlinked := false
	if !opts.NoSymlinkLets {
		var letsSteps []Step
		if _, err := linkLets(projectRoot, wtPath, modeCreate, func(s, m string) { letsSteps = append(letsSteps, Step{Status: s, Message: m}) }); err != nil {
			addSteps(letsSteps)
			return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "symlink .lets/", err)
		}
		addSteps(letsSteps)
		letsSymlinked = true
	} else {
		addStep(StepSkip, ".lets/ symlink disabled by flag")
	}

	// Step 9: the tracker adapter's declared store links (tracker-<name>.md `## Worktree`
	// `links:`), never a hardcoded store path.
	var storeLinks []StoreLink
	if !opts.NoStoreLinks {
		links := loadStoreLinks(projectRoot, opts.PluginRoot, addStep)
		for _, l := range links {
			var linkSteps []Step
			linked, err := linkStore(projectRoot, wtPath, l, func(s, m string) { linkSteps = append(linkSteps, Step{Status: s, Message: m}) })
			addSteps(linkSteps)
			storeLinks = append(storeLinks, StoreLink{Path: l.Path, Linked: linked})
			if err != nil {
				return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "store link "+l.Path, err)
			}
		}
	} else {
		addStep(StepSkip, "store links disabled by flag")
	}

	// Step 9.5: ensure the LETS-managed symlinks are ignored INSIDE the worktree
	// via the shared info/exclude (lets-x5ucf). EnsureGitignore (Step 6) only
	// touches the main repo's working .gitignore; the worktree checks out its
	// branch's committed copy, which can lack the entry (or carry a dir-only
	// `/.lets/` that misses the `.lets` symlink) - leaving the links as untracked
	// noise. Only ignore what we actually linked; best-effort.
	var excludes []string
	if letsSymlinked {
		excludes = append(excludes, ".lets", ".lets.pre-adopt*")
	}
	for _, sl := range storeLinks {
		if sl.Linked {
			excludes = append(excludes, sl.Path)
		}
	}
	if len(excludes) > 0 {
		if err := ensureWorktreeExcludes(ctx, projectRoot, excludes); err != nil {
			addStep(StepWarn, fmt.Sprintf("could not update .git/info/exclude (%s): %v - symlinks may show as untracked in the worktree", strings.Join(excludes, ", "), err))
		} else {
			addStep(StepOK, "info/exclude ensured ("+strings.Join(excludes, ", ")+")")
		}
	}

	// Step 10: verify.
	if err := VerifyCreate(ctx, projectRoot, wtPath, plan, storeLinks); err != nil {
		return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "verify failed", err)
	}
	addStep(StepOK, "verify: branch, symlinks, paths")

	// Success.
	result.OK = true
	result.Worktree.LetsSymlinked = letsSymlinked
	result.Worktree.StoreLinks = storeLinks
	result.Worktree.StoreLinked = allLinked(storeLinks)
	result.Worktree.BeadsSymlinked = result.Worktree.StoreLinked // deprecated alias, see result.go
	// .lets ignore sanity: after Step 9.5 the symlink should be ignored in every
	// worktree. If it still isn't (exclude write failed, or a genuinely tracked
	// .lets), warn — it would surface as untracked. Check `.lets` (no slash) so a
	// symlink is matched, unlike the old dir-only `.lets/` probe.
	if letsSymlinked {
		if cmd := exec.CommandContext(ctx, "git", "-C", projectRoot, "check-ignore", "-q", ".lets"); cmd.Run() != nil {
			addStep(StepWarn, ".lets is NOT ignored by git — the worktree symlink may show as untracked; add `.lets` to .git/info/exclude or .gitignore")
		}
	}
	// Auto-switch UX: success-path notification. Main repo is now on a different
	// branch than user started on; surface explicit restore command.
	if prevMainBranch != "" {
		addStep(StepWarn, fmt.Sprintf("main repo left on %s (was on %s); restore with: git -C %s switch %s",
			mergeBranch(projectRoot), prevMainBranch, projectRoot, prevMainBranch))
	}
	return result, nil
}

func checkGitVersion(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "git", "--version").Output()
	if err != nil {
		return &Error{Code: ExitGitFailed, Kind: "git_not_found", Cause: err}
	}
	// Parse "git version 2.40.1" — naive: extract third whitespace-token.
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return nil // can't parse, allow through
	}
	if compareSemver(fields[2], minGitVersion) < 0 {
		return &Error{
			Code:        ExitGitFailed,
			Kind:        "git_too_old",
			Message:     fmt.Sprintf("git %s+ required; you have %s", minGitVersion, fields[2]),
			Remediation: "upgrade git",
		}
	}
	return nil
}

// compareSemver returns -1/0/1 for a<b, a==b, a>b on dotted-numeric versions.
// Tolerates "-rc.1" suffix by stripping at first '-'.
func compareSemver(a, b string) int {
	norm := func(s string) []int {
		if i := strings.IndexByte(s, '-'); i >= 0 {
			s = s[:i]
		}
		parts := strings.Split(s, ".")
		out := make([]int, len(parts))
		for i, p := range parts {
			n, _ := strconv.Atoi(p)
			out[i] = n
		}
		return out
	}
	av, bv := norm(a), norm(b)
	maxLen := len(av)
	if len(bv) > maxLen {
		maxLen = len(bv)
	}
	for i := 0; i < maxLen; i++ {
		var x, y int
		if i < len(av) {
			x = av[i]
		}
		if i < len(bv) {
			y = bv[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// loadStoreLinks reads the active tracker adapter's declared store links. A load
// reason (missing adapter, undeclared links, an adapter older than the plugin) is a
// warning naming /lets:update, never a failure.
func loadStoreLinks(projectRoot, pluginFlag string, addStep func(status, msg string)) []trackeradapter.Link {
	home, _ := os.UserHomeDir()
	tracker := letsconfig.ResolvedEnv(projectRoot, home, nil)["LETS_TRACKER"]
	if tracker == "" {
		tracker = "beads"
	}
	pluginRoot, err := initcmd.DetectPluginRoot(pluginFlag)
	if err != nil {
		pluginRoot = ""
	}
	wt, reason := trackeradapter.Load(projectRoot, tracker, pluginRoot)
	if reason != "" {
		addStep(StepWarn, fmt.Sprintf("tracker adapter %q: %s - run /lets:update so the installed adapter declares its ## Worktree links", tracker, reason))
	}
	return wt.Links
}

// allLinked reports whether every declared store link is a symlink (and there is at least one).
func allLinked(links []StoreLink) bool {
	if len(links) == 0 {
		return false
	}
	for _, l := range links {
		if !l.Linked {
			return false
		}
	}
	return true
}

// mergeBranch resolves LETS_MERGE_BRANCH through the ONE resolver
// (project .lets/.env over ~/.lets/.env, then the origin default branch, then main).
func mergeBranch(projectRoot string) string {
	home, _ := os.UserHomeDir()
	return letsconfig.ResolvedEnv(projectRoot, home, func(r string) string { return gitutil.DefaultBranch(r, 2*time.Second) })["LETS_MERGE_BRANCH"]
}

func currentBranch(ctx context.Context, repo string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "branch", "--show-current").Output()
	return strings.TrimSpace(string(out)), err
}

// ensureWorktreeExcludes appends any missing entries to the repo's shared git
// exclude file (the common dir's info/exclude). Why not the tracked .gitignore:
// EnsureGitignore writes the main repo's WORKING .gitignore, but a fresh worktree
// checks out its branch's COMMITTED .gitignore — which may lack the LETS entries
// (or carry only a directory-form `/.lets/` that can't match the `.lets` symlink),
// leaving `.lets` / the declared store links showing as untracked inside the worktree
// (lets-x5ucf, child-repo scenario). info/exclude lives in the common git dir,
// so it is shared across main + every worktree, is untracked, and is never pushed
// to collaborators — the right layer to ignore LETS-managed symlinks regardless of
// the committed .gitignore. Best-effort: the caller surfaces failures as a StepWarn,
// never blocks create. Entries are narrow (`.lets`, each declared link path) so
// untracked non-LETS content next to a link still surfaces in `git status`.
func ensureWorktreeExcludes(ctx context.Context, projectRoot string, entries []string) error {
	out, err := exec.CommandContext(ctx, "git", "-C", projectRoot, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return fmt.Errorf("resolve info/exclude path: %w", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return fmt.Errorf("empty info/exclude path")
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, p)
	}
	data, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		existing[strings.TrimSpace(line)] = true
	}
	var toAppend []string
	for _, e := range entries {
		if !existing[e] {
			toAppend = append(toAppend, e)
		}
	}
	if len(toAppend) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	buf := append([]byte{}, data...)
	if len(buf) > 0 && buf[len(buf)-1] != '\n' {
		buf = append(buf, '\n')
	}
	for _, e := range toAppend {
		buf = append(buf, []byte(e+"\n")...)
	}
	return initcmd.AtomicWriteBytes(p, buf, 0o644)
}

// ensureCleanTree refuses if main repo has uncommitted changes OR is mid-operation
// (rebase, merge, cherry-pick, bisect).
func ensureCleanTree(ctx context.Context, repo string) error {
	out, _ := exec.CommandContext(ctx, "git", "-C", repo, "status", "--porcelain").Output()
	if len(strings.TrimSpace(string(out))) > 0 {
		return &Error{
			Code:        ExitDirtyWorktree,
			Kind:        "main_repo_dirty",
			Message:     "main repo has uncommitted changes",
			Remediation: "commit or stash changes in main repo, then retry",
		}
	}
	// Check mid-operation markers.
	gitDir, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--git-dir").Output()
	if err != nil {
		return &Error{Code: ExitGitFailed, Kind: "git_dir_lookup", Cause: err}
	}
	gd := strings.TrimSpace(string(gitDir))
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(repo, gd)
	}
	// Mid-op markers we refuse to switch through. Review S-12 added
	// REVERT_HEAD (git revert with conflicts) and AUTO_MERGE (git 2.41+
	// recursive merge in progress) on top of the original five.
	for _, marker := range []string{
		"rebase-merge", "rebase-apply",
		"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD",
		"BISECT_LOG", "AUTO_MERGE",
	} {
		if _, err := os.Stat(filepath.Join(gd, marker)); err == nil {
			return &Error{
				Code:        ExitDirtyWorktree,
				Kind:        "main_repo_mid_op",
				Message:     fmt.Sprintf("main repo is mid-operation (%s present)", marker),
				Remediation: "complete or abort the in-progress operation in main, then retry",
			}
		}
	}
	return nil
}
