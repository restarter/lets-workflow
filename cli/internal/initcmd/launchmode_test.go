package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// launchAnnounce is the line every all-in launch prints before it runs (lets-d3wu3).
const launchAnnounce = "with all-in (bypassPermissions): no harness permission prompts in that session; LETS gates (push, PR, merge, close, peer sends) still ask"

// allInDefaults are the shapes that would make all-in a default or a fallback.
var allInDefaults = []string{"neither set -> `all-in`", "unset -> `all-in`", "falls back to `all-in`", "fallback to `all-in`", "default is `all-in`", "All-in (Recommended)", "all-in (Recommended)"}

// launchModeProblems lists every way the launch-mode contract is broken in the
// command files f (keyed worktree, team, init, start): all-in is never a default or
// a fallback, and the announce line precedes every launch block that can carry it.
func launchModeProblems(f map[string]string) []string {
	var p []string
	for key, text := range f {
		for _, bad := range allInDefaults {
			if strings.Contains(text, bad) {
				p = append(p, key+": all-in must never be a default or a fallback: "+bad)
			}
		}
	}
	wt := f["worktree"]
	for _, need := range []string{"neither set -> `default`", "never inferred, never a fallback, never the default", "launching <name> " + launchAnnounce, "`crossSessionInbound` is never written"} {
		if !strings.Contains(wt, need) {
			p = append(p, "worktree.md Launch mode must state: "+need)
		}
	}
	if first, by := strings.Index(wt, launchAnnounce), strings.Index(wt, `--command "claude --permission-mode bypassPermissions`); first < 0 || by < first {
		p = append(p, "worktree.md must state the announce line before any bypassPermissions launch command")
	}
	t5 := sectionSpan(wt, "### Step T5: Launch the lead")
	if ann, cmd := strings.Index(t5, "print the all-in announce line"), strings.Index(t5, "CMD=$(cat <<"); ann < 0 || cmd < ann {
		p = append(p, "worktree.md T5 must print the all-in announce line before the lead's launch")
	}
	team := f["team"]
	n2 := sectionSpan(team, "### Step N2: Launch Each Worker")
	if ann, cmd := strings.Index(n2, "launching <worker_name> "+launchAnnounce), strings.Index(n2, "CMD=$(cat <<"); ann < 0 || cmd < ann || !strings.Contains(n2, "neither -> `default`") {
		p = append(p, "team.md N2 must resolve the workers' launch mode (neither -> default) and announce an all-in worker before its launch")
	}
	for _, need := range []string{"carries `--permission-mode bypassPermissions` or `--dangerously-skip-permissions`, print the all-in announce line"} {
		if !strings.Contains(t5, need) {
			p = append(p, "worktree.md T5 must announce on either bypass form: "+need)
		}
	}
	if !strings.Contains(n2, "carries `--permission-mode bypassPermissions` or `--dangerously-skip-permissions`, first print one line") {
		p = append(p, "team.md N2 must announce on either bypass form")
	}
	if !strings.Contains(n2, "the team's agent_command fixes the workers' launch (<value>); --<flag> is not applied") {
		p = append(p, "team.md N2 must say a typed launch-mode flag is not applied over a team file's agent_command")
	}
	if !strings.Contains(sectionSpan(team, "\n## Run (backend orca)\n"), "orca_launch_mode_unsupported") {
		p = append(p, "team.md Run (backend orca) must name orca_launch_mode_unsupported for a non-default mode")
	}
	if !strings.Contains(f["init"], `{ label: "Default (Recommended)"`) || !strings.Contains(f["init"], "--launch-mode={LAUNCH_MODE}") {
		p = append(p, "init.md must recommend the default launch mode and pass --launch-mode")
	}
	if !strings.Contains(f["init"], "binds `$LAUNCH_MODE` to that CURRENT value - empty when unset, never `default`") || strings.Contains(f["init"], "`default` when unset") {
		p = append(p, "init.md Keep current must keep an unset LETS_LAUNCH_MODE unset - bind the current value, empty when unset, never default")
	}
	if !strings.Contains(f["start"], "LETS_LAUNCH_MODE is unset") {
		p = append(p, "start.md main mode must say when LETS_LAUNCH_MODE is unset")
	}
	return p
}

func TestLaunchModeContract(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	files := map[string]string{"worktree": read("worktree.md"), "team": read("team.md"), "init": read("init.md"), "start": read("start.md")}
	for _, problem := range launchModeProblems(files) {
		t.Error(problem)
	}
	mutants := []struct{ name, key, old, repl, want string }{
		{"all-in becomes the fallback", "worktree", "neither set -> `default`", "neither set -> `all-in`", "never be a default or a fallback"},
		{"init recommends all-in", "init", `{ label: "Default (Recommended)", `, `{ label: "Default", `, "recommend the default"},
		{"init labels all-in recommended", "init", `{ label: "All-in", `, `{ label: "All-in (Recommended)", `, "never be a default or a fallback"},
		{"team worker launches all-in unannounced", "team", "first print one line: \"launching <worker_name> " + launchAnnounce + "\"", "launch it", "announce an all-in worker"},
		{"the lead launches all-in unannounced", "worktree", "print the all-in announce line", "launch", "T5 must print the all-in announce line"},
		{"T5 misses the dangerously flag", "worktree", "carries `--permission-mode bypassPermissions` or `--dangerously-skip-permissions`, print", "carries `--permission-mode bypassPermissions`, print", "either bypass form"},
		{"N2 misses the dangerously flag", "team", "carries `--permission-mode bypassPermissions` or `--dangerously-skip-permissions`, first", "carries `--permission-mode bypassPermissions`, first", "either bypass form"},
		{"N2 silently drops a typed flag", "team", "the team's agent_command fixes the workers' launch (<value>); --<flag> is not applied", "the team's command wins", "not applied over a team file"},
		{"keep current materializes default", "init", "\"(unset = default)\" when it is absent, and binds `$LAUNCH_MODE` to that CURRENT value - empty when unset, never `default` (`--launch-mode=` keeps the key unset)", "`default` when unset", "Keep current must keep an unset"},
		{"start stops naming an unset key", "start", "LETS_LAUNCH_MODE is unset", "launch mode", "LETS_LAUNCH_MODE is unset"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(files[m.key], m.old) {
				t.Fatalf("mutant cannot apply: %q is not in %s", m.old, m.key)
			}
			mutated := map[string]string{}
			for k, v := range files {
				mutated[k] = v
			}
			mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, 1)
			for _, problem := range launchModeProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
