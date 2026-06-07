package statuslinecmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/initcmd"
)

// Flags is the desired absolute appearance state (idempotent set, not a toggle).
type Flags struct {
	Light   bool
	Compact bool
	NoTip   bool
	NoDir   bool
	NoTask  bool
}

// Any reports whether at least one flag is set.
func (f Flags) Any() bool {
	return f.Light || f.Compact || f.NoTip || f.NoDir || f.NoTask
}

// suffix renders the flags as the trailing args of the statusLine command.
func (f Flags) suffix() string {
	var p []string
	if f.Light {
		p = append(p, "--light")
	}
	if f.Compact {
		p = append(p, "--compact")
	}
	if f.NoTip {
		p = append(p, "--no-tip")
	}
	if f.NoDir {
		p = append(p, "--no-dir")
	}
	if f.NoTask {
		p = append(p, "--no-task")
	}
	return strings.Join(p, " ")
}

// command renders the full statusLine command string for these flags.
func (f Flags) command() string {
	if s := f.suffix(); s != "" {
		return "lets statusline " + s
	}
	return "lets statusline"
}

func (f Flags) appearance() *Appearance {
	return &Appearance{Light: f.Light, Compact: f.Compact, NoTip: f.NoTip, NoDir: f.NoDir, NoTask: f.NoTask}
}

// settingsPath is the personal (gitignored) settings file the appearance is
// persisted to — never the tracked .claude/settings.json, so a developer's
// --light choice does not get forced onto collaborators.
func settingsPath(root string) string {
	return filepath.Join(root, ".claude", "settings.local.json")
}

// readSettings reads settings.local.json into a map. Missing/empty file -> empty
// map (not an error); malformed JSON -> error (caller refuses to mutate).
func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// currentCommand extracts statusLine.command from a settings map ("" if absent).
func currentCommand(settings map[string]any) string {
	sl, _ := settings["statusLine"].(map[string]any)
	if sl == nil {
		return ""
	}
	cmd, _ := sl["command"].(string)
	return cmd
}

// parseCommand maps a persisted command back to Flags. ok=false means the
// command is not a recognized "lets statusline [known flags]" form (foreign).
func parseCommand(cmd string) (Flags, bool) {
	fields := strings.Fields(cmd)
	if len(fields) < 2 || fields[0] != "lets" || fields[1] != "statusline" {
		return Flags{}, false
	}
	var f Flags
	for _, a := range fields[2:] {
		switch a {
		case "--light":
			f.Light = true
		case "--compact":
			f.Compact = true
		case "--no-tip":
			f.NoTip = true
		case "--no-dir":
			f.NoDir = true
		case "--no-task":
			f.NoTask = true
		default:
			return Flags{}, false
		}
	}
	return f, true
}

// Show reports the current persisted appearance without writing.
func Show(root string) (Result, error) {
	res := newResult(root)
	path := settingsPath(root)
	res.SettingsPath = path

	settings, err := readSettings(path)
	if err != nil {
		e := ErrMalformed(path, err)
		res.OK, res.Error = false, errInfo(e)
		res.Steps = append(res.Steps, Step{Status: StepErr, Message: "settings.local.json is not valid JSON"})
		return res, e
	}
	cmd := currentCommand(settings)
	if cmd == "" {
		res.OK = true
		res.Command = "lets statusline"
		res.Appearance = (Flags{}).appearance()
		res.Steps = append(res.Steps, Step{Status: StepSkip, Message: "no statusLine in settings.local.json (using defaults)"})
		return res, nil
	}
	f, ok := parseCommand(cmd)
	if !ok {
		e := ErrForeign(cmd)
		res.OK, res.Error, res.Command = false, errInfo(e), cmd
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "foreign statusLine command present"})
		return res, e
	}
	res.OK, res.Command, res.Appearance = true, cmd, f.appearance()
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: "read current appearance"})
	return res, nil
}

// Apply persists want to settings.local.json, replacing only the statusLine key
// and preserving all other keys. A foreign existing command is refused unless
// force is set.
func Apply(root string, want Flags, force bool) (Result, error) {
	res := newResult(root)
	path := settingsPath(root)
	res.SettingsPath = path

	settings, err := readSettings(path)
	if err != nil {
		e := ErrMalformed(path, err)
		res.OK, res.Error = false, errInfo(e)
		res.Steps = append(res.Steps, Step{Status: StepErr, Message: "settings.local.json is not valid JSON"})
		return res, e
	}
	if cur := currentCommand(settings); cur != "" {
		if _, ok := parseCommand(cur); !ok && !force {
			e := ErrForeign(cur)
			res.OK, res.Error, res.Command = false, errInfo(e), cur
			res.Steps = append(res.Steps, Step{Status: StepErr, Message: "refusing to overwrite a foreign statusLine (use --force)"})
			return res, e
		}
	}

	settings["statusLine"] = map[string]any{"type": "command", "command": want.command()}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		e := ErrFilesystem("marshal settings", err)
		res.OK, res.Error = false, errInfo(e)
		return res, e
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e := ErrFilesystem("create .claude dir", err)
		res.OK, res.Error = false, errInfo(e)
		return res, e
	}
	if err := initcmd.AtomicWriteBytes(path, data, 0o644); err != nil {
		e := ErrFilesystem("write "+path, err)
		res.OK, res.Error = false, errInfo(e)
		res.Steps = append(res.Steps, Step{Status: StepErr, Message: "could not write settings.local.json"})
		return res, e
	}
	res.OK, res.Command, res.Appearance, res.Changed = true, want.command(), want.appearance(), true
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: "persisted appearance to settings.local.json"})
	return res, nil
}
