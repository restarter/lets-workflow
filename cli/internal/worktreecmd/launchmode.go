//go:build unix

package worktreecmd

import (
	"fmt"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

// LaunchCommand renders base (the agent command, e.g. `claude`) for a launch mode
// (lets-d3wu3) - the one renderer of the three levels: default -> base as is,
// auto -> base + ` --permission-mode auto`, all-in -> base + ` --permission-mode
// bypassPermissions`. "" is default. An unknown mode is an error, never a guess:
// all-in is selected only by name.
func LaunchCommand(mode, base string) (string, error) {
	switch mode {
	case "", "default":
		return base, nil
	case "auto":
		return base + " --permission-mode auto", nil
	case "all-in":
		return base + " --permission-mode bypassPermissions", nil
	}
	return "", fmt.Errorf("launch mode %q is not one of %v", mode, letsconfig.LaunchModes)
}

// Launch classes ClassifyLaunch reports. The harness holds a cross-session message
// only when exactly one side runs in the bypass class; default and auto are one
// class, bypass the other (lets-d3wu3).
const (
	LaunchClassDefault = "default"
	LaunchClassAuto    = "auto"
	LaunchClassBypass  = "bypass"
)

// Warnings ClassifyLaunch reports alongside the class.
const (
	LaunchWarnSettingsOverride    = "settings_override"     // any --settings: it can set a permission mode the class does not show
	LaunchWarnCrossSessionInbound = "cross_session_inbound" // the command names crossSessionInbound, which LETS never writes
)

// ClassifyLaunch reads the permission class a launch command starts in - the one
// classifier the markdown announces from (lets-gbfpd Task 16), never a substring
// test in a prompt. bypass: --permission-mode bypassPermissions (either spelling),
// --dangerously-skip-permissions, --allow-dangerously-skip-permissions, or a
// --settings value naming bypassPermissions; auto: --permission-mode auto (either
// spelling); else default. The command is split like a shell would (quotes,
// backslashes) and a quoted argument holding a whole command (`sh -c '...'`) is
// classified too; any bypass spelling wins over auto. warnings: settings_override
// for any --settings, cross_session_inbound when the command names
// crossSessionInbound. An empty command is default.
func ClassifyLaunch(cmd string) (class string, warnings []string) {
	bypass, auto, settings := classifyFields(shellFields(cmd), 0)
	if settings {
		warnings = append(warnings, LaunchWarnSettingsOverride)
	}
	if strings.Contains(cmd, "crossSessionInbound") {
		warnings = append(warnings, LaunchWarnCrossSessionInbound)
	}
	switch {
	case bypass:
		return LaunchClassBypass, warnings
	case auto:
		return LaunchClassAuto, warnings
	}
	return LaunchClassDefault, warnings
}

// maxLaunchNesting bounds how deep a quoted sub-command is classified.
const maxLaunchNesting = 4

func classifyFields(fields []string, depth int) (bypass, auto, settings bool) {
	mode := func(v string) {
		switch v {
		case "bypassPermissions":
			bypass = true
		case "auto":
			auto = true
		}
	}
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		next := ""
		if i+1 < len(fields) {
			next = fields[i+1]
		}
		switch {
		case f == "--permission-mode":
			mode(next)
		case strings.HasPrefix(f, "--permission-mode="):
			mode(strings.TrimPrefix(f, "--permission-mode="))
		case f == "--dangerously-skip-permissions", f == "--allow-dangerously-skip-permissions":
			bypass = true
		case f == "--settings":
			settings = true
			if strings.Contains(next, "bypassPermissions") {
				bypass = true
			}
		case strings.HasPrefix(f, "--settings="):
			settings = true
			if strings.Contains(f, "bypassPermissions") {
				bypass = true
			}
		case depth < maxLaunchNesting && strings.ContainsAny(f, " \t"):
			b, a, s := classifyFields(shellFields(f), depth+1)
			bypass, auto, settings = bypass || b, auto || a, settings || s
		}
	}
	return bypass, auto, settings
}

// shellFields splits s into words the way a POSIX shell would for a plain command:
// whitespace separates, and outside quotes the control operators ; & | ( ) < >
// also end the current word and are themselves dropped (so a flag glued to one,
// `--flag;` or `(claude`, is a word of its own); single quotes are literal, double
// quotes keep their content (a backslash escapes ", \, $ and a backtick there), and
// operators inside either quote stay literal; a bare backslash escapes the next
// character. An unterminated quote runs to the end. No expansion happens.
func shellFields(s string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			inWord = true
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				cur.WriteString(s[i+1:])
				i = len(s)
				continue
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			inWord = true
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\"\\$`", s[i+1]) >= 0 {
					i++
				}
				cur.WriteByte(s[i])
			}
		case c == '\\' && i+1 < len(s):
			inWord = true
			i++
			cur.WriteByte(s[i])
		case c == ' ' || c == '\t' || c == '\n' || strings.IndexByte(";&|()<>", c) >= 0:
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			inWord = true
			cur.WriteByte(c)
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// LaunchModeInfo is LETS_LAUNCH_MODE as `lets worktree info` reports it: the value
// as written, where it came from (project | user | unset - letsconfig.LaunchModeSetting)
// and whether it is one of default | auto | all-in. An unset value is valid (it
// means default); an invalid one is treated as default and never injected into
// LETS Config.
type LaunchModeInfo struct {
	Value  string `json:"value"`
	Source string `json:"source"`
	Valid  bool   `json:"valid"`
}

// TeamLaunch is how the team's lead is launched, read from the team file that
// claims this worktree: agent_command and orca_agent as recorded, each with its
// ClassifyLaunch class and warnings. The markdown announces from these fields.
type TeamLaunch struct {
	AgentCommand string   `json:"agent_command"`
	Class        string   `json:"class"`
	Warnings     []string `json:"warnings"`
	OrcaAgent    string   `json:"orca_agent"`
	OrcaClass    string   `json:"orca_class"`
	OrcaWarnings []string `json:"orca_warnings"`
}

// launchModeInfo resolves LETS_LAUNCH_MODE for the project at mainRoot.
func launchModeInfo(mainRoot, homeDir string) *LaunchModeInfo {
	v, src := letsconfig.LaunchModeSetting(mainRoot, homeDir)
	return &LaunchModeInfo{Value: v, Source: src, Valid: v == "" || letsconfig.ValidLaunchMode(v)}
}
