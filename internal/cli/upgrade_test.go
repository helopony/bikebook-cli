package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpgradeSkipsWhenDisabled(t *testing.T) {
	t.Setenv("BIKEBOOK_NO_UPGRADE", "1")
	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{"--json", "upgrade"}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "skipped"`) {
		t.Fatalf("stdout missing skipped status: %s", stdout.String())
	}
}

func TestReleaseAssetName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "darwin", goarch: "arm64", want: "bikebook_v1.2.3_darwin_arm64.tar.gz"},
		{goos: "linux", goarch: "amd64", want: "bikebook_v1.2.3_linux_amd64.tar.gz"},
		{goos: "windows", goarch: "amd64", want: "bikebook_v1.2.3_windows_amd64.zip"},
	}
	for _, tt := range tests {
		if got := releaseAssetName("v1.2.3", tt.goos, tt.goarch); got != tt.want {
			t.Fatalf("releaseAssetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestFindReleaseAsset(t *testing.T) {
	release := githubRelease{
		TagName: "v1.2.3",
		Assets: []githubReleaseAsset{
			{Name: "bikebook_v1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.test/linux"},
		},
	}
	asset, ok := findReleaseAsset(release, "linux", "amd64")
	if !ok || asset.BrowserDownloadURL != "https://example.test/linux" {
		t.Fatalf("asset = %+v, ok = %v", asset, ok)
	}
}

func TestSameVersionIgnoresVPrefix(t *testing.T) {
	if !sameVersion("1.2.3", "v1.2.3") {
		t.Fatal("expected versions to match")
	}
	if sameVersion("1.2.3", "v1.2.4") {
		t.Fatal("expected versions to differ")
	}
}
