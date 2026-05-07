// Package sessionstart implements the SessionStart hook output: it writes the
// rules context followed by the LETS Config block to the provided writer.
//
// This package owns no I/O policy: callers supply the rules file path, the
// project root, and the output writer. detection helpers (DetectProjectRoot)
// are exposed for cobra wiring.
package sessionstart

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
)

// configKeys is the whitelist of LETS_* keys read from .env, in the fixed
// order they're emitted in the LETS Config block. Mirrors session-start.sh.
var configKeys = []string{
	"LETS_LANGUAGE",
	"LETS_MERGE_BRANCH",
	"LETS_PR_FLOW",
	"LETS_TRACKER",
}

// Run writes the SessionStart hook output to w:
//  1. Verbatim contents of rulesPath
//  2. Blank line
//  3. ## LETS Config block (only if projectRoot is non-empty)
//
// projectRoot may be empty - then the LETS Config block is suppressed entirely
// (matching bash behavior when `git rev-parse` returns nothing).
//
// Note: a missing rules file returns an error here (strict). The bash hook
// was permissive (`cat <missing>` writes to stderr and continues). We diverge
// intentionally: missing rules-context.md is a misconfiguration that should
// surface loudly in the hook runner, not silently produce a partial output.
func Run(w io.Writer, rulesPath, projectRoot string) error {
	rulesData, err := os.ReadFile(rulesPath)
	if err != nil {
		return fmt.Errorf("read rules file %q: %w", rulesPath, err)
	}
	if _, err := w.Write(rulesData); err != nil {
		return err
	}

	if projectRoot == "" {
		return nil
	}

	env, _ := readEnvFile(filepath.Join(projectRoot, ".lets", ".env"))

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "## LETS Config"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "LETS_PROJECT_ROOT=%s\n", projectRoot); err != nil {
		return err
	}
	for _, key := range configKeys {
		if val := env[key]; val != "" {
			if _, err := fmt.Fprintf(w, "%s=%s\n", key, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// DetectProjectRoot returns the git toplevel for the current working
// directory. Falls back to os.Getwd() if git is unavailable or the cwd is
// not in a git repo. Returns empty string if both fail.
func DetectProjectRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// readEnvFile parses the .env file at path. A missing file is not an error -
// returns empty map. This matches bash behavior where a missing .env produces
// just LETS_PROJECT_ROOT in the Config block.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return map[string]string{}, err
	}
	defer f.Close()
	return envfile.Parse(f)
}
