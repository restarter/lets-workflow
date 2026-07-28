package letsconfig

import (
	"os"
	"path/filepath"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
)

// MergedEnv overlays the project .lets/.env over the user-level ~/.lets/.env
// (project wins; only non-empty project values mask - an explicitly emptied
// project key falls through to the user default rather than deleting it).
//
// This is the ONE precedence rule for LETS_* config resolution. The SessionStart
// hook layers its LETS_MERGE_BRANCH git-fallback on top; `lets notify` reads
// LETS_LAUNCHER straight out of it. No caller may hand-roll a second reader -
// a divergent precedence between the hook's injected LETS_LAUNCHER and the one
// `lets notify` dispatches on would be silently wrong.
//
// Whitelist filtering is NOT applied here (callers decide): the hook filters at
// emit time via Names(), notifycmd reads a single known key.
//
// homeDir == "" skips the user-level layer. A missing file is not an error.
func MergedEnv(projectRoot, homeDir string) map[string]string {
	merged := map[string]string{}
	if homeDir != "" {
		userEnv, _ := readEnvFile(filepath.Join(homeDir, ".lets", ".env"))
		for k, v := range userEnv {
			if v != "" {
				merged[k] = v
			}
		}
	}
	if projectRoot != "" {
		projEnv, _ := readEnvFile(filepath.Join(projectRoot, ".lets", ".env"))
		for k, v := range projEnv {
			if v != "" {
				merged[k] = v
			}
		}
	}
	return merged
}

// readEnvFile parses the .env file at path. A missing file is not an error -
// returns an empty map.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return map[string]string{}, err
	}
	defer func() { _ = f.Close() }()
	return envfile.Parse(f)
}
