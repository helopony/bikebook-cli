package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var latestReleaseURL = "https://api.github.com/repos/helopony/bikebook-cli/releases/latest"
var latestAvailable = LatestAvailableVersion

type VersionInfo struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	BuiltAt         string `json:"built_at"`
	APIBase         string `json:"api_base"`
	LatestAvailable string `json:"latest_available"`
}

type latestVersionCache struct {
	LatestAvailable string    `json:"latest_available"`
	CheckedAt       time.Time `json:"checked_at"`
}

func BuildVersionInfo(apiBase string) VersionInfo {
	return VersionInfo{
		Version:         version,
		Commit:          commit,
		BuiltAt:         builtAt,
		APIBase:         apiBase,
		LatestAvailable: "unknown",
	}
}

func LatestAvailableVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return latestAvailableVersion(ctx, http.DefaultClient, time.Now())
}

func latestAvailableVersion(ctx context.Context, client *http.Client, now time.Time) (string, error) {
	cached, err := readLatestVersionCache()
	if err == nil && cached.LatestAvailable != "" && now.Sub(cached.CheckedAt) < 24*time.Hour {
		return cached.LatestAvailable, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "unknown", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bikebook-cli/"+version)

	resp, err := client.Do(req)
	if err != nil {
		if cached.LatestAvailable != "" {
			return cached.LatestAvailable, nil
		}
		return "unknown", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if cached.LatestAvailable != "" {
			return cached.LatestAvailable, nil
		}
		return "unknown", nil
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "unknown", err
	}

	latest := strings.TrimSpace(payload.TagName)
	if latest == "" {
		latest = strings.TrimSpace(payload.Name)
	}
	if latest == "" {
		latest = "unknown"
	}
	_ = writeLatestVersionCache(latestVersionCache{LatestAvailable: latest, CheckedAt: now})
	return latest, nil
}

func readLatestVersionCache() (latestVersionCache, error) {
	path, err := latestVersionCachePath()
	if err != nil {
		return latestVersionCache{}, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return latestVersionCache{}, err
	}
	var cached latestVersionCache
	if err := json.Unmarshal(bytes, &cached); err != nil {
		return latestVersionCache{}, err
	}
	return cached, nil
}

func writeLatestVersionCache(cached latestVersionCache) error {
	path, err := latestVersionCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0600)
}

func latestVersionCachePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "bikebook", "version.json"), nil
}
