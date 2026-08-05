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
// let a variant-tagged block escape the scan entirely (S11). It now also selects
// WHICH tier of the scan applies to a fence's body (see the walk below).
var bashFence = regexp.MustCompile("^```(?:bash|sh|shell|zsh|console)\\b")

// bdBareWord matches `bd` as a standalone token. Applied to every NON-bash fence
// and to whole .workflow.js assets, where there is no legitimate bd at all: those
// bodies are prompts, and the neutral-verb platform exists precisely so a subagent
// never learns one tracker's dialect. This tier is what sees the forms bdCommand
// cannot by construction - `Run: bd stats`, `Use bd commands (bd show, bd
// comments)`, `{suggested bd create command}` - none of which sit in command
// position (lets-x1rnx).
var bdBareWord = regexp.MustCompile(`(^|[^A-Za-z0-9_])bd([^A-Za-z0-9_]|$)`)

// allowedExecutableBd: repo-relative file -> substrings of lines permitted to
// carry executable bd inside a ```bash fence. EMPTY BY DESIGN (lets-x1rnx): the
// last exception - detect-task's merge-branch liveness probe - now goes through
// the neutral `show` verb like everything else. A new entry here means a command
// body learned to speak one tracker's dialect again; TestAllowlistShipsEmpty
// makes re-adding one an explicit, reviewable test change rather than a silent
// line in the commit that needed it.
var allowedExecutableBd = map[string][]string{}

// TestNoExecutableBdInCommandBodies enforces the neutral-verb invariant: no bd
// survives in a command/skill body or a workflow asset. Promotes the one-time grep
// sweep into a durable CI guard so a future edit can't silently re-introduce a
// literal bd verb-call (the failure mode the neutral-verb rewrite, lets-5d48z, set
// out to eliminate).
//
// Scope is deliberately wider than the name's "Executable" suggests, and wider than
// it was before lets-x1rnx. Subagent-embedded bd - backlog's explorer and brainstorm
// prompts - used to be described here as "naturally out of scope (the Cat-C
// carve-out)". That carve-out is gone: the orchestrator now injects tracker data and
// no subagent gets tracker access, so a bd inside a prompt fence is a defect, not an
// exception. It went unseen for three releases precisely because it sat in a plain
// fence in a file the walk did not read.
func TestNoExecutableBdInCommandBodies(t *testing.T) {
	root := pluginDir(t)
	var scanned, scannedWorkflows int
	for _, sub := range []string{"commands", "skills"} {
		err := filepath.Walk(filepath.Join(root, sub), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			isWorkflow := strings.HasSuffix(path, ".workflow.js")
			if info.IsDir() || (!strings.HasSuffix(path, ".md") && !isWorkflow) {
				return nil
			}
			if isWorkflow {
				scannedWorkflows++
			} else {
				scanned++
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			// A .workflow.js asset is prompt strings and orchestration end to end - no
			// fences to track, and no legitimate bd anywhere. It is a committed COPY of
			// a command's prompt, which is how backlog's kept instructing subagents to
			// run bd long after the command itself stopped: the walk used to be .md-only,
			// so nothing could see it.
			if isWorkflow {
				for i, line := range strings.Split(string(data), "\n") {
					if bdBareWord.MatchString(line) {
						t.Errorf("%s:%d bd in a workflow asset (the orchestrator injects tracker data; agents get none of their own): %s", rel, i+1, strings.TrimSpace(line))
					}
				}
				return nil
			}
			// Fence STACK, not a toggle. A TAGGED line (```bash, ```lets-tracker, ```json)
			// always OPENS - including inside another fence, which is the only way a bash
			// block nested in an output template is ever reached. A BARE ``` closes the
			// innermost fence, or opens a plain one when nothing is open.
			//
			// Depth matters because a toggle-on-every-fence machine consumes the nested
			// opener AS a closer: the nested body is then classified as outside any fence
			// and skipped by both tiers, and the parity stays inverted for the rest of the
			// file. That is a coverage REGRESSION - the pre-lets-x1rnx machine, which only
			// ever tracked bash openers, caught a bd inside `worktree.md`'s nested block and
			// the toggle version did not. Live examples: commands/worktree.md:165-174,
			// commands/github-pr.md:956-969.
			//
			// Known limitation, unchanged: a heredoc BODY carrying a ``` line pops a fence
			// early - acceptable, heredoc-carried markdown is data, not commands to run.
			var fences []bool // one entry per open fence; true = bash-family
			for i, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") {
					switch {
					case len(trimmed) > 3: // tagged -> always an opener, even when nested
						fences = append(fences, bashFence.MatchString(trimmed))
					case len(fences) > 0: // bare inside something -> closes the innermost
						fences = fences[:len(fences)-1]
					default: // bare at top level -> opens a plain fence
						fences = append(fences, false)
					}
					continue
				}
				if len(fences) == 0 {
					continue
				}
				bashKind := fences[len(fences)-1]
				// The `#` skip is for BASH COMMENTS. In any other fence `#` is a markdown
				// heading - ~140 of them sit inside output templates and Task( prompts, and
				// skipping those would blind the bare-word tier to `### Suggested bd create`.
				if bashKind && strings.HasPrefix(trimmed, "#") {
					continue
				}
				// Two tiers. In a bash fence the prose-vs-command distinction is real, so
				// only command-position bd counts. In any other fence - a Task( prompt, a
				// lets-tracker block, an output template - no bd is legitimate, and the
				// forms that actually regress are not in command position.
				if bashKind {
					if !bdCommand.MatchString(line) {
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
					continue
				}
				if !bdBareWord.MatchString(line) {
					continue
				}
				// Deliberately a different message: this tier fires inside output templates
				// (text the model PRINTS) and inside Task( prompts, where a lets-tracker
				// block is exactly what you cannot use - verb resolution is orchestrator-only.
				t.Errorf("%s:%d bd named in a non-bash fence - a prompt or output template must not teach one tracker's dialect; the orchestrator injects tracker data instead: %s", rel, i+1, trimmed)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Per-source counters, not one total: a combined count stays non-zero when ONE
	// source silently drops out, which is how the .workflow.js blindness would come
	// back (a renamed suffix, a relocated skill dir) with CI green.
	if scanned == 0 {
		t.Fatal("scanned 0 markdown files under plugins/lets - test wiring broken")
	}
	if scannedWorkflows == 0 {
		t.Fatal("scanned 0 *.workflow.js assets - the walk extension regressed; workflow assets are committed COPIES of command prompts and go unguarded without it")
	}
}

// TestAllowlistShipsEmpty makes re-adding an exception an explicit test change.
// Without it "the allowlist ships empty" is a comment: the commit that needed an
// exemption could append its own and leave CI green, which is exactly how the one
// entry this map used to hold survived three releases.
func TestAllowlistShipsEmpty(t *testing.T) {
	if len(allowedExecutableBd) != 0 {
		t.Errorf("allowedExecutableBd ships empty by design (lets-x1rnx); got %d entr(ies): %v", len(allowedExecutableBd), allowedExecutableBd)
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

// neutralStatuses is the canonical status vocabulary a ```lets-tracker line may
// name: required open/in_progress/closed plus optional in_review/blocked. Whether
// a given adapter CARRIES an optional one is a runtime check the command makes
// against that adapter's `## Neutral statuses` section; this is the spelling gate.
var neutralStatuses = map[string]bool{
	"open": true, "in_progress": true, "closed": true, "in_review": true, "blocked": true,
}

// statusArg captures a literal status= value. A placeholder (`status=<neutral>`)
// does not match the class and is deliberately skipped rather than false-failed.
var statusArg = regexp.MustCompile(`\bstatus=([A-Za-z_]+)`)

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
				// Argument VALUES went unpinned until lets-x1rnx - which is how a
				// `status=in_review` reached a reviewed plan without anyone noticing that
				// beads enumerates only open/in_progress/closed/blocked and would reject
				// it at runtime, mid-/lets:done.
				if m := statusArg.FindStringSubmatch(trimmed); m != nil && !neutralStatuses[m[1]] {
					t.Errorf("%s:%d ```lets-tracker line uses status=%s - not a neutral status (open|in_progress|closed|in_review|blocked)", rel, i+1, m[1])
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
