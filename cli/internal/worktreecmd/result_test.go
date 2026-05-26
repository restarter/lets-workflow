//go:build unix

package worktreecmd

import (
	"encoding/json"
	"testing"
)

// TestResult_SchemaContract pins the JSON shapes for all 4 subcommand result
// envelopes (Create / Remove / List / Info) plus the bare Envelope core they
// embed. Bumping SchemaVersion or renaming/removing a field must touch this
// test on purpose. Mirrors initcmd.TestResult_SchemaContract and
// updatecmd.TestResult_SchemaContract — closing the gap that left worktreecmd
// the only --json package without a contract guard.
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion changed to %d - update consumers (commands/worktree.md) and this test", SchemaVersion)
	}

	// Envelope core: every subcommand's JSON MUST include these top-level keys
	// regardless of success / failure. Order in `requiredCore` matches result.go.
	requiredCore := []string{"schema_version", "ok", "subcommand", "project_root", "steps"}

	t.Run("create_success", func(t *testing.T) {
		r := &CreateResult{
			Envelope: Envelope{
				SchemaVersion: SchemaVersion,
				OK:            true,
				Subcommand:    "create",
				ProjectRoot:   "/p",
				Steps:         []Step{{Status: StepOK, Message: "ok"}},
			},
			Worktree: &WorktreeInfo{
				Name: "foo", Path: "/p/.worktrees/foo", Branch: "worktree-foo",
				BranchMode: "created", BaseRef: "main",
				LetsSymlinked: true, BeadsSymlinked: true,
			},
			NextSteps: &NextSteps{AbsolutePath: "/p/.worktrees/foo"},
		}
		m := marshalToMap(t, r)
		requireKeys(t, m, requiredCore...)
		requireKeys(t, m, "worktree", "next_steps")
		wt, ok := m["worktree"].(map[string]any)
		if !ok {
			t.Fatalf("worktree shape = %#v", m["worktree"])
		}
		requireKeys(t, wt, "name", "path", "branch", "branch_mode", "base_ref", "lets_symlinked", "beads_symlinked")
		ns, ok := m["next_steps"].(map[string]any)
		if !ok {
			t.Fatalf("next_steps shape = %#v", m["next_steps"])
		}
		// next_steps.absolute_path is the LOAD-BEARING field — commands/worktree.md
		// reads it to tell the user where to `cd`. Renaming it without a schema
		// bump silently breaks every existing markdown skill.
		requireKeys(t, ns, "absolute_path")
	})

	t.Run("create_failure_with_rollback", func(t *testing.T) {
		r := &CreateResult{
			Envelope: Envelope{
				SchemaVersion: SchemaVersion,
				OK:            false,
				Subcommand:    "create",
				ProjectRoot:   "/p",
				Steps:         []Step{},
				Error:         &ErrorInfo{Kind: "post_create_failed", Message: "symlink"},
			},
			Rollback: &RollbackInfo{Attempted: true, Succeeded: false, Residual: []string{"/p/.worktrees/foo"}},
		}
		m := marshalToMap(t, r)
		requireKeys(t, m, requiredCore...)
		requireKeys(t, m, "error", "rollback")
		errObj, ok := m["error"].(map[string]any)
		if !ok {
			t.Fatalf("error shape = %#v", m["error"])
		}
		requireKeys(t, errObj, "kind", "message")
		rb, ok := m["rollback"].(map[string]any)
		if !ok {
			t.Fatalf("rollback shape = %#v", m["rollback"])
		}
		requireKeys(t, rb, "attempted", "succeeded", "residual")
	})

	t.Run("remove_success", func(t *testing.T) {
		r := &RemoveResult{
			Envelope: Envelope{
				SchemaVersion: SchemaVersion,
				OK:            true,
				Subcommand:    "remove",
				ProjectRoot:   "/p",
				Steps:         []Step{},
			},
			Removed: &RemovedInfo{Name: "foo", Path: "/p/.worktrees/foo", Branch: "worktree-foo"},
		}
		m := marshalToMap(t, r)
		requireKeys(t, m, requiredCore...)
		requireKeys(t, m, "removed")
		rem, ok := m["removed"].(map[string]any)
		if !ok {
			t.Fatalf("removed shape = %#v", m["removed"])
		}
		requireKeys(t, rem, "name", "path", "branch", "branch_deleted", "had_uncommitted_changes", "forced")
	})

	t.Run("list_success", func(t *testing.T) {
		r := &ListResult{
			Envelope: Envelope{
				SchemaVersion: SchemaVersion,
				OK:            true,
				Subcommand:    "list",
				ProjectRoot:   "/p",
				Steps:         []Step{},
			},
			Worktrees: []WorktreeInfo{{Name: "foo", Path: "/p/.worktrees/foo", Branch: "worktree-foo"}},
			Main:      &WorktreeInfo{Path: "/p", Branch: "main", IsMain: true},
		}
		m := marshalToMap(t, r)
		requireKeys(t, m, requiredCore...)
		requireKeys(t, m, "worktrees", "main")
		wts, ok := m["worktrees"].([]any)
		if !ok || len(wts) != 1 {
			t.Fatalf("worktrees shape = %#v", m["worktrees"])
		}
	})

	t.Run("info_in_worktree", func(t *testing.T) {
		r := &InfoResult{
			Envelope: Envelope{
				SchemaVersion: SchemaVersion,
				OK:            true,
				Subcommand:    "info",
				ProjectRoot:   "/p",
				Steps:         []Step{},
			},
			InWorktree: true,
			Worktree:   &WorktreeInfo{Name: "foo", Path: "/p/.worktrees/foo", Branch: "worktree-foo"},
			MainRoot:   "/p",
		}
		m := marshalToMap(t, r)
		requireKeys(t, m, requiredCore...)
		requireKeys(t, m, "in_worktree", "main_root", "worktree")
	})
}

// TestStepStatus_Enum pins the four Step.Status values. Renaming a status
// (ok→success, warn→warning) would silently break markdown skills that
// branch on `step.status == "warn"`.
func TestStepStatus_Enum(t *testing.T) {
	cases := map[string]string{
		"ok":    StepOK,
		"skip":  StepSkip,
		"warn":  StepWarn,
		"error": StepErr,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("Step status constant = %q, want %q (consumers branch on the string literal)", got, want)
		}
	}
}

func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func requireKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing JSON key %q in object: %v", k, mapKeys(m))
		}
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
