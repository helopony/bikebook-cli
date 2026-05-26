package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSetAPIKeyWrites0600AndRedacts(t *testing.T) {
	home := withTempHome(t)

	var stdout, stderr bytes.Buffer
	code := executeConfigForTest([]string{"config", "set", "api-key", "--profile", "shop", "--json"}, "bbk_live_secret\n", &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "bbk_live_secret") {
		t.Fatalf("stdout exposed raw key: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "bbk_live_***") {
		t.Fatalf("stdout missing redacted key: %s", stdout.String())
	}

	path := filepath.Join(home, ".bikebook", "config.toml")
	mode, err := fileMode(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode != 0600 {
		t.Fatalf("mode = %v, want 0600", mode)
	}

	store, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Profiles["shop"].APIKey; got != "bbk_live_secret" {
		t.Fatalf("stored key = %q", got)
	}
}

func TestConfigSetAPIKeyCanUseEnvWithoutArgument(t *testing.T) {
	withTempHome(t)
	t.Setenv("BIKEBOOK_API_KEY", "bbk_test_envsecret")

	var stdout, stderr bytes.Buffer
	code := executeConfigForTest([]string{"config", "set", "api-key", "--profile", "envprof"}, "", &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	store, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Profiles["envprof"].APIKey; got != "bbk_test_envsecret" {
		t.Fatalf("stored key = %q", got)
	}
}

func TestConfigListAndProfilesRedactKeys(t *testing.T) {
	withTempHome(t)
	store := NewConfigStore()
	_ = store.setAPIKey(DefaultProfile, "bbk_live_defaultsecret")
	_ = store.setAPIKey("shop", "bbk_test_shopsecret")
	store.CurrentProfile = "shop"
	if err := SaveConfig(store); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := executeConfigForTest([]string{"config", "profiles", "list", "--json"}, "", &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	got := stdout.String()
	for _, raw := range []string{"bbk_live_defaultsecret", "bbk_test_shopsecret"} {
		if strings.Contains(got, raw) {
			t.Fatalf("profile list exposed raw key %q: %s", raw, got)
		}
	}
	for _, redacted := range []string{"bbk_live_***", "bbk_test_***"} {
		if !strings.Contains(got, redacted) {
			t.Fatalf("profile list missing %q: %s", redacted, got)
		}
	}
}

func TestConfigProfilesUseAndUnset(t *testing.T) {
	withTempHome(t)

	var stdout, stderr bytes.Buffer
	code := executeConfigForTest([]string{"config", "profiles", "use", "shop"}, "", &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("use exit = %d, stderr = %s", code, stderr.String())
	}
	store, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if store.CurrentProfile != "shop" {
		t.Fatalf("current profile = %q", store.CurrentProfile)
	}

	code = executeConfigForTest([]string{"config", "profiles", "unset", "shop"}, "", &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("unset exit = %d, stderr = %s", code, stderr.String())
	}
	store, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if store.CurrentProfile != DefaultProfile {
		t.Fatalf("current profile after unset = %q", store.CurrentProfile)
	}
	if _, ok := store.Profiles["shop"]; ok {
		t.Fatal("shop profile was not removed")
	}
}

func TestConfigGetAndUnsetAPIKey(t *testing.T) {
	withTempHome(t)
	store := NewConfigStore()
	_ = store.setAPIKey(DefaultProfile, "bbk_live_secret")
	if err := SaveConfig(store); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := executeConfigForTest([]string{"config", "get", "api-key", "--json"}, "", &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("get exit = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "bbk_live_secret") || !strings.Contains(stdout.String(), "bbk_live_***") {
		t.Fatalf("get did not redact key: %s", stdout.String())
	}

	code = executeConfigForTest([]string{"config", "unset", "api-key"}, "", &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("unset exit = %d, stderr = %s", code, stderr.String())
	}
	store, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if store.Profiles[DefaultProfile].APIKey != "" {
		t.Fatalf("api key was not unset")
	}
}

func executeConfigForTest(args []string, stdin string, stdout, stderr *bytes.Buffer) int {
	cmd, opts := newRootCommand()
	SetOutput(cmd, stdout, stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		contract := contractFromOptions(*opts, stdout)
		return RenderError(stderr, contract, err)
	}
	return ExitSuccess
}

func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestConfigPathUsesBikebookHome(t *testing.T) {
	home := withTempHome(t)
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".bikebook", "config.toml")
	if path != want {
		t.Fatalf("ConfigPath() = %q, want %q", path, want)
	}
	exists, err := configExists()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("config should not exist yet")
	}
	if err := SaveConfig(NewConfigStore()); err != nil {
		t.Fatal(err)
	}
	exists, err = configExists()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("config should exist after save")
	}
}
