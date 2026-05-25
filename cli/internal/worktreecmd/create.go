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

	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
)

// detectInsideWorktreeAt inspects whether the given path is inside a git
// worktree (rather than the main repo). Unlike initcmd.DetectInsideWorktree
// it does NOT use cwd — important for callers that operate on an explicit
// projectRoot (and for tests that run from a worktree but target a fresh
// temp repo). Returns false on any git error.
func detectInsideWorktreeAt(ctx context.Context, path string) bool {
	gitDir, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--git-dir").Output()
	if err != nil {
		return false
	}
	commonDir, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return false
	}
	resolve := func(s string) string {
		s = strings.TrimSpace(s)
		if !filepath.IsAbs(s) {
			abs, err := filepath.Abs(filepath.Join(path, s))
			if err != nil {
				return s
			}
			return abs
		}
		return s
	}
	return resolve(string(gitDir)) != resolve(string(commonDir))
}

// CreateOptions configures the create flow.
type CreateOptions struct {
	Name               string
	Mode               BranchMode
	Base               string
	NoSymlinkLets      bool
	NoSymlinkBeads     bool
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
	if detectInsideWorktreeAt(ctx, projectRoot) {
		addStep(StepErr, "guard: cannot create worktree from inside a worktree")
		return fail(ErrInsideWorktree())
	}
	addStep(StepOK, "guard: in main repo")
	_ = initcmd.DetectInsideWorktree // keep import warm; cwd-based variant still used by lets init

	// Step 3: resolve base ref (LETS_MERGE_BRANCH from .lets/.env, fallback "main").
	base := opts.Base
	if base == "" {
		base = resolveBaseFromEnv(projectRoot)
	}
	addStep(StepOK, "base ref: "+base)

	// Step 4: resolve branch (attach vs create).
	plan, err := ResolveBranch(ctx, projectRoot, opts.Name, opts.Mode, base)
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
			mergeBase := resolveBaseFromEnv(projectRoot)
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
		if strings.Contains(msg, "already exists") {
			kind = "worktree_path_exists"
			code = ExitWorktreeExists
		}
		return fail(&Error{
			Code:        code,
			Kind:        kind,
			Message:     msg,
			Remediation: "if path is stale: lets worktree remove " + opts.Name + " (use --force if dirty)",
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

	// Step 8: symlink .lets/ (replace any pre-existing dir that statusline may have created).
	letsSymlinked := false
	defensiveRemove := false
	if !opts.NoSymlinkLets {
		mainLets := filepath.Join(projectRoot, ".lets")
		wtLets := filepath.Join(wtPath, ".lets")
		if fi, err := os.Lstat(wtLets); err == nil {
			if fi.Mode()&os.ModeSymlink == 0 {
				// Real dir present (statusline-race or otherwise). Remove and replace.
				_ = os.RemoveAll(wtLets)
				defensiveRemove = true
			} else {
				_ = os.Remove(wtLets) // stale symlink
			}
		}
		if err := CreateRelativeSymlink(wtLets, mainLets, projectRoot); err != nil {
			return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "symlink .lets/", err)
		}
		letsSymlinked = true
		if defensiveRemove {
			addStep(StepWarn, ".lets/ symlinked (defensive: pre-existing real dir removed; statusline race likely)")
		} else {
			addStep(StepOK, ".lets/ symlinked")
		}
	} else {
		addStep(StepSkip, ".lets/ symlink disabled by flag")
	}

	// Step 9: symlink .beads/.env + chmod 700 (and hardenings).
	beadsSymlinked := false
	if !opts.NoSymlinkBeads {
		mainBeadsEnv := filepath.Join(projectRoot, ".beads", ".env")
		if _, err := os.Stat(mainBeadsEnv); err == nil {
			wtBeads := filepath.Join(wtPath, ".beads")
			// ORDER: MkdirAll → Chmod → Symlink (eliminate visibility window).
			if err := os.MkdirAll(wtBeads, 0o700); err != nil {
				return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "mkdir .beads", err)
			}
			if err := os.Chmod(wtBeads, 0o700); err != nil {
				return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "chmod worktree .beads", err)
			}
			// Harden main side too (idempotent).
			_ = os.Chmod(filepath.Join(projectRoot, ".beads"), 0o700)
			_ = os.Chmod(mainBeadsEnv, 0o600)
			if err := CreateRelativeSymlink(filepath.Join(wtBeads, ".env"), mainBeadsEnv, projectRoot); err != nil {
				return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "symlink .beads/.env", err)
			}
			beadsSymlinked = true
			addStep(StepOK, ".beads/.env symlinked, chmod 700/600")
		} else {
			addStep(StepSkip, "no .beads/.env in main repo")
		}
	} else {
		addStep(StepSkip, ".beads/.env symlink disabled by flag")
	}

	// Step 10: verify.
	if err := VerifyCreate(ctx, projectRoot, wtPath, plan); err != nil {
		return rollback(ctx, result, projectRoot, wtPath, plan, prevMainBranch, "verify failed", err)
	}
	addStep(StepOK, "verify: branch, symlinks, paths")

	// Success.
	result.OK = true
	result.Worktree.LetsSymlinked = letsSymlinked
	result.Worktree.BeadsSymlinked = beadsSymlinked
	// Tracked-.lets/ opt-in warning.
	if cmd := exec.CommandContext(ctx, "git", "-C", projectRoot, "check-ignore", "-q", ".lets/"); cmd.Run() != nil {
		addStep(StepWarn, ".lets/ is NOT gitignored — tracked-`.lets/` opt-in not supported yet; symlink may show as untracked in worktree")
	}
	// Auto-switch UX: success-path notification. Main repo is now on a different
	// branch than user started on; surface explicit restore command.
	if prevMainBranch != "" {
		addStep(StepWarn, fmt.Sprintf("main repo left on %s (was on %s); restore with: git -C %s switch %s",
			resolveBaseFromEnv(projectRoot), prevMainBranch, projectRoot, prevMainBranch))
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

func resolveBaseFromEnv(projectRoot string) string {
	f, err := os.Open(filepath.Join(projectRoot, ".lets", ".env"))
	if err != nil {
		return "main"
	}
	defer f.Close()
	vals, err := envfile.Parse(f)
	if err != nil {
		return "main"
	}
	if v, ok := vals["LETS_MERGE_BRANCH"]; ok && v != "" {
		return v
	}
	return "main"
}

func currentBranch(ctx context.Context, repo string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "branch", "--show-current").Output()
	return strings.TrimSpace(string(out)), err
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
	for _, marker := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "BISECT_LOG"} {
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
