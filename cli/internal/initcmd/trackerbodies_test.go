package initcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// pluginDir resolves <repo>/plugins/lets from this test file's path
// (cli/internal/initcmd/trackerbodies_test.go -> up 3 -> repo root).
func pluginDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "plugins", "lets")
}

// bdCommand matches bd invoked as a SHELL COMMAND: at the start of a (trimmed)
// line, after a pipe, after && / ;, or inside $(...). It deliberately does NOT
// match the bare word "bd" in prose ("the bd task", "via bd"), so the sweep keys
// on lines the model would RUN, not every mention. Only applied inside ```bash
// fences (neutral ```lets-tracker blocks and prose are never scanned).
var bdCommand = regexp.MustCompile(`(?:^|\||;|&&|\$\()\s*bd\s`)

// allowedExecutableBd: repo-relative file -> substrings of lines permitted to
// carry executable bd inside a ```bash fence. The ONLY sanctioned exception is
// the detect-task merge-branch liveness probe - an allowlisted beads-only read
// gated on LETS_TRACKER=beads (D7 / lets-rules "Tracker Adapters"). Everything
// else in a command/skill BODY must be a neutral ```lets-tracker block.
var allowedExecutableBd = map[string][]string{
	filepath.Join("skills", "detect-task", "SKILL.md"): {
		`bd show "$FILE_TASK" --json`,
	},
}

// TestNoExecutableBdInCommandBodies enforces the обезлічування invariant: no
// executable bd survives in a command/skill body's ```bash fence outside the
// allowlist. Promotes the one-time grep sweep into a durable CI guard so a future
// edit can't silently re-introduce a literal bd verb-call (the failure mode the
// neutral-verb rewrite, lets-5d48z, set out to eliminate). Subagent-embedded bd
// (e.g. backlog's explorer/brainstorm prompts) lives in plain ``` fences, not
// ```bash, so it is naturally out of scope (the Cat-C carve-out).
func TestNoExecutableBdInCommandBodies(t *testing.T) {
	root := pluginDir(t)
	var scanned int
	for _, sub := range []string{"commands", "skills"} {
		err := filepath.Walk(filepath.Join(root, sub), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			scanned++
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			inBash := false
			for i, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") {
					switch {
					case !inBash && (trimmed == "```bash" || trimmed == "```sh"):
						inBash = true
					case inBash:
						inBash = false
					}
					continue
				}
				if !inBash || !bdCommand.MatchString(line) {
					continue
				}
				allowed := false
				for _, frag := range allowedExecutableBd[rel] {
					if strings.Contains(line, frag) {
						allowed = true
						break
					}
				}
				if !allowed {
					t.Errorf("%s:%d executable bd in a command body (use a ```lets-tracker block): %s", rel, i+1, trimmed)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned == 0 {
		t.Fatal("scanned 0 markdown files under plugins/lets - test wiring broken")
	}
}
