//go:build !darwin

package statusline

// readKeychainToken always returns empty string on non-macOS platforms.
// Linux: secret-service integration is deferred to lets-ds6bc.
// Windows: Credential Manager integration is deferred to lets-ds6bc.
//
// On these platforms, callers must fall back to ~/.claude/.credentials.json.
func readKeychainToken() string {
	return ""
}
