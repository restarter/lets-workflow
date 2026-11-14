package initcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecuteStep6Record runs execute.md's Step 6 block (lets-ejts9) in a temp repo:
// it records the plan Step 2 loaded, labels an unconfirmed glob, measures the range
// from a named boundary, and never prints a count without one.
func TestExecuteStep6Record(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	src := readPlugin(t, "commands/execute.md")
	fences := bashFences(region(t, src, "execute.md", "## Step 6: Record Completion", "## Rules"))
	if len(fences) != 1 {
		t.Fatalf("Step 6 holds %d bash fences, want 1", len(fences))
	}
	block := fences[0]

	git := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	setup := func(t *testing.T) (dir, base string) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		git(t, dir, "init", "-q", "-b", "feature/lets-x-y")
		c := []string{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m"}
		git(t, dir, append(c, "base")...)
		base = git(t, dir, "rev-parse", "HEAD")
		git(t, dir, append(c, "one")...)
		git(t, dir, append(c, "two")...)
		for _, d := range []string{".lets/plans", ".lets/sessions"} {
			if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, ".lets/plans/2026-01-01-0000-lets-x-plan.md"), []byte("# p\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir, base
	}
	run := func(t *testing.T, dir, planUsed, runBase string) string {
		t.Helper()
		s := strings.NewReplacer("{PLAN_USED}", planUsed, "{RUN_BASE}", runBase, "{LETS_MERGE_BRANCH}", "main", "<task-id>", "lets-x").Replace(block)
		cmd := exec.Command("bash", "-c", s)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "TASK_ID=lets-x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Step 6 block: %v\n%s", err, out)
		}
		b, err := os.ReadFile(filepath.Join(dir, ".lets/cache/exec-complete-lets-x.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	t.Run("plan used and task start", func(t *testing.T) {
		dir, base := setup(t)
		task := "task: lets-x\nstart: " + base + "\n"
		if err := os.WriteFile(filepath.Join(dir, ".lets/sessions/.task-feature-lets-x-y"), []byte(task), 0o600); err != nil {
			t.Fatal(err)
		}
		got := run(t, dir, "/p/$HOME/plan.md", "") // a $ in the path stays literal
		for _, want := range []string{"Plan: /p/$HOME/plan.md\n", "Commits: 2 (since the task start, "} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})
	t.Run("lost plan is named unconfirmed, run base fallback", func(t *testing.T) {
		dir, base := setup(t)
		got := run(t, dir, "", base)
		for _, want := range []string{"lets-x-plan.md (newest plan for the task, not confirmed as the one that ran)", "Commits: 2 (since the run start (first chunk's base), "} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})
	t.Run("no boundary prints no number", func(t *testing.T) {
		dir, _ := setup(t)
		got := run(t, dir, "/p/x.md", "")
		if !strings.Contains(got, "Commits: unknown - no task boundary\n") {
			t.Errorf("want the no-boundary line in:\n%s", got)
		}
	})
}
