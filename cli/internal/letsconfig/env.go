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
	eachLayer(projectRoot, homeDir, func(_ string, env map[string]string) {
		for k, v := range env {
			if v != "" {
				merged[k] = v
			}
		}
	})
	return merged
}

// MergedEnvSource is MergedEnv's value for one key plus the layer that supplied
// it: "project" (.lets/.env), "user" (~/.lets/.env) or "unset" (neither has a
// non-empty value; value ""). It walks the same layers as MergedEnv (eachLayer),
// so the two never disagree.
func MergedEnvSource(projectRoot, homeDir, key string) (value, source string) {
	value, source = "", "unset"
	eachLayer(projectRoot, homeDir, func(layer string, env map[string]string) {
		if v := env[key]; v != "" {
			value, source = v, layer
		}
	})
	return value, source
}

// eachLayer calls fn for every config layer, lowest precedence first: the
// user-level ~/.lets/.env (layer "user", skipped when homeDir is "") and then the
// project .lets/.env (layer "project", skipped when projectRoot is ""). A later
// non-empty value wins - the one precedence rule MergedEnv documents.
func eachLayer(projectRoot, homeDir string, fn func(layer string, env map[string]string)) {
	if homeDir != "" {
		userEnv, _ := readEnvFile(filepath.Join(homeDir, ".lets", ".env"))
		fn("user", userEnv)
	}
	if projectRoot != "" {
		projEnv, _ := readEnvFile(filepath.Join(projectRoot, ".lets", ".env"))
		fn("project", projEnv)
	}
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

// ResolvedEnv is MergedEnv plus the LETS_MERGE_BRANCH fallback: when neither file
// supplies it, defaultBranch(projectRoot) (the repo's origin default branch) when
// the function is non-nil and returns a name, else "main". A derived name is capped
// at envfile.MaxValueLen like every parsed value (branch names are attacker-chosen
// in cloned repos). This is the ONE resolver for callers that need the merge branch:
// the SessionStart hook, worktree adopt/remove/sweep, the statusline. Callers on a
// render path pass nil so they never fork git. letsconfig gains no gitutil import:
// callers pass a closure such as
//
//	func(r string) string { return gitutil.DefaultBranch(r, 2*time.Second) }
//
// LETS_GATES is always present and normalized (GatesLevel: empty or unknown ->
// GatesDefault), so no caller can see a level that lowers the gates. A caller
// that needs the raw value (the hook's unknown-value Notice) reads MergedEnv.
func ResolvedEnv(projectRoot, homeDir string, defaultBranch func(root string) string) map[string]string {
	merged := MergedEnv(projectRoot, homeDir)
	merged["LETS_GATES"] = GatesLevel(merged["LETS_GATES"])
	if merged["LETS_MERGE_BRANCH"] != "" {
		return merged
	}
	b := ""
	if defaultBranch != nil {
		b = defaultBranch(projectRoot)
	}
	if b == "" {
		b = "main"
	}
	if len(b) > envfile.MaxValueLen {
		b = b[:envfile.MaxValueLen]
	}
	merged["LETS_MERGE_BRANCH"] = b
	return merged
}
