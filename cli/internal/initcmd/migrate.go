package initcmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MigrateStatuslineSh deletes .lets/statusline.sh if it's our shim or legacy
// bash. Leaves foreign scripts alone with a notice. Returns human-readable
// action description ("" if nothing done).
func MigrateStatuslineSh(projectRoot string) (string, error) {
	path := filepath.Join(projectRoot, ".lets", "statusline.sh")
	state := detectStatuslineSh(path)
	switch state {
	case StatuslineAbsent:
		return "", nil
	case StatuslineCurrentShim, StatuslineLegacyBash:
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return ".lets/statusline.sh removed (managed by lets binary now)", nil
	case StatuslineForeign:
		return ".lets/statusline.sh appears customized - left alone, please review", nil
	}
	return "", nil
}

// MigrateYamlToEnv reads legacy .lets/config.yaml, writes .lets/.env, deletes
// the yaml. Also handles the orphan case where both .env and yaml coexist
// (mixed-state cleanup): yaml deleted, .env left as-is.
//
// Return contract:
//   - msg!="", did=true, err=nil → migration ran (StepMigrate by caller)
//   - msg!="", did=false, err=nil → soft warning surfaced to caller (e.g. yaml
//     present but unreadable due to permissions); rendered as StepWarn
//   - msg=="", did=false, err=nil → no-op (yaml absent)
//   - err!=nil → hard failure (parse/write/remove error); caller aborts
func MigrateYamlToEnv(projectRoot string) (string, bool, error) {
	yamlPath := filepath.Join(projectRoot, ".lets", "config.yaml")
	envPath := filepath.Join(projectRoot, ".lets", ".env")

	if _, err := os.Stat(yamlPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}

	if _, err := os.Stat(envPath); err == nil {
		// Orphan yaml lying alongside fresh .env - remove it. .env is
		// authoritative; yaml is a leftover from a partial earlier state.
		if err := os.Remove(yamlPath); err != nil {
			return "", false, err
		}
		// did=true so the caller renders this as StepMigrate (a migration-
		// related action, even if not a strict yaml→env conversion).
		return "removed legacy .lets/config.yaml (superseded by existing .env)", true, nil
	}

	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		// File exists but unreadable (permissions, IO). Surface as warn so
		// the user knows their legacy config wasn't migrated silently.
		return fmt.Sprintf(".lets/config.yaml present but unreadable (%v) - migration skipped", err), false, nil
	}
	prefs, err := parseLegacyYaml(yamlData)
	if err != nil {
		return "", false, fmt.Errorf("legacy yaml parse: %w", err)
	}
	envBytes := renderEnv(prefs)
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		return "", false, err
	}
	if err := AtomicWriteBytes(envPath, envBytes, 0o644); err != nil {
		return "", false, err
	}
	if err := os.Remove(yamlPath); err != nil {
		return "", false, err
	}
	return ".lets/config.yaml -> .lets/.env (yaml deleted)", true, nil
}

var (
	yamlLanguageRe    = regexp.MustCompile(`(?m)^language:\s*(\S+?)\s*(?:#.*)?$`)
	yamlMergeBranchRe = regexp.MustCompile(`(?m)^merge-branch:\s*(\S+?)\s*(?:#.*)?$`)
	yamlGithubRe      = regexp.MustCompile(`(?m)^github:\s*(\S+?)\s*(?:#.*)?$`)
)

// parseLegacyYaml parses the 3-key legacy yaml format. Returns Prefs with
// defaults applied for missing keys. Refuses block scalars (`|` literal,
// `>` folded) on any tracked key - they're not in the legacy format and
// would let attacker-controlled content sneak in newlines that break the
// per-line regex contract.
//
// Tolerates:
//   - UTF-8 BOM at file start
//   - CRLF line endings (normalized to LF before regex)
//   - Inline `# comment` after the value (stripped, then trimmed)
//   - Surrounding single/double quotes around the value
func parseLegacyYaml(data []byte) (Prefs, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	body := string(data)

	for _, key := range []string{"language", "merge-branch", "github"} {
		if strings.Contains(body, key+": |") || strings.Contains(body, key+": >") {
			return Prefs{}, fmt.Errorf("block scalar (|/>) not supported in legacy yaml key %q", key)
		}
	}

	p := Prefs{Language: "English", MergeBranch: "main", PRFlow: "local", Tracker: "beads"}

	// Sanitize: each captured value must be safe.
	allowedRe := regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

	if m := yamlLanguageRe.FindStringSubmatch(body); m != nil {
		v := strings.Trim(m[1], `"'`)
		if allowedRe.MatchString(v) {
			p.Language = v
		}
	}
	if m := yamlMergeBranchRe.FindStringSubmatch(body); m != nil {
		v := strings.Trim(m[1], `"'`)
		if allowedRe.MatchString(v) {
			p.MergeBranch = v
		}
	}
	if m := yamlGithubRe.FindStringSubmatch(body); m != nil {
		v := strings.ToLower(strings.Trim(m[1], `"'`))
		switch v {
		case "true", "yes", "1", "github":
			p.PRFlow = "github"
		case "bitbucket":
			p.PRFlow = "bitbucket"
		case "false", "no", "0", "local":
			p.PRFlow = "local"
		}
	}
	return p, nil
}

// EnsureGitignore appends entries to .gitignore if absent. Ensures trailing
// newline before append. Idempotent.
//
// Existing-entry detection skips blank lines and `#` comment lines so we
// don't accidentally treat a line like "# .lets/" (commented out) as a real
// entry that satisfies the requirement.
func EnsureGitignore(projectRoot string, entries []string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		existing[line] = true
	}
	var toAppend []string
	for _, entry := range entries {
		if !existing[entry] {
			toAppend = append(toAppend, entry)
		}
	}
	if len(toAppend) == 0 {
		return nil
	}
	var buf []byte
	buf = append(buf, data...)
	if len(buf) > 0 && buf[len(buf)-1] != '\n' {
		buf = append(buf, '\n')
	}
	for _, e := range toAppend {
		buf = append(buf, []byte(e+"\n")...)
	}
	return AtomicWriteBytes(path, buf, 0o644)
}
