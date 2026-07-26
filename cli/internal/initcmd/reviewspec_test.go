package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The SPEC injection (lets-yobxe) lives in THREE places by necessity: review.md's
// prompt template, check.md's inline lens, and review.workflow.js (the Dynamic
// Workflow runtime forbids import/filesystem, so the block cannot be shared).
// Unlike decide()/computeVerdict() - untestable JS logic kept in sync by
// discipline - this one is a shared PROMPT STRING, so it can be pinned.
//
// A checklist grep cannot do this job: a multi-file `grep -c` exits 0 when ANY
// single file matches, so "2 of 3 sites populated" reads as green. Same reason
// trackerbodies_test.go exists - a one-time grep sweep is not durable.

// specSites maps a plugin-relative file to the substrings it MUST contain.
var specSites = map[string][]string{
	filepath.Join("commands", "review.md"): {
		"--- BEGIN SPEC",
		"--- END SPEC ---",
		"SCOPE vs SPEC:",
		// The cap is the whole point of the change: the observed failure was a
		// BLOCKER-severity false positive, not a miss.
		"cap\nit at [SUGGESTION]",
	},
	filepath.Join("commands", "check.md"): {
		"--- BEGIN SPEC",
		"--- END SPEC ---",
		"SCOPE vs SPEC:",
		"cap any scope / dead-code finding at [SUGGESTION]",
	},
	filepath.Join("skills", "review-workflow", "review.workflow.js"): {
		"--- BEGIN SPEC",
		"--- END SPEC ---",
		"SCOPE vs SPEC:",
		"cap it at SUGGESTION",
	},
}

func TestReviewSpecBlocksInSync(t *testing.T) {
	root := pluginDir(t)
	for rel, needles := range specSites {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		body := string(data)
		for _, n := range needles {
			if !strings.Contains(body, n) {
				t.Errorf("%s: missing SPEC-block text %q - the three sites have drifted apart", rel, n)
			}
		}
	}
}

// TestReviewWorkflowInterpolatesSpecBlocks guards the failure mode a
// presence-check misses entirely: a block that is DEFINED but never spliced
// into a prompt. The skeptic block is the one that matters most - it verifies
// findings, and decide() turns its real=false into a DROP, so a skeptic that
// silently lost its spec would refute planned-work findings it cannot judge.
func TestReviewWorkflowInterpolatesSpecBlocks(t *testing.T) {
	path := filepath.Join(pluginDir(t), "skills", "review-workflow", "review.workflow.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	for _, decl := range []string{"const specBlock =", "const specBlockSkeptic =", "const treeBlock ="} {
		if !strings.Contains(body, decl) {
			t.Errorf("review.workflow.js: missing declaration %q", decl)
		}
	}

	// The review prompt takes specBlock; the skeptic prompt takes the narrower
	// specBlockSkeptic. Both take treeBlock. Interpolation is ${name}.
	for _, use := range []string{"${specBlock}", "${specBlockSkeptic}", "${treeBlock}"} {
		if strings.Count(body, use) == 0 {
			t.Errorf("review.workflow.js: %s is declared but never interpolated into a prompt", use)
		}
	}

	// The reviewer's block must NOT reach the skeptic: it says "cap it at
	// SUGGESTION", and a skeptic's only lever is real=false, which decide()
	// maps to a DROP for a SUGGESTION - turning a cap into a silent delete.
	skeptic := body[strings.Index(body, "function skepticPrompt"):]
	if strings.Contains(skeptic, "${specBlock}") {
		t.Error("review.workflow.js: skepticPrompt interpolates the REVIEWER's specBlock; it must use specBlockSkeptic (a skeptic cannot set a tier, so 'cap at SUGGESTION' becomes a drop)")
	}
}

// TestReviewNeverCreatesWorktree pins the design boundary the plan review
// established: /lets:review reviews where it was launched. An earlier revision
// materialized the PR in a throwaway detached worktree, which introduced
// symlink exfiltration into public PR comments, path traversal through
// `worktree remove --force`, and orphaned trees. The only permitted mention is
// the prohibition itself.
func TestReviewNeverCreatesWorktree(t *testing.T) {
	path := filepath.Join(pluginDir(t), "commands", "review.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "git worktree") {
			continue
		}
		if strings.Contains(line, "NEVER create a git worktree") {
			continue
		}
		t.Errorf("commands/review.md:%d mentions `git worktree` outside the prohibition - review must never create one: %s", i+1, strings.TrimSpace(line))
	}
}
