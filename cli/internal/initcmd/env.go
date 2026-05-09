package initcmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

// EnvActionKind enumerates RegenerateEnv outcomes.
type EnvActionKind string

const (
	EnvCreated     EnvActionKind = "created"
	EnvSkip        EnvActionKind = "skip"
	EnvRegenerated EnvActionKind = "regenerated"
)

// EnvAction is the outcome of RegenerateEnv (or initial creation).
type EnvAction struct {
	Kind        EnvActionKind `json:"kind"`
	Path        string        `json:"path"`
	PrevVersion string        `json:"prev_version,omitempty"`
	NewVersion  string        `json:"new_version,omitempty"`
	ChangedKeys []string      `json:"changed_keys"`
	BackupPath  string        `json:"backup_path,omitempty"`
	// PreservedLines kept for legacy JSON consumers; always 0 in regenerate model.
	PreservedLines int `json:"preserved_lines"`
}

// RegenerateEnv writes a fresh canonical .env at path, preserving user values
// from the existing file (if any) and merging CLI-flag overrides from prefs.
// Foreign keys (non-LETS_*) are appended at the bottom under a marker.
//
// Behavior:
//   - reads existing .env (if present) to extract LETS_* values + foreign keys
//   - if any prefs.* CLI flag was set explicitly, it overrides the value from .env
//   - if file already in sync (version matches AND no value changes) → returns
//     EnvSkip without writing
//   - otherwise writes canonical Header + LETS_ENV_VERSION + Keys block +
//     foreign tail; atomic write via tmp+rename; backup at <path>.bak
//
// Known limitation: existing LETS_* values are read via envfile.Parse, which
// truncates values at 200 bytes (see envfile.MaxValueLen). Canonical LETS_*
// values are short by design (Language: "Ukrainian", Branch: "main", etc.) so
// this is safe in practice. Foreign keys are extracted via raw line walker
// and preserved verbatim regardless of length.
//
// Reusable: lets-hdrdr.3 (/lets:update) imports this directly.
func RegenerateEnv(path string, prefs Prefs) (EnvAction, error) {
	action := EnvAction{Path: path, NewVersion: version.Version, ChangedKeys: []string{}}

	var existingValues map[string]string
	var existingData []byte
	var foreignBlock string
	fileExists := false
	if data, err := os.ReadFile(path); err == nil {
		fileExists = true
		existingData = data

		// envfile.Parse signature is (io.Reader) -> (map[string]string, error).
		// Parse truncates values >200 bytes per its godoc; LETS canonical keys
		// are short by design so this is safe in practice. Foreign keys are
		// extracted via raw-line walker below to preserve any long values
		// verbatim.
		parsed, perr := envfile.Parse(bytes.NewReader(data))
		if perr != nil {
			return action, fmt.Errorf("parse existing .env: %w", perr)
		}
		existingValues = parsed
		action.PrevVersion = existingValues[letsconfig.VersionKeyName]
		foreignBlock = extractForeignKeys(data)
	} else if !os.IsNotExist(err) {
		return action, fmt.Errorf("read existing .env: %w", err)
	}

	// Resolve final prefs: CLI flag values override .env values.
	finalPrefs := mergePrefs(prefs, existingValues)
	action.ChangedKeys = diffKeys(existingValues, finalPrefs)

	// Skip path: file exists, version matches, no value changes → no-op
	if fileExists && action.PrevVersion == version.Version && len(action.ChangedKeys) == 0 {
		action.Kind = EnvSkip
		return action, nil
	}

	// Backup before mutating (only when there was a prior file)
	if fileExists {
		bakPath := path + ".bak"
		_ = os.WriteFile(bakPath, existingData, 0o600)
		action.BackupPath = bakPath
	}

	// Render canonical body + append foreign keys
	body := renderEnv(finalPrefs)
	if foreignBlock != "" {
		body = append(body, []byte("\n# User-added keys (preserved across upgrades)\n")...)
		body = append(body, []byte(foreignBlock)...)
	}

	if err := os.MkdirAll(strings.TrimSuffix(path, "/.env"), 0o755); err != nil {
		// best-effort; atomicWriteBytes will retry/fail with concrete error
		_ = err
	}
	if err := atomicWriteBytes(path, body, 0o644); err != nil {
		return action, fmt.Errorf("write .env: %w", err)
	}

	if !fileExists {
		action.Kind = EnvCreated
	} else {
		action.Kind = EnvRegenerated
	}
	return action, nil
}

// readEnvVersion returns the LETS_ENV_VERSION value from the .env at path,
// or empty string if the file is missing/unreadable/lacks the marker.
func readEnvVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	values, err := envfile.Parse(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return values[letsconfig.VersionKeyName]
}

// mergePrefs returns prefs filled from existingValues for any field where prefs
// is zero-value. CLI flag values (non-empty in prefs) win over existingValues.
//
// Tracker is asymmetric: it has no CLI flag (no --tracker), so prefs.Tracker
// is always non-empty (cobra wrapper fills from defaults). For Tracker we
// invert the precedence — existing value wins over default — so user
// customization in .env is preserved across regen.
func mergePrefs(prefs Prefs, existing map[string]string) Prefs {
	result := prefs
	if result.Language == "" {
		result.Language = existing["LETS_LANGUAGE"]
	}
	if result.MergeBranch == "" {
		result.MergeBranch = existing["LETS_MERGE_BRANCH"]
	}
	if result.PRFlow == "" {
		result.PRFlow = existing["LETS_PR_FLOW"]
	}
	if existingTracker := existing["LETS_TRACKER"]; existingTracker != "" {
		result.Tracker = existingTracker
	}
	return result
}

// diffKeys returns user-facing keys whose values differ between old map and
// new Prefs.
func diffKeys(old map[string]string, new Prefs) []string {
	var changed []string
	if old["LETS_LANGUAGE"] != new.Language {
		changed = append(changed, "LETS_LANGUAGE")
	}
	if old["LETS_MERGE_BRANCH"] != new.MergeBranch {
		changed = append(changed, "LETS_MERGE_BRANCH")
	}
	if old["LETS_PR_FLOW"] != new.PRFlow {
		changed = append(changed, "LETS_PR_FLOW")
	}
	if old["LETS_TRACKER"] != new.Tracker {
		changed = append(changed, "LETS_TRACKER")
	}
	if len(changed) == 0 {
		return []string{}
	}
	return changed
}

// extractForeignKeys returns the substring of the original .env containing only
// non-LETS_* (and non-LETS_ENV_VERSION) key-value lines. Comments adjacent to
// foreign keys are dropped (we re-render canonical comments around our own
// keys; foreign keys go bare). Preserves source-order and verbatim values.
func extractForeignKeys(data []byte) string {
	canonical := map[string]bool{
		letsconfig.VersionKeyName: true,
	}
	for _, k := range letsconfig.Names() {
		canonical[k] = true
	}
	var buf bytes.Buffer
	for _, line := range bytes.Split(data, []byte("\n")) {
		s := strings.TrimSpace(string(line))
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		eq := strings.Index(s, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(s[:eq])
		if !canonical[key] {
			buf.WriteString(s)
			buf.WriteString("\n")
		}
	}
	return buf.String()
}
