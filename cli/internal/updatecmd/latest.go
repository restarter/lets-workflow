package updatecmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// releasesURL is the GitHub API endpoint for the latest *stable* release
// (the API excludes prereleases from /releases/latest by definition).
// A var (not const) so tests can point it at an httptest.Server. NOTE: tests
// that swap it must not run with t.Parallel() - they'd race under -race.
var releasesURL = "https://api.github.com/repos/restarter/lets-workflow/releases/latest"

// httpTimeout bounds a single GitHub API call. `lets update` is interactive
// but a hung call would block visibly, so keep it short.
const httpTimeout = 5 * time.Second

// cacheTTL bounds how long a cached "latest version" lookup is reused before
// re-hitting GitHub. Generous because releases are infrequent and the
// unauthenticated API allows only 60 req/hr.
const cacheTTL = time.Hour

// cacheFileName is the file under .lets/cache/ holding the last lookup.
const cacheFileName = "update-check.json"

// LatestInfo is the resolved "latest release" answer plus provenance.
type LatestInfo struct {
	Version   string    `json:"version"`    // semver without leading "v", e.g. "0.6.0"
	Source    string    `json:"source"`     // "live" (fresh GitHub hit) | "cache" (TTL-fresh or stale fallback)
	CheckedAt time.Time `json:"checked_at"` // when the value was actually fetched from GitHub
}

// cacheEntry is the on-disk shape of <cacheDir>/update-check.json.
type cacheEntry struct {
	Version   string    `json:"latest_version"`
	CheckedAt time.Time `json:"checked_at"`
}

// FetchLatest resolves the latest stable release version.
//
//   - forceRefresh==false and a TTL-fresh cache exists -> returns it (Source "cache").
//   - otherwise GETs releasesURL (httpTimeout), parses {"tag_name":"vX.Y.Z"},
//     writes the cache, returns it (Source "live").
//   - on HTTP failure: returns a stale cache entry if any (Source "cache"),
//     else the HTTP error.
//
// cacheDir is <projectRoot>/.lets/cache (created if missing on write).
func FetchLatest(ctx context.Context, cacheDir string, forceRefresh bool) (LatestInfo, error) {
	cachePath := filepath.Join(cacheDir, cacheFileName)

	if !forceRefresh {
		if e, ok := readCache(cachePath); ok && time.Since(e.CheckedAt) < cacheTTL {
			return LatestInfo{Version: e.Version, Source: "cache", CheckedAt: e.CheckedAt}, nil
		}
	}

	v, err := fetchFromGitHub(ctx)
	if err != nil {
		if e, ok := readCache(cachePath); ok {
			return LatestInfo{Version: e.Version, Source: "cache", CheckedAt: e.CheckedAt}, nil
		}
		return LatestInfo{}, err
	}

	now := time.Now()
	_ = writeCache(cachePath, cacheEntry{Version: v, CheckedAt: now})
	return LatestInfo{Version: v, Source: "live", CheckedAt: now}, nil
}

// fetchFromGitHub does one GET against releasesURL and extracts tag_name with
// the leading "v" stripped. Bounds itself with httpTimeout on top of ctx.
func fetchFromGitHub(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lets-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases API: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	tag := strings.TrimSpace(body.TagName)
	if tag == "" {
		return "", fmt.Errorf("github releases API: empty tag_name")
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func readCache(path string) (cacheEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil || e.Version == "" {
		return cacheEntry{}, false
	}
	return e, true
}

func writeCache(path string, e cacheEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
