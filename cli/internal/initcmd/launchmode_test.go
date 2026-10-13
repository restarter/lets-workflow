package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// launchAnnounce is the line every all-in launch prints before it runs (lets-d3wu3);
// it names where the mode came from (lets-gbfpd Task 16).
const launchAnnounce = "with all-in (bypassPermissions; source: <source>): no harness permission prompts in that session; LETS gates (push, PR, merge, close, peer sends) still ask"

// substringTrigger is the shape of an announce decided by a model substring test of
// the launch command - replaced by Go's class (ClassifyLaunch) everywhere.
const substringTrigger = "carries `--permission-mode bypassPermissions`"

// allInDefaults are the shapes that would make all-in a default or a fallback.
var allInDefaults = []string{"neither set -> `all-in`", "unset -> `all-in`", "falls back to `all-in`", "fallback to `all-in`", "default is `all-in`", "All-in (Recommended)", "all-in (Recommended)"}

// launchModeProblems lists every way the launch-mode contract is broken in the
// command files f (keyed worktree, team, init, start): all-in is never a default or
// a fallback, the announce line names its source and precedes every launch block
// that can carry it, every announce is driven by Go's launch class (never a
// substring test), team run shows the launch at its confirm, team create forwards
// the launch-mode flags, and the Orca run refuses a non-default mode.
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
	for key, text := range f {
		if strings.Contains(text, substringTrigger) {
			p = append(p, key+".md must announce from Go's launch class, not a substring test: "+substringTrigger)
		}
	}
	for _, need := range []string{"neither set -> `default`", "never inferred, never a fallback, never the default", "launching <name> " + launchAnnounce, "`crossSessionInbound` is never written",
		"`lets worktree info --json` -> `launch_mode`", "`launch_mode.valid=false` (a hand-edited value outside default | auto | all-in) -> `default`, plus one line", "`LETS_LAUNCH_MODE in .lets/.env` (`source=project`)"} {
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
	if cls, ann := strings.Index(t5, "when the lead's command is in the `bypass` class - after T3 the envelope's `launch_class` (orca: `orca_launch_class`), on Reopen `team_launch.class` (orca: `team_launch.orca_class`)"), strings.Index(t5, "print the all-in announce line"); cls < 0 || ann < cls {
		p = append(p, "worktree.md T5 must announce from Go's class: launch_class after T3, team_launch.class on Reopen")
	}
	if !strings.Contains(t5, "`launch_warnings` / `team_launch.warnings` as `agent_command: <warning>`") {
		p = append(p, "worktree.md T5 must print each of Go's launch warnings")
	}
	if !strings.Contains(sectionSpan(wt, "### Step T3: Team file"), "`launch_class` / `launch_warnings` for `agent_command`") {
		p = append(p, "worktree.md T3 must read team-init's launch_class")
	}
	if !strings.Contains(sectionSpan(wt, "### Reopen a team"), "run `lets worktree info --dir \"<path>\" --json` and take `team_launch`") {
		p = append(p, "worktree.md Reopen must take the lead's command and class from info's team_launch")
	}
	team := f["team"]
	r45 := sectionSpan(team, "### Step R4.5: Launch Mode")
	for _, need := range []string{"neither -> `default`", "its class `team_launch.class`", "Each `team_launch.warnings` entry", "`launch_mode.valid=false` -> `default`, plus one line", "`LETS_LAUNCH_MODE in .lets/.env` (`launch_mode.source=project`)"} {
		if !strings.Contains(r45, need) {
			p = append(p, "team.md R4.5 must resolve the workers' launch through Go (neither -> default): "+need)
		}
	}
	if !strings.Contains(r45, "the team's agent_command fixes the workers' launch (<value>); --<flag> is not applied") {
		p = append(p, "team.md R4.5 must say a typed launch-mode flag is not applied over a team file's agent_command")
	}
	// R5's plan block holds a "## Team Plan" heading, so its span runs to the gate's
	// Launch handler, not to the next heading.
	r5 := ""
	if i := strings.Index(team, "### Step R5: Confirm Launch"); i >= 0 {
		r5 = team[i:]
		if j := strings.Index(r5, "**Launch** ->"); j >= 0 {
			r5 = r5[:j]
		}
	}
	if r45i, r5i := strings.Index(team, "### Step R4.5: Launch Mode"), strings.Index(team, "### Step R5: Confirm Launch"); r45i < 0 || r5i < r45i ||
		!strings.Contains(r5, "Launch: {mode} ({source})") || !strings.Contains(r5, "Command: {agent_command}") {
		p = append(p, "team.md R5 Team Plan must show the launch mode, its source and the verbatim agent_command, resolved before the confirm")
	}
	n2 := sectionSpan(team, "### Step N2: Launch Each Worker")
	if ann, cmd := strings.Index(n2, "launching <worker_name> "+launchAnnounce), strings.Index(n2, "CMD=$(cat <<"); ann < 0 || cmd < ann {
		p = append(p, "team.md N2 must announce an all-in worker before its launch")
	}
	if cls, ann := strings.Index(n2, "When the class (Step R4.5) is `bypass`, first print one line"), strings.Index(n2, "launching <worker_name> "+launchAnnounce); cls < 0 || ann < cls {
		p = append(p, "team.md N2 must announce from Go's class (Step R4.5)")
	}
	orca := sectionSpan(team, "\n## Run (backend orca)\n")
	if !strings.Contains(orca, "orca_launch_mode_unsupported") || !strings.Contains(orca, "a launch class other than `default` (Step R4.5) is **Refused:**") ||
		!strings.Contains(orca, "`/lets:team run --backend sessions`") || strings.Contains(orca, "then continue") {
		p = append(p, "team.md Run (backend orca) must refuse a non-default launch mode (orca_launch_mode_unsupported), naming --backend sessions")
	}
	if !strings.Contains(team, "- `--backend sessions` -> the sessions backend without the Orca addon") {
		p = append(p, "team.md Guard 3 must accept --backend sessions, the way the orca refusal names")
	}
	if !strings.Contains(sectionSpan(team, "\n## Create\n"), "Pass any `--orca` / `--cmux` / `--tmux` override and any `--launch-mode <m>` / `--auto` / `--all-in` through") {
		p = append(p, "team.md Create must pass the launch-mode flags through to /lets:worktree create --team")
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
		{"the announce loses its source", "worktree", "launching <name> " + launchAnnounce, "launching <name> with all-in (bypassPermissions): no harness permission prompts in that session; LETS gates (push, PR, merge, close, peer sends) still ask", "Launch mode must state"},
		{"an invalid key is used as is", "worktree", "`launch_mode.valid=false` (a hand-edited value outside default | auto | all-in) -> `default`, plus one line", "", "Launch mode must state"},
		{"the lead launches all-in unannounced", "worktree", "print the all-in announce line", "launch", "T5 must print the all-in announce line"},
		{"T5 announces from a substring test", "worktree", "when the lead's command is in the `bypass` class - after T3 the envelope's `launch_class` (orca: `orca_launch_class`), on Reopen `team_launch.class` (orca: `team_launch.orca_class`) -", "when `agent_command` carries `--permission-mode bypassPermissions` or `--dangerously-skip-permissions`,", "not a substring test"},
		{"T5 announces without Go's class", "worktree", "when the lead's command is in the `bypass` class - after T3 the envelope's `launch_class`", "when the lead runs all-in - after T3", "T5 must announce from Go's class"},
		{"T5 drops Go's warnings", "worktree", "`launch_warnings` / `team_launch.warnings` as `agent_command: <warning>`", "the warnings", "print each of Go's launch warnings"},
		{"Reopen reads the command from the file", "worktree", "run `lets worktree info --dir \"<path>\" --json` and take `team_launch`", "read `agent_command` from the file", "Reopen must take the lead's command"},
		{"N2 announces from a substring test", "team", "When the class (Step R4.5) is `bypass`, first print one line", "When `{agent_command}` carries `--permission-mode bypassPermissions`, first print one line", "not a substring test"},
		{"R4.5 drops Go's class", "team", "its class `team_launch.class`", "its class", "R4.5 must resolve the workers' launch through Go"},
		{"R4.5 silently drops a typed flag", "team", "the team's agent_command fixes the workers' launch (<value>); --<flag> is not applied", "the team's command wins", "not applied over a team file"},
		{"R5 hides the launch mode", "team", "Launch: {mode} ({source})\n", "", "R5 Team Plan must show the launch mode"},
		{"R5 hides the command", "team", "Command: {agent_command}\n", "", "R5 Team Plan must show the launch mode"},
		{"orca continues in a non-default mode", "team", "a launch class other than `default` (Step R4.5) is **Refused:**", "a launch class other than `default` (Step R4.5) -> one line, then continue:", "must refuse a non-default launch mode"},
		{"orca refusal names no way out", "team", "run `/lets:team run --backend sessions` to launch", "use another launcher to launch", "naming --backend sessions"},
		{"Guard 3 refuses --backend sessions", "team", "- `--backend sessions` -> the sessions backend without the Orca addon", "- `--backend legacy` -> refused", "accept --backend sessions"},
		{"team create drops the launch-mode flags", "team", "Pass any `--orca` / `--cmux` / `--tmux` override and any `--launch-mode <m>` / `--auto` / `--all-in` through", "Pass any `--orca` / `--cmux` / `--tmux` override through", "Create must pass the launch-mode flags"},
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
