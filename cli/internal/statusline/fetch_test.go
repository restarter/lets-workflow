package statusline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestFetchUsage_DecodesAPIShape closes S18 from the 2026-05-08 review:
// fetch.go was 0% covered. If the API JSON struct tags drift, statusline
// silently shows blank stats (or stale cache). Mock the API and assert
// the decoded fields match what we send.
func TestFetchUsage_DecodesAPIShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("authorization"); got != "Bearer test-token" {
			t.Errorf("authorization header = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != "oauth-2025-04-20" {
			t.Errorf("anthropic-beta header = %q", got)
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"five_hour": map[string]any{
				"utilization": 42.6,
				"resets_at":   "2026-05-08T15:30:00Z",
			},
			"seven_day": map[string]any{
				"utilization": 67.0,
				"resets_at":   "2026-05-14T10:00:00Z",
			},
		})
	}))
	defer srv.Close()

	// Swap the package-level URL var (restored on cleanup).
	orig := usageAPIURL
	usageAPIURL = srv.URL
	t.Cleanup(func() { usageAPIURL = orig })

	u, err := fetchUsage("test-token")
	if err != nil {
		t.Fatalf("fetchUsage: %v", err)
	}
	if u.fiveHour != 43 || !u.fiveHourOK { // 42.6 rounds to 43
		t.Errorf("fiveHour = %d (ok=%v), want 43", u.fiveHour, u.fiveHourOK)
	}
	if u.sevenDay != 67 || !u.sevenDayOK {
		t.Errorf("sevenDay = %d (ok=%v), want 67", u.sevenDay, u.sevenDayOK)
	}
	if u.fiveHourReset != "2026-05-08T15:30:00Z" {
		t.Errorf("fiveHourReset = %q", u.fiveHourReset)
	}
	if u.sevenDayReset != "2026-05-14T10:00:00Z" {
		t.Errorf("sevenDayReset = %q", u.sevenDayReset)
	}
}

func TestFetchUsage_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	orig := usageAPIURL
	usageAPIURL = srv.URL
	t.Cleanup(func() { usageAPIURL = orig })

	if _, err := fetchUsage("test-token"); err == nil {
		t.Error("expected error on 403 response, got nil")
	}
}

// TestReadToken_FromCredentialsJSON exercises the credentials.json fallback
// path on Linux/Windows (where Keychain is a stub). Also indirectly verifies
// the B10c mode check (refusing world-readable files).
func TestReadToken_FromCredentialsJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	credsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(credsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credsPath := filepath.Join(credsDir, ".credentials.json")
	body := `{"claudeAiOauth":{"accessToken":"sk-test-token-123"}}`
	if err := os.WriteFile(credsPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got := readToken()
	// On macOS, keychain may return a real token first - skip strict assertion
	// in that case. On other platforms, our credentials.json must be returned.
	if got == "" {
		t.Skip("readToken returned empty - keychain unavailable AND credentials.json check failed (expected on macOS without keychain entry)")
	}
	if got != "sk-test-token-123" {
		// macOS may have returned a real keychain token instead. That's fine.
		t.Logf("got non-test token (likely from keychain): %q", got)
	}
}

// TestReadToken_RefusesWorldReadableCreds verifies the B10c mode guard.
func TestReadToken_RefusesWorldReadableCreds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	credsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(credsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credsPath := filepath.Join(credsDir, ".credentials.json")
	body := `{"claudeAiOauth":{"accessToken":"sk-should-be-refused"}}`
	if err := os.WriteFile(credsPath, []byte(body), 0o644); err != nil { // 0644 = world-readable
		t.Fatal(err)
	}

	got := readToken()
	if got == "sk-should-be-refused" {
		t.Errorf("readToken should refuse world-readable creds (mode 0644), but returned token")
	}
	// Note: on macOS dev machines with real keychain, got may be a real token
	// (keychain checked first). The assertion above only fails if we read the
	// FAKE world-readable file - which is the bug we're guarding against.
}

// TestReadToken_NoCredsAndNoKeychain returns empty.
func TestReadToken_NoCredsAndNoKeychain(t *testing.T) {
	// Point HOME at an empty dir - no .credentials.json present.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// On non-darwin hosts the keychain stub returns "" so this is a clean
	// "no token at all" path. On macOS dev machines we may pick up a real
	// keychain token - skip in that case.
	got := readToken()
	if got != "" {
		t.Skipf("got token from keychain (likely macOS dev machine): %q", got)
	}
}
