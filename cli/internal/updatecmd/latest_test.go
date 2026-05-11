package updatecmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withReleasesURL(t *testing.T, u string) {
	t.Helper()
	orig := releasesURL
	releasesURL = u
	t.Cleanup(func() { releasesURL = orig })
}

func TestFetchLatest_LiveThenCached(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.6.0"})
	}))
	defer srv.Close()
	withReleasesURL(t, srv.URL)
	dir := t.TempDir()

	got, err := FetchLatest(context.Background(), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0.6.0" || got.Source != "live" {
		t.Fatalf("first = %+v, want version 0.6.0 source live", got)
	}
	// Second call within TTL: served from cache, no new HTTP hit.
	got2, err := FetchLatest(context.Background(), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Version != "0.6.0" || got2.Source != "cache" {
		t.Fatalf("second = %+v, want version 0.6.0 source cache", got2)
	}
	if hits != 1 {
		t.Fatalf("HTTP hits = %d, want 1 (second served from cache)", hits)
	}
}

func TestFetchLatest_RefreshCacheBypassesTTL(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.7.0"})
	}))
	defer srv.Close()
	withReleasesURL(t, srv.URL)
	dir := t.TempDir()
	// Seed a fresh cache with an older version.
	if err := writeCache(filepath.Join(dir, cacheFileName), cacheEntry{Version: "0.6.0", CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	got, err := FetchLatest(context.Background(), dir, true) // forceRefresh
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0.7.0" || got.Source != "live" || hits != 1 {
		t.Fatalf("got %+v hits %d, want 0.7.0/live/1", got, hits)
	}
}

func TestFetchLatest_HTTPErrorFallsBackToStaleCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	withReleasesURL(t, srv.URL)
	dir := t.TempDir()
	stale := time.Now().Add(-2 * cacheTTL) // older than TTL
	if err := writeCache(filepath.Join(dir, cacheFileName), cacheEntry{Version: "0.5.1", CheckedAt: stale}); err != nil {
		t.Fatal(err)
	}

	got, err := FetchLatest(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("expected stale-cache fallback, got error: %v", err)
	}
	if got.Version != "0.5.1" || got.Source != "cache" {
		t.Fatalf("got %+v, want 0.5.1/cache", got)
	}
}

func TestFetchLatest_HTTPErrorNoCacheReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	withReleasesURL(t, srv.URL)
	if _, err := FetchLatest(context.Background(), t.TempDir(), false); err == nil {
		t.Fatal("expected error when GitHub fails and no cache exists")
	}
}

func TestFetchLatest_StripsLeadingV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.2.3"})
	}))
	defer srv.Close()
	withReleasesURL(t, srv.URL)
	got, err := FetchLatest(context.Background(), t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.2.3" {
		t.Fatalf("Version = %q, want 1.2.3 (v stripped)", got.Version)
	}
}

func TestFetchLatest_RejectsGarbageTag(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.0.0\x1b[2Jnot-a-version"})
	}))
	defer srv.Close()
	withReleasesURL(t, srv.URL)
	if _, err := FetchLatest(context.Background(), dir, false); err == nil {
		t.Fatal("expected error for a non-semver tag_name")
	}
	// And nothing got cached.
	if _, ok := readCache(filepath.Join(dir, cacheFileName)); ok {
		t.Fatal("a garbage tag should not be written to the cache")
	}
}

func TestReadCache_RejectsEmptyVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, cacheFileName)
	if err := os.WriteFile(p, []byte(`{"latest_version":"","checked_at":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readCache(p); ok {
		t.Fatal("readCache should reject an entry with empty version")
	}
}
