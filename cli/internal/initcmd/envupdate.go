package initcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

// EnvActionKind enumerates .env outcomes.
type EnvActionKind string

const (
	EnvCreated EnvActionKind = "created" // first-time write
	EnvSkip    EnvActionKind = "skip"    // existed, no --force-env, OR --force-env with no actual changes
	EnvUpdated EnvActionKind = "updated" // --force-env applied surgical patch
)

// EnvAction describes what happened to .lets/.env in a Run().
type EnvAction struct {
	Kind EnvActionKind `json:"kind"`
	Path string        `json:"path"`
	// ChangedKeys lists the LETS_* keys whose VALUES changed (or were
	// appended for previously-missing keys).
	ChangedKeys []string `json:"changed_keys"`
	// PreservedLines counts lines that survived without value change:
	// comments, blank lines, foreign keys, AND canonical LETS_* keys whose
	// existing value already matched the desired value. Lines whose values
	// were updated are NOT counted (they're in ChangedKeys).
	PreservedLines int `json:"preserved_lines"`
	// BackupPath set to ".lets/.env.bak" when --force-env updated existing file.
	BackupPath string `json:"backup_path,omitempty"`
}

// keyLineRe matches `LETS_FOO=value` lines only. Foreign keys (e.g. user's
// GITHUB_TOKEN, LETS_=oops) pass through as raw bytes — never parsed, so
// quote/escape edge cases are irrelevant.
//
// Regex requires at least one alpha char after LETS_ to reject empty-key
// pathological cases (LETS_=val).
var keyLineRe = regexp.MustCompile(`^(LETS_[A-Z][A-Z0-9_]*)=(.*)$`)

// UpdateEnvKeys reads envPath, replaces values for the canonical LETS_* keys
// (per letsconfig.Keys) with values from p, preserves all other lines
// (comments, blanks, foreign keys, key order). Keys absent from the file are
// appended at the end with their canonical comment from letsconfig.Keys.
//
// If all desired values match existing values AND no canonical keys were missing,
// returns EnvAction{Kind: EnvSkip} without touching the file (preserves mtime,
// no git churn).
//
// Writes a single .env.bak in the same directory (overwriting any prior backup)
// before rewriting. On .env write failure, .bak is preserved as recovery.
// BackupPath is set in the returned EnvAction.
func UpdateEnvKeys(envPath string, p Prefs) (EnvAction, error) {
	original, err := os.ReadFile(envPath)
	if err != nil {
		return EnvAction{}, fmt.Errorf("read .env: %w", err)
	}

	// Single source of truth for Prefs↔Key wiring (mirrors renderEnv).
	desired := p.AsValues()

	seen := map[string]bool{}
	changed := []string{}
	preserved := 0
	var out bytes.Buffer

	scanner := bufio.NewScanner(bytes.NewReader(original))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)

		// Comments / blank lines: preserve verbatim
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
			out.WriteByte('\n')
			preserved++
			continue
		}

		m := keyLineRe.FindStringSubmatch(line)
		if m == nil {
			// Foreign key (no LETS_ prefix) or non-key line: pass through
			out.WriteString(line)
			out.WriteByte('\n')
			preserved++
			continue
		}

		key, oldVal := m[1], m[2]
		if want, ok := desired[key]; ok {
			seen[key] = true
			if oldVal != want {
				changed = append(changed, key)
			} else {
				preserved++ // value unchanged — line survived without modification
			}
			fmt.Fprintf(&out, "%s=%s\n", key, want)
		} else {
			// LETS_FUTURE_KEY etc. — preserved unchanged
			out.WriteString(line)
			out.WriteByte('\n')
			preserved++
		}
	}
	if err := scanner.Err(); err != nil {
		return EnvAction{}, fmt.Errorf("scan .env: %w", err)
	}

	// Append missing canonical keys, each with their letsconfig.Keys comment
	missingKeys := []letsconfig.Key{}
	for _, k := range letsconfig.Keys {
		if !seen[k.Name] {
			missingKeys = append(missingKeys, k)
		}
	}
	if len(missingKeys) > 0 {
		out.WriteString("\n# (added by /lets:init)\n")
		for _, k := range missingKeys {
			fmt.Fprintf(&out, "\n# %s\n", k.Comment)
			fmt.Fprintf(&out, "%s=%s\n", k.Name, desired[k.Name])
			changed = append(changed, k.Name)
		}
	}

	// No-op detection: no value changes AND no missing keys → skip without rewriting
	if len(changed) == 0 {
		return EnvAction{Kind: EnvSkip, Path: envPath, ChangedKeys: []string{}, PreservedLines: preserved}, nil
	}

	// Write .env.bak (single, overwriting any previous)
	backupPath := envPath + ".bak"
	if err := atomicWriteBytes(backupPath, original, 0o644); err != nil {
		return EnvAction{}, fmt.Errorf("write .env.bak: %w", err)
	}

	if err := atomicWriteBytes(envPath, out.Bytes(), 0o644); err != nil {
		// Backup PRESERVED for user recovery. Surface BackupPath in returned
		// EnvAction so caller can tell the user where their data is.
		return EnvAction{
			Kind: EnvUpdated, Path: envPath,
			ChangedKeys: changed, PreservedLines: preserved,
			BackupPath: backupPath,
		}, fmt.Errorf("write .env (backup preserved at %s): %w", backupPath, err)
	}

	return EnvAction{
		Kind:           EnvUpdated,
		Path:           envPath,
		ChangedKeys:    changed,
		PreservedLines: preserved,
		BackupPath:     backupPath,
	}, nil
}
