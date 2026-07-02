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
// line, after a pipe / ; / & (covers && and |&) / backtick, inside $(...), after
// a command-position keyword (then/do/else/xargs/exec/env/time), or after a
// line-leading VAR=x env-assignment prefix. It deliberately does NOT match the
// bare word "bd" in prose ("the bd task", "via bd"), so the sweep keys on lines
// the model would RUN, not every mention. Bash comment lines are skipped by the
// caller. Only applied inside bash-family fences (neutral ```lets-tracker
// blocks and prose are never scanned).
var bdCommand = regexp.MustCompile("(?:^|[|;&`]|\\$\\(|\\b(?:then|do|else|elif|xargs|exec|time|env)\\s+|^\\s*\\w+=\\S*\\s+)\\s*bd\\s")

// bashFence matches a bash-family fence opener incl. tag variants (```shell,
// ```zsh, ```console, or an info string after the tag) - an exact-match gate
// let a variant-tagged block escape the scan entirely (S11).
var bashFence = regexp.MustCompile("^```(?:bash|sh|shell|zsh|console)\\b")

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

// TestNoExecutableBdInCommandBodies enforces the neutral-verb invariant: no
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
					case !inBash && bashFence.MatchString(trimmed):
						inBash = true
					case inBash:
						// Known limitation: a heredoc BODY containing a ``` line closes
						// the fence early (unscanned tail) - acceptable; heredoc-carried
						// markdown is data, not commands the model runs.
						inBash = false
					}
					continue
				}
				if !inBash || strings.HasPrefix(trimmed, "#") || !bdCommand.MatchString(line) {
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

// canonicalVerbTokens is the callable neutral vocabulary for a ```lets-tracker
// block line's leading token (ready/stats split into their callable forms).
// TestTrackerAdapters_VerbVocabInSync owns the same canon on the adapter/rules
// side; this is the BODY side of the contract.
var canonicalVerbTokens = map[string]bool{
	"create": true, "show": true, "comment-add": true, "set-status": true,
	"close": true, "comment-list": true, "list-by-status": true, "search": true,
	"ready": true, "stats": true, "label": true, "assignee": true, "set-field": true,
}

// TestTrackerBlockVerbsResolvable is the inverse of
// TestNoExecutableBdInCommandBodies (branch-review S10): every ```lets-tracker
// invocation line must START with a canonical verb, or it silently fails to
// resolve at runtime - a typo (`comment-adds`, `set_status`) passes the no-bd
// guard and every adapter contract test. Multi-line inline body="..." values
// are skipped via double-quote parity tracking (their lines are data, not
// verbs); comment-only and blank lines are ignored.
func TestTrackerBlockVerbsResolvable(t *testing.T) {
	root := pluginDir(t)
	var blocks int
	for _, sub := range []string{"commands", "skills"} {
		err := filepath.Walk(filepath.Join(root, sub), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			inBlock, inString := false, false
			for i, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") {
					if !inBlock && trimmed == "```lets-tracker" {
						inBlock = true
						blocks++
					} else if inBlock {
						inBlock, inString = false, false
					}
					continue
				}
				if !inBlock {
					continue
				}
				if inString { // inside a multi-line body="..." value
					if strings.Count(line, "\"")%2 == 1 {
						inString = false
					}
					continue
				}
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				token := strings.Fields(trimmed)[0]
				if !canonicalVerbTokens[token] {
					t.Errorf("%s:%d ```lets-tracker line starts with %q - not a canonical verb (would not resolve against any adapter)", rel, i+1, token)
				}
				if strings.Count(line, "\"")%2 == 1 {
					inString = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if blocks == 0 {
		t.Fatal("found 0 ```lets-tracker blocks under plugins/lets - test wiring broken")
	}
}
