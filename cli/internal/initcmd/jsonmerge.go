package initcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SetStatusLine mutates settings.json to set:
//   - .statusLine = {type:"command", command:"lets statusline"}
//
// Preserves all other fields. Atomic write (tmp + rename). Refuses if existing
// statusLine is StatusLineForeign (caller should check before invoking).
//
// Backup behavior: writes a single <path>.bak before mutation; subsequent
// runs overwrite that single .bak. (No timestamped 3-rotation scheme - the
// scrap-then-restore use case for one-shot init is just "the file before
// I touched it", and git already covers history beyond that.)
func SetStatusLine(path string) error {
	settings, err := readSettingsJSON(path)
	if err != nil {
		return err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	state := detectStatusLineField(settings)
	if state == StatusLineForeign {
		return fmt.Errorf("settings.json has foreign statusLine - refusing to overwrite")
	}

	// Single-file backup before mutation (overwrites previous .bak)
	if data, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", data, 0o600)
	}

	settings["statusLine"] = map[string]any{
		"type":    "command",
		"command": "lets statusline",
	}

	return atomicWriteJSON(path, settings)
}

// atomicWriteJSON marshals m with 2-space indent and writes via tmp+rename,
// preserving existing file mode if any.
func atomicWriteJSON(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteBytes(path, data, 0o644)
}

// atomicWriteBytes writes via tmp + rename. Preserves existing file mode if
// the target exists. Sync before rename for crash-safety.
func atomicWriteBytes(path string, data []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".lets-init-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
