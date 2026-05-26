package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildVersionInfo(t *testing.T) {
	previousVersion, previousCommit, previousBuiltAt := version, commit, builtAt
	t.Cleanup(func() {
		version = previousVersion
		commit = previousCommit
		builtAt = previousBuiltAt
	})
	SetBuildInfo("1.2.3", "abc123", "2026-05-26T18:00:00Z")

	info := BuildVersionInfo("https://example.test/public/v1")

	if info.Version != "1.2.3" || info.Commit != "abc123" || info.BuiltAt != "2026-05-26T18:00:00Z" {
		t.Fatalf("unexpected build info: %+v", info)
	}
	if info.APIBase != "https://example.test/public/v1" || info.LatestAvailable != "unknown" {
		t.Fatalf("unexpected version info: %+v", info)
	}
}

func TestLatestAvailableVersionUsesDailyCache(t *testing.T) {
	withTempCache(t)
	if err := writeLatestVersionCache(latestVersionCache{
		LatestAvailable: "v1.2.3",
		CheckedAt:       time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP client should not be called for fresh cache")
		return nil, nil
	})}

	got, err := latestAvailableVersion(t.Context(), client, time.Date(2026, 5, 26, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("latestAvailableVersion() error = %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("latest = %q", got)
	}
}

func TestLatestAvailableVersionFetchesAndCaches(t *testing.T) {
	cacheDir := withTempCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	latestReleaseURLForTest(t, server.URL)

	got, err := latestAvailableVersion(t.Context(), client, time.Date(2026, 5, 26, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("latestAvailableVersion() error = %v", err)
	}
	if got != "v2.0.0" {
		t.Fatalf("latest = %q", got)
	}
	cachePath, err := latestVersionCachePath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cachePath, cacheDir) {
		t.Fatalf("cache path %q should be under %q", cachePath, cacheDir)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withTempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	return dir
}

func latestReleaseURLForTest(t *testing.T, url string) {
	t.Helper()
	previous := latestReleaseURL
	latestReleaseURL = url
	t.Cleanup(func() {
		latestReleaseURL = previous
	})
}
