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

// httpClient is a package-private HTTP client for usage API fetches.
//
// Avoids http.DefaultClient (any other package mutating DefaultTransport
// would affect us) and applies belt-and-suspenders defenses:
//   - Timeout covers the full request including body read (per-request ctx
//     bounds connection + headers; this catches body-read stalls).
//   - CheckRedirect refuses redirects so a Bearer token isn't reflected to
//     a redirect target (api.anthropic.com is unlikely to redirect, but
//     cheap defense-in-depth for credential-bearing requests).
//   - Transport caps connection count (we make at most 1 request at a time).
var httpClient = &http.Client{
	Timeout: apiTimeout + time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		MaxConnsPerHost:     2,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

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
	// 0o700 matches what `lets init` sets for .lets/cache. The dir holds
	// usage utilization timestamps - operational metadata we don't want
	// readable by other users on shared hosts.
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}
	// Re-tighten in case the dir already existed at a wider mode (e.g. an
	// older bash-statusline installation with default umask).
	_ = os.Chmod(cacheDir, 0o700)
	return writeUsageCache(filepath.Join(cacheDir, "usage"), u)
}

// readToken tries macOS Keychain first (no-op on non-darwin), then
// ~/.claude/.credentials.json fallback. Empty string on full failure.
//
// The credentials.json fallback refuses to read the file if its mode allows
// group or world access (mode & 0o077 != 0). On a multi-user host another
// user could otherwise replace the token (token-confusion attack) or just
// read it. Anthropic creates the file at 0o600; we enforce it on read.
func readToken() string {
	if t := readKeychainToken(); t != "" {
		return t
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	credsPath := filepath.Join(home, ".claude", ".credentials.json")
	fi, err := os.Stat(credsPath)
	if err != nil {
		return ""
	}
	if fi.Mode().Perm()&0o077 != 0 {
		// Refuse to read group/world-accessible credentials. Caller treats
		// empty token as "no token available" and silently skips fetch.
		return ""
	}
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

	resp, err := httpClient.Do(req)
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
