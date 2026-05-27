package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	upgradeHTTPClient = http.DefaultClient
	currentExecutable = os.Executable
)

type UpgradeResult struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func newUpgradeCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade bikebook to the latest GitHub release",
		RunE: func(cmd *cobra.Command, args []string) error {
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			result, err := Upgrade(cmd.Context())
			if err != nil {
				return err
			}
			return RenderData(cmd.OutOrStdout(), contract, result)
		},
	}
}

func Upgrade(ctx context.Context) (UpgradeResult, error) {
	if envEnabled("BIKEBOOK_NO_UPGRADE") {
		return UpgradeResult{Status: "skipped", Reason: "BIKEBOOK_NO_UPGRADE is set"}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return UpgradeResult{}, err
	}
	if sameVersion(version, release.TagName) {
		return UpgradeResult{Status: "up_to_date", Version: release.TagName}, nil
	}
	asset, ok := findReleaseAsset(release, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return UpgradeResult{}, NewLocalError(ExitGeneric, "cli_upgrade_unavailable", "no release asset matches this platform", "platform", runtime.GOOS+"/"+runtime.GOARCH, "")
	}

	exePath, err := currentExecutable()
	if err != nil {
		return UpgradeResult{}, NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "", "")
	}
	tempDir, err := os.MkdirTemp("", "bikebook-upgrade-*")
	if err != nil {
		return UpgradeResult{}, NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "", "")
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, asset.Name)
	if err := downloadFile(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		return UpgradeResult{}, err
	}
	binaryPath, err := extractReleaseBinary(archivePath, tempDir, runtime.GOOS)
	if err != nil {
		return UpgradeResult{}, err
	}
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return UpgradeResult{}, NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "", "")
	}
	if err := os.Rename(binaryPath, exePath); err != nil {
		return UpgradeResult{}, NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "make sure the current user can replace the installed bikebook binary", "")
	}
	return UpgradeResult{Status: "upgraded", Version: release.TagName, Path: exePath}, nil
}

func fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bikebook-cli/"+version)
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return githubRelease{}, NewLocalError(ExitNetwork, "cli_network_error", err.Error(), "", "", "")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return githubRelease{}, NewLocalError(ExitNetwork, "cli_network_error", "failed to fetch latest release", "release", "", "")
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, NewLocalError(ExitGeneric, "cli_invalid_response", err.Error(), "release", "", "")
	}
	if strings.TrimSpace(release.TagName) == "" {
		return githubRelease{}, NewLocalError(ExitGeneric, "cli_invalid_response", "latest release has no tag_name", "release", "", "")
	}
	return release, nil
}

func sameVersion(current, latest string) bool {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	latest = strings.TrimPrefix(strings.TrimSpace(latest), "v")
	return current != "" && latest != "" && current == latest
}

func findReleaseAsset(release githubRelease, goos, goarch string) (githubReleaseAsset, bool) {
	want := releaseAssetName(release.TagName, goos, goarch)
	for _, asset := range release.Assets {
		if asset.Name == want {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}

func releaseAssetName(tag, goos, goarch string) string {
	extension := ".tar.gz"
	if goos == "windows" {
		extension = ".zip"
	}
	return "bikebook_" + tag + "_" + goos + "_" + goarch + extension
}

func downloadFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "bikebook-cli/"+version)
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return NewLocalError(ExitNetwork, "cli_network_error", err.Error(), "", "", "")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return NewLocalError(ExitNetwork, "cli_network_error", "failed to download release asset", "asset", "", "")
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "", "")
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return NewLocalError(ExitNetwork, "cli_network_error", err.Error(), "", "", "")
	}
	return nil
}

func extractReleaseBinary(archivePath, destDir, goos string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") || goos == "windows" {
		return extractZipBinary(archivePath, destDir, binaryName(goos))
	}
	return extractTarGzBinary(archivePath, destDir, binaryName(goos))
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "bikebook.exe"
	}
	return "bikebook"
}

func extractTarGzBinary(archivePath, destDir, name string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "asset", "", "")
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "asset", "", "")
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "asset", "", "")
		}
		if filepath.Base(header.Name) != name {
			continue
		}
		return writeExtractedBinary(reader, filepath.Join(destDir, name))
	}
	return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", "release archive did not contain bikebook binary", "asset", "", "")
}

func extractZipBinary(archivePath, destDir, name string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "asset", "", "")
	}
	defer reader.Close()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != name {
			continue
		}
		in, err := file.Open()
		if err != nil {
			return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "asset", "", "")
		}
		defer in.Close()
		return writeExtractedBinary(in, filepath.Join(destDir, name))
	}
	return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", "release archive did not contain bikebook binary", "asset", "", "")
}

func writeExtractedBinary(reader io.Reader, path string) (string, error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "", "")
	}
	defer out.Close()
	if _, err := io.Copy(out, reader); err != nil {
		return "", NewLocalError(ExitGeneric, "cli_upgrade_failed", err.Error(), "path", "", "")
	}
	return path, nil
}
