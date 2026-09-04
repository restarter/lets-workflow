package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The invariant this pins (lets-2ntow):
//
// detect-task returns a task id that a caller hands straight to a tracker verb,
// and on beads that verb resolves to a command the MODEL types - `bd show <id>` -
// so the id is an unquoted value in a second shell. The id can come off disk:
// `.lets/` is shared by every worktree, and a TRACKED `.task-<slug>` file
// materialises on checkout, so a hostile branch can plant one.
//
// This used to be handled by a sentence inside the `branch=<ref>` bullet telling
// every other path to sanitize what it received - and the path the sentence named
// by name, Step 1.5, did not. The fix was structural: candidates go through ONE
// gate, and the gate is the only thing that emits an id. These tests fail the
// build if that shape erodes, which is the difference between an invariant and a
// convention.
const (
	detectTaskSkill = "skills/detect-task/SKILL.md"
	idGateEmit      = `echo "TASK_ID=`
	idGateClass     = `*[!A-Za-z0-9._-]*`
)

// fenceBodies returns the body of every fenced block in src, whatever the tag.
// Tag-agnostic on purpose: an emit hidden in a ```sh or untagged fence would
// still be a second exit.
func fenceBodies(src string) []string {
	var out []string
	lines := strings.Split(src, "\n")
	inFence := false
	var cur []string
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			if inFence {
				out = append(out, strings.Join(cur, "\n"))
				cur = nil
			}
			inFence = !inFence
			continue
		}
		if inFence {
			cur = append(cur, ln)
		}
	}
	return out
}

// TestDetectTaskIdGate: exactly one block emits an id, and that block validates
// the character class. One exit, guarded.
func TestDetectTaskIdGate(t *testing.T) {
	path := filepath.Join(pluginDir(t), detectTaskSkill)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", detectTaskSkill, err)
	}

	var emitters []string
	for _, body := range fenceBodies(string(raw)) {
		if strings.Contains(body, idGateEmit) {
			emitters = append(emitters, body)
		}
	}

	if len(emitters) != 1 {
		t.Fatalf("detect-task must have exactly ONE block that emits an id; found %d.\n"+
			"A second exit is a second answer: it is how Step 1.5 came to return an unvalidated\n"+
			"id while a rule three sections away said it must not. Route the new path through\n"+
			"the id gate instead of giving it its own %s.", len(emitters), idGateEmit)
	}

	if !strings.Contains(emitters[0], idGateClass) {
		t.Errorf("the id gate emits without checking %s.\n"+
			"That class IS the gate - an id reaches a tracker verb as an unquoted value in a\n"+
			"shell the model types, so an unchecked emit hands that shell whatever was on disk.", idGateClass)
	}
}

// TestDetectTaskIsSoleIdSource: no other command or skill emits TASK_ID.
// The guarantee in detect-task's Output is only worth relying on while detect-task
// is the one thing that can produce the value.
func TestDetectTaskIsSoleIdSource(t *testing.T) {
	root := pluginDir(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if filepath.ToSlash(rel) == detectTaskSkill {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, body := range fenceBodies(string(raw)) {
			if strings.Contains(body, idGateEmit) {
				t.Errorf("%s emits an id: callers READ detect-task's TASK_ID, they do not produce one.\n"+
					"Producing it elsewhere reintroduces an unvalidated source and voids the guarantee\n"+
					"detect-task's Output makes on behalf of every consumer.", filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
