//go:build darwin

package statusline

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// readKeychainToken queries macOS Keychain for the Claude Code OAuth token.
// Mirrors bash:
//
//	security find-generic-password -s "Claude Code-credentials" -w
//
// The blob is JSON; we extract .claudeAiOauth.accessToken. Empty string if
// any step fails (caller falls back to credentials.json).
func readKeychainToken() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-s", "Claude Code-credentials", "-w")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return ""
	}

	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return ""
	}
	return creds.ClaudeAiOauth.AccessToken
}
