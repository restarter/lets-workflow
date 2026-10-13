package letsconfig

import "path/filepath"

// LaunchModes is the set of accepted LETS_LAUNCH_MODE values (lets-d3wu3): how a
// session LETS launches (a worker, a team's lead) starts. default = plain claude,
// auto = --permission-mode auto, all-in = --permission-mode bypassPermissions. The
// harness holds a cross-session message only when exactly one side is in the
// bypass class, so default and auto are one class and all-in the other. all-in is
// never a fallback: only an explicit flag or a value the owner set selects it.
var LaunchModes = []string{"default", "auto", "all-in"}

// ValidLaunchMode reports whether s is an accepted LETS_LAUNCH_MODE value. The
// empty string is NOT valid here - callers treat "" as "not supplied" and fall
// back to Defaults()["LETS_LAUNCH_MODE"] (default) before validating.
func ValidLaunchMode(s string) bool {
	for _, m := range LaunchModes {
		if m == s {
			return true
		}
	}
	return false
}

// LaunchModeSetting reads LETS_LAUNCH_MODE with the MergedEnv precedence and names
// where it came from: the project .lets/.env (source "project"), else the
// user-level ~/.lets/.env (source "user"), else "" (source "unset"). Only a
// non-empty value counts, as in MergedEnv. The value is returned as written -
// validating it is the caller's (an invalid value is treated as default, never
// guessed). homeDir == "" skips the user-level layer.
func LaunchModeSetting(projectRoot, homeDir string) (value, source string) {
	const key = "LETS_LAUNCH_MODE"
	if projectRoot != "" {
		if env, _ := readEnvFile(filepath.Join(projectRoot, ".lets", ".env")); env[key] != "" {
			return env[key], "project"
		}
	}
	if homeDir != "" {
		if env, _ := readEnvFile(filepath.Join(homeDir, ".lets", ".env")); env[key] != "" {
			return env[key], "user"
		}
	}
	return "", "unset"
}
