//go:build unix

package orcacmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func mainCheckout(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}

func TestOpen_ArgvAndResult(t *testing.T) {
	repo := mainCheckout(t)
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::/o/lets-abc-x","path":"/o/lets-abc-x","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Prompt: "/lets:start lets-abc"})
	if err != nil || !res.Launch.Launched || res.Launch.Branch != "lets-abc-x" || res.Launch.OrcaWorktreeID != "r::/o/lets-abc-x" {
		t.Fatalf("err=%v launch=%+v", err, res.Launch)
	}
	want := []string{"worktree", "create", "--repo", "path:" + repo, "--name", "lets-abc-x", "--no-parent", "--agent", "claude", "--prompt", "/lets:start lets-abc", "--json"}
	if last := (*calls)[len(*calls)-1].args; !slices.Equal(last, want) {
		t.Errorf("argv = %q\nwant  %q", last, want)
	}
}

func TestOpen_DupGuardAndFallbacks(t *testing.T) {
	repo := mainCheckout(t)
	useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "status --json" {
			return statusRunning, "", false
		}
		return `{"result":{"worktrees":[{"worktreeId":"r::/o/x","path":"/o/x","branch":"refs/heads/lets-abc-x","displayName":"lets-abc-x"}]}}`, "", false
	})
	if res, _ := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x"}); res.Launch.Launched || res.Launch.Reason != "already_open" {
		t.Errorf("dup guard: %+v", res.Launch)
	}

	useFakeOrca(t, func([]string) (string, string, bool) {
		return `{"ok":true,"result":{"app":{"running":false}}}`, "", false
	})
	if res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); err != nil || res.Launch.Launched || res.Launch.Reason != ReasonAppNotRunning || !strings.Contains(res.Launch.FallbackCommand, "--no-orca") {
		t.Errorf("app not running: err=%v %+v", err, res.Launch)
	}

	useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "status --json" {
			return statusRunning, "", false
		}
		if strings.HasPrefix(joined(args), "worktree ps") {
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":false,"error":{"code":"invalid_argument","message":"Unknown flag --agent for command: worktree create"}}`, "", true
	})
	if res, _ := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); res.Launch.Launched || res.Launch.Reason != ReasonCapabilityMissing {
		t.Errorf("capability missing: %+v", res.Launch)
	}

	oldLook := lookOrca
	lookOrca = func() (string, bool) { return "", false }
	defer func() { lookOrca = oldLook }()
	if res, _ := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); res.Launch.Reason != ReasonNotFound {
		t.Errorf("not found: %+v", res.Launch)
	}
}

func TestOpen_RepoInvalidIsHard(t *testing.T) {
	if _, err := Open(context.Background(), OpenOptions{Repo: filepath.Join(t.TempDir(), "nope"), Name: "n"}); err == nil {
		t.Error("missing repo dir must be a hard error")
	}
	plain := t.TempDir()
	if err := os.MkdirAll(filepath.Join(plain, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), OpenOptions{Repo: plain, Name: "n"}); err == nil {
		t.Error("a non-repo dir must be a hard error")
	}
}

func TestNotify_TargetFromEnvOrCwd(t *testing.T) {
	cwdTarget := t.TempDir()
	t.Setenv("ORCA_WORKTREE_ID", "")
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "worktree ps --json" {
			return `{"result":{"worktrees":[{"worktreeId":"r::cwd","path":"` + cwdTarget + `"}]}}`, "", false
		}
		return `{"ok":true}`, "", false
	})
	res, err := Notify(context.Background(), NotifyOptions{Title: "Plan ready", Body: "token=ghp_abcdefghijklmnopqrstuvwxyz0123", Cwd: cwdTarget})
	if err != nil || !res.Notify.Notified || res.Notify.Target != "r::cwd" {
		t.Fatalf("cwd target: err=%v %+v", err, res.Notify)
	}
	last := joined((*calls)[len(*calls)-1].args)
	if !strings.Contains(last, "--worktree id:r::cwd") || strings.Contains(last, "ghp_") {
		t.Errorf("set argv = %q (must target the id and redact the body)", last)
	}
	if res, _ := Notify(context.Background(), NotifyOptions{Title: "t", Cwd: t.TempDir()}); res.Notify.Notified || res.Notify.Reason != ReasonWorktreeNotFound {
		t.Errorf("no match: %+v", res.Notify)
	}
	if _, err := Notify(context.Background(), NotifyOptions{}); err == nil {
		t.Error("a missing title is a hard error")
	}
}

func TestTruncate_RuneBoundary(t *testing.T) {
	s := strings.Repeat("ж", 300)
	got := Truncate(s, 280)
	if len([]rune(got)) != 280 || !strings.HasPrefix(s, got) {
		t.Errorf("truncate cut mid-rune or wrong length: %d", len([]rune(got)))
	}
}

// openRepoWithOrigin is a main checkout with one commit pushed to a bare origin's main.
func openRepoWithOrigin(t *testing.T) string {
	t.Helper()
	repo := mainCheckout(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	for _, a := range [][]string{
		{"-C", repo, "config", "user.email", "t@e"},
		{"-C", repo, "config", "user.name", "t"},
		{"-C", repo, "commit", "-q", "--allow-empty", "-m", "base"},
		{"init", "-q", "--bare", "-b", "main", origin},
		{"-C", repo, "remote", "add", "origin", origin},
		{"-C", repo, "push", "-q", "-u", "origin", "HEAD:main"},
	} {
		if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
	}
	return repo
}

// Orca moves origin/main before it cuts: the envelope names the commit the created
// worktree actually has (Codex round 2).
func TestOpen_ReconcilesMovedBase(t *testing.T) {
	repo := openRepoWithOrigin(t)
	wt := filepath.Join(t.TempDir(), "lets-abc-x")
	moved := ""
	useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		out, err := exec.Command("git", "-C", repo, "commit-tree", "-p", "origin/main", "-m", "moved", "origin/main^{tree}").Output()
		if err != nil {
			return "", "commit-tree failed", true
		}
		moved = strings.TrimSpace(string(out))
		if err := exec.Command("git", "-C", repo, "update-ref", "refs/remotes/origin/main", moved).Run(); err != nil {
			return "", "update-ref failed", true
		}
		if err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", "lets-abc-x", wt, "origin/main").Run(); err != nil {
			return "", "worktree add failed", true
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::` + wt + `","path":"` + wt + `","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Merge: "main"})
	if err != nil || !res.Launch.Launched || res.Launch.BaseResolution == nil || res.Launch.BaseResolution.Sha != moved {
		t.Fatalf("err=%v launch=%+v, want base_resolution.sha %s", err, res.Launch, moved)
	}
}

func TestOpen_PassesResolvedBaseForNewBranch(t *testing.T) {
	repo := openRepoWithOrigin(t)
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::/o/lets-abc-x","path":"/o/lets-abc-x","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Merge: "main"})
	if err != nil || !res.Launch.Launched || res.Launch.BaseRef != "origin/main" {
		t.Fatalf("err=%v launch=%+v", err, res.Launch)
	}
	if last := (*calls)[len(*calls)-1].args; !slices.Contains(last, "--base-branch") || !slices.Contains(last, "origin/main") {
		t.Errorf("argv %q has no --base-branch origin/main", last)
	}
	// /o/lets-abc-x does not exist and no lets-abc-x branch is local: the HEAD Orca cut
	// cannot be read, so the resolution is omitted, never presented as confirmed
	if res.Launch.BaseResolution != nil {
		t.Errorf("an unreadable HEAD kept base_resolution %+v", res.Launch.BaseResolution)
	}
}

// Orca cuts from a commit that is neither the resolved sha nor the tip of the ref it
// was given (origin/main unmoved): the envelope never labels that commit as the
// resolved remote base (c5-F1).
func TestOpen_CutFromAnotherCommitOmitsResolution(t *testing.T) {
	repo := openRepoWithOrigin(t)
	wt := filepath.Join(t.TempDir(), "lets-abc-x")
	useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		out, err := exec.Command("git", "-C", repo, "commit-tree", "-p", "origin/main", "-m", "elsewhere", "origin/main^{tree}").Output()
		if err != nil {
			return "", "commit-tree failed", true
		}
		if err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", "lets-abc-x", wt, strings.TrimSpace(string(out))).Run(); err != nil {
			return "", "worktree add failed", true
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::` + wt + `","path":"` + wt + `","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Merge: "main"})
	if err != nil || !res.Launch.Launched || res.Launch.BaseRef != "origin/main" || res.Launch.BaseResolution != nil {
		t.Fatalf("err=%v launch=%+v, want base_ref origin/main and no base_resolution", err, res.Launch)
	}
	warned := false
	for _, s := range res.Steps {
		warned = warned || (s.Status == StepWarn && strings.Contains(s.Message, "which is not origin/main - base_resolution omitted"))
	}
	if !warned {
		t.Errorf("no warn names the foreign cut: %+v", res.Steps)
	}
}

// Orca cuts exactly the resolved commit and the ref moves only AFTER the cut (another
// fetch): HEAD is the resolution itself, so base_resolution stays, naming that sha.
func TestOpen_RefMovedAfterCutKeepsResolution(t *testing.T) {
	repo := openRepoWithOrigin(t)
	out, err := exec.Command("git", "-C", repo, "rev-parse", "origin/main").Output()
	if err != nil {
		t.Fatal(err)
	}
	resolved := strings.TrimSpace(string(out))
	wt := filepath.Join(t.TempDir(), "lets-abc-x")
	useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		if err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", "lets-abc-x", wt, "origin/main").Run(); err != nil {
			return "", "worktree add failed", true
		}
		out, err := exec.Command("git", "-C", repo, "commit-tree", "-p", "origin/main", "-m", "moved after", "origin/main^{tree}").Output()
		if err != nil {
			return "", "commit-tree failed", true
		}
		if err := exec.Command("git", "-C", repo, "update-ref", "refs/remotes/origin/main", strings.TrimSpace(string(out))).Run(); err != nil {
			return "", "update-ref failed", true
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::` + wt + `","path":"` + wt + `","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Merge: "main"})
	if err != nil || !res.Launch.Launched || res.Launch.BaseResolution == nil || res.Launch.BaseResolution.Sha != resolved {
		t.Fatalf("err=%v launch=%+v, want base_resolution.sha %s (the resolved commit Orca cut)", err, res.Launch, resolved)
	}
	for _, s := range res.Steps {
		if s.Status == StepWarn && strings.Contains(s.Message, "which is not") {
			t.Errorf("a cut from the resolved commit was reported foreign: %+v", res.Steps)
		}
	}
}

// KD10/G3: an existing local branch of that name is left to Orca as before - no
// --base-branch, no resolution (c5-F2a).
func TestOpen_ExistingBranchPassesNoBase(t *testing.T) {
	repo := openRepoWithOrigin(t)
	if out, err := exec.Command("git", "-C", repo, "branch", "lets-abc-x").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v %s", err, out)
	}
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::/o/lets-abc-x","path":"/o/lets-abc-x","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Merge: "main"})
	if err != nil || !res.Launch.Launched || res.Launch.BaseRef != "" || res.Launch.BaseResolution != nil {
		t.Fatalf("err=%v launch=%+v, want no base for an existing branch", err, res.Launch)
	}
	if last := (*calls)[len(*calls)-1].args; slices.Contains(last, "--base-branch") {
		t.Errorf("argv %q passes --base-branch for an existing branch", last)
	}
}

// A base refusal degrades: launched=false with the kind as the reason and the
// fallback command kept, and Orca is never asked to create (c5-F2b).
func TestOpen_BaseRefusalFallsBack(t *testing.T) {
	repo := mainCheckout(t)
	for _, name := range []string{"a", "b"} {
		bare := filepath.Join(t.TempDir(), name+".git")
		for _, a := range [][]string{{"init", "-q", "--bare", bare}, {"-C", repo, "remote", "add", name, bare}} {
			if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", a, err, out)
			}
		}
	}
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":true}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Merge: "main"})
	if err != nil || res.Launch.Launched || res.Launch.Reason != "remote_ambiguous" || res.Launch.FallbackCommand == "" {
		t.Fatalf("err=%v launch=%+v, want launched=false, reason remote_ambiguous, a fallback command", err, res.Launch)
	}
	for _, c := range *calls {
		if strings.HasPrefix(joined(c.args), "worktree create") {
			t.Errorf("a refused base reached orca: %q", c.args)
		}
	}
}

// noAgentOrca fakes Orca for a --no-agent open: create runs onCreate, terminal list
// returns terms. It returns the recorded calls.
func noAgentOrca(t *testing.T, terms string, onCreate func(args []string) (string, bool)) *[]fakeCall {
	t.Helper()
	if terms == "" {
		terms = `{"ok":true,"result":{"terminals":[]}}`
	}
	return useFakeOrca(t, func(args []string) (string, string, bool) {
		switch {
		case joined(args) == "status --json":
			return statusRunning, "", false
		case joined(args) == "worktree ps --json":
			return `{"ok":true,"result":{"worktrees":[]}}`, "", false
		case joined(args) == "terminal list --json":
			return terms, "", false
		case len(args) > 1 && args[0] == "worktree" && args[1] == "create":
			out, fail := onCreate(args)
			return out, "", fail
		}
		return "{}", "", false
	})
}

func createCalls(calls *[]fakeCall) [][]string {
	var out [][]string
	for _, c := range *calls {
		if len(c.args) > 1 && c.args[0] == "worktree" && c.args[1] == "create" {
			out = append(out, c.args)
		}
	}
	return out
}

func TestOpen_NoAgentArgv(t *testing.T) {
	repo := mainCheckout(t)
	calls := noAgentOrca(t, "", func([]string) (string, bool) {
		return `{"ok":true,"result":{"worktree":{"id":"r::/o/lets-abc-x","path":"/o/lets-abc-x","branch":"refs/heads/lets-abc-x"}}}`, false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
	if err != nil || !res.OK || !res.Launch.Launched || res.Launch.Path != "/o/lets-abc-x" {
		t.Fatalf("err=%v res=%+v launch=%+v", err, res, res.Launch)
	}
	want := []string{"worktree", "create", "--repo", "path:" + repo, "--name", "lets-abc-x", "--no-parent", "--setup", "skip", "--json"}
	if got := createCalls(calls); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Errorf("argv = %q\nwant  %q", got, want)
	}
}

func TestOpen_NoAgentWithPromptIsUsage(t *testing.T) {
	repo := mainCheckout(t)
	calls := noAgentOrca(t, "", func([]string) (string, bool) { return "{}", false })
	_, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "n", NoAgent: true, Prompt: "/lets:start x"})
	wantExit(t, err, ExitUsage, "usage")
	if len(*calls) != 0 {
		t.Errorf("no Orca call on a usage error, got %d", len(*calls))
	}
}

// commitRepo is a main checkout with one commit, so git worktree add works.
func commitRepo(t *testing.T) string {
	t.Helper()
	repo := mainCheckout(t)
	gitT(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i")
	return repo
}

func TestOpen_NoAgentCreateFailureReconciles(t *testing.T) {
	failCreate := `{"ok":false,"error":{"code":"internal","message":"boom"}}`

	t.Run("git shows the worktree", func(t *testing.T) {
		repo := commitRepo(t)
		wt := filepath.Join(t.TempDir(), "lets-abc-x")
		noAgentOrca(t, "", func([]string) (string, bool) {
			gitT(t, repo, "worktree", "add", "-q", "-b", "lets-abc-x", wt)
			return failCreate, true
		})
		res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
		wantExit(t, err, ExitCreateAmbiguous, "create_ambiguous")
		if res.OK || res.Launch.Launched || res.Launch.FallbackCommand != "" {
			t.Errorf("no fallback after a create git shows: %+v", res.Launch)
		}
		real, _ := filepath.EvalSymlinks(wt)
		if !fsutilSame(res.Launch.Path, real) || !strings.Contains(res.Error.Message, filepath.Base(wt)) {
			t.Errorf("the path must be named: launch=%+v error=%+v", res.Launch, res.Error)
		}
		if _, err := os.Stat(wt); err != nil {
			t.Errorf("the worktree is never auto-removed: %v", err)
		}
	})

	t.Run("git cannot be asked", func(t *testing.T) {
		repo := commitRepo(t)
		old := worktreeListProbe
		worktreeListProbe = func(context.Context, string) ([]string, error) { return nil, errors.New("git broke") }
		t.Cleanup(func() { worktreeListProbe = old })
		noAgentOrca(t, "", func([]string) (string, bool) { return failCreate, true })
		res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
		wantExit(t, err, ExitCreateAmbiguous, "create_ambiguous")
		if res.Launch.FallbackCommand != "" || !strings.Contains(res.Error.Message, "git could not be asked") {
			t.Errorf("probe failure: launch=%+v error=%+v", res.Launch, res.Error)
		}
	})

	t.Run("nothing created degrades", func(t *testing.T) {
		repo := commitRepo(t)
		noAgentOrca(t, "", func([]string) (string, bool) { return failCreate, true })
		res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
		if err != nil || !res.OK || res.Launch.Launched || !strings.Contains(res.Launch.FallbackCommand, "--no-orca") {
			t.Errorf("want the usual fallback: err=%v launch=%+v", err, res.Launch)
		}
	})
}

func fsutilSame(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra != "" && ra == rb
}

func TestOpen_NoAgentRefusesARunningAgent(t *testing.T) {
	repo := mainCheckout(t)
	p := "/o/lets-abc-x"
	terms := `{"ok":true,"result":{"terminals":[{"handle":"term_a","worktreePath":"` + p + `","title":"claude","tabId":"t","leafId":"l","agentIdentity":"claude"}]}}`
	noAgentOrca(t, terms, func([]string) (string, bool) {
		return `{"ok":true,"result":{"worktree":{"id":"r::` + p + `","path":"` + p + `","branch":"refs/heads/lets-abc-x"}}}`, false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
	wantExit(t, err, ExitAgentTerminalPresent, "agent_terminal_present")
	if res.OK || res.Launch.Launched || res.Launch.Path != p || res.Launch.FallbackCommand != "" {
		t.Errorf("launch=%+v", res.Launch)
	}

	// A plain shell in the new worktree passes.
	shell := `{"ok":true,"result":{"terminals":[{"handle":"term_b","worktreePath":"` + p + `","title":"zsh","tabId":"t","leafId":"l"}]}}`
	noAgentOrca(t, shell, func([]string) (string, bool) {
		return `{"ok":true,"result":{"worktree":{"id":"r::` + p + `","path":"` + p + `","branch":"refs/heads/lets-abc-x"}}}`, false
	})
	if res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true}); err != nil || !res.OK || !res.Launch.Launched {
		t.Errorf("a plain shell must pass: err=%v launch=%+v", err, res.Launch)
	}
}

// psAfterCreate fakes Orca whose create fails and whose `worktree ps` answers an
// empty list before the create and psAfter (failing when psFails) after it.
func psAfterCreate(t *testing.T, psAfter string, psFails bool) {
	t.Helper()
	created := false
	useFakeOrca(t, func(args []string) (string, string, bool) {
		switch {
		case joined(args) == "status --json":
			return statusRunning, "", false
		case joined(args) == "worktree ps --json":
			if !created {
				return `{"ok":true,"result":{"worktrees":[]}}`, "", false
			}
			if psFails {
				return "", "boom", true
			}
			return psAfter, "", false
		case len(args) > 1 && args[0] == "worktree" && args[1] == "create":
			created = true
			return `{"ok":false,"error":{"code":"internal","message":"boom"}}`, "", true
		}
		return "{}", "", false
	})
}

// Orca's own list is reconciled too (lets-5whww review fix 7).
func TestOpen_NoAgentCreateFailureReconcilesOrca(t *testing.T) {
	t.Run("orca lists the worktree", func(t *testing.T) {
		repo := commitRepo(t)
		psAfterCreate(t, `{"ok":true,"result":{"worktrees":[{"worktreeId":"r::/o/lets-abc-x","path":"/o/lets-abc-x","branch":"refs/heads/lets-abc-x","displayName":"lets-abc-x"}]}}`, false)
		res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
		wantExit(t, err, ExitCreateAmbiguous, "create_ambiguous")
		if res.Launch.Path != "/o/lets-abc-x" || res.Launch.FallbackCommand != "" {
			t.Errorf("launch=%+v", res.Launch)
		}
	})
	t.Run("orca list fails", func(t *testing.T) {
		repo := commitRepo(t)
		psAfterCreate(t, "", true)
		res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
		wantExit(t, err, ExitCreateAmbiguous, "create_ambiguous")
		if res.Launch.FallbackCommand != "" {
			t.Errorf("no fallback: %+v", res.Launch)
		}
	})
	t.Run("nothing anywhere degrades", func(t *testing.T) {
		repo := commitRepo(t)
		psAfterCreate(t, `{"ok":true,"result":{"worktrees":[]}}`, false)
		res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", NoAgent: true})
		if err != nil || !res.OK || res.Launch.Launched || !strings.Contains(res.Launch.FallbackCommand, "--no-orca") {
			t.Errorf("want the usual fallback: err=%v launch=%+v", err, res.Launch)
		}
	})
}
