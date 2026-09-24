package initcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
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
	return AtomicWriteBytes(path, data, 0o644)
}

// AtomicWriteBytes writes via tmp + rename. Preserves existing file mode if
// the target exists, else uses defaultMode. Sync before rename for crash-safety.
//
// Exported for reuse by sibling packages (updatecmd writes the rules copy and
// the latest-release cache with it) - keeps "write a primary artifact" atomic
// everywhere, not just inside initcmd. Delegates to fsutil.AtomicWriteBytes.
func AtomicWriteBytes(path string, data []byte, defaultMode os.FileMode) error {
	return fsutil.AtomicWriteBytes(path, data, defaultMode)
}
