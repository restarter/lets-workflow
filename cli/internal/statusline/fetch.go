package statusline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	usageAPIURL = "https://api.anthropic.com/oauth/usage"
	apiTimeout  = 3 * time.Second // matches bash `curl -m 3`
	cacheTTL    = 5 * time.Minute
)

// fetchAndCacheUsage retrieves OAuth token (keychain → credentials.json),
// hits the Anthropic usage API, and writes the result to cacheDir/usage.
//
// Used by the background subprocess (`lets statusline --fetch-usage-only`).
// Errors are silent in the caller - this runs detached, no UI cares.
func fetchAndCacheUsage(cacheDir string) error {
	token := readToken()
	if token == "" {
		return errors.New("no oauth token available")
	}

	u, err := fetchUsage(token)
	if err != nil {
		return err
	}
	if !u.fiveHourOK || !u.sevenDayOK {
		return errors.New("api returned no usage values")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	return writeUsageCache(filepath.Join(cacheDir, "usage"), u)
}

// readToken tries macOS Keychain first (no-op on non-darwin), then
// ~/.claude/.credentials.json fallback. Empty string on full failure.
func readToken() string {
	if t := readKeychainToken(); t != "" {
		return t
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	credsPath := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return ""
	}
	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return ""
	}
	return creds.ClaudeAiOauth.AccessToken
}

// fetchUsage hits the Anthropic usage API and decodes percentages.
func fetchUsage(token string) (usage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageAPIURL, nil)
	if err != nil {
		return usage{}, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("user-agent", "claude-code/2.1.11")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return usage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return usage{}, fmt.Errorf("api status %d", resp.StatusCode)
	}

	var body struct {
		FiveHour struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return usage{}, err
	}

	u := usage{
		fiveHour:      int(body.FiveHour.Utilization + 0.5),
		fiveHourOK:    true,
		sevenDay:      int(body.SevenDay.Utilization + 0.5),
		sevenDayOK:    true,
		fiveHourReset: body.FiveHour.ResetsAt,
		sevenDayReset: body.SevenDay.ResetsAt,
	}
	return u, nil
}
