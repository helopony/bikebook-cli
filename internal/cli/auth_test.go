package cli

import (
	"testing"
)

func TestResolveAuthOrderAndEnvDefault(t *testing.T) {
	withTempHome(t)
	t.Setenv("BIKEBOOK_API_KEY", "bbk_test_env")

	auth, err := ResolveAuth(rootOptions{})
	if err != nil {
		t.Fatalf("ResolveAuth() error = %v", err)
	}

	if auth.APIKey != "bbk_test_env" {
		t.Fatalf("APIKey = %q, want env key", auth.APIKey)
	}
	if auth.Source != AuthSourceEnv {
		t.Fatalf("Source = %q, want %q", auth.Source, AuthSourceEnv)
	}
	if auth.Env != EnvTest {
		t.Fatalf("Env = %q, want %q", auth.Env, EnvTest)
	}
	if auth.Redacted != "bbk_test_***" {
		t.Fatalf("Redacted = %q", auth.Redacted)
	}

	auth, err = ResolveAuth(rootOptions{apiKey: "bbk_live_flag"})
	if err != nil {
		t.Fatalf("ResolveAuth(flag) error = %v", err)
	}
	if auth.APIKey != "bbk_live_flag" || auth.Source != AuthSourceFlag || auth.Env != EnvLive {
		t.Fatalf("flag auth did not win: %+v", auth)
	}
}

func TestResolveAuthConfigProfileAndMismatch(t *testing.T) {
	withTempHome(t)
	store := NewConfigStore()
	if err := store.setAPIKey(DefaultProfile, "bbk_live_default"); err != nil {
		t.Fatal(err)
	}
	if err := store.setAPIKey("shop", "bbk_test_shop"); err != nil {
		t.Fatal(err)
	}
	store.CurrentProfile = "shop"
	if err := SaveConfig(store); err != nil {
		t.Fatal(err)
	}

	auth, err := ResolveAuth(rootOptions{})
	if err != nil {
		t.Fatalf("ResolveAuth() error = %v", err)
	}
	if auth.Profile != "shop" || auth.APIKey != "bbk_test_shop" || auth.Source != AuthSourceConfig {
		t.Fatalf("unexpected config auth: %+v", auth)
	}

	_, err = ResolveAuth(rootOptions{env: EnvLive})
	if err == nil {
		t.Fatal("ResolveAuth() mismatch error = nil")
	}
	cliErr, ok := err.(*CLIError)
	if !ok {
		t.Fatalf("error type = %T, want *CLIError", err)
	}
	if cliErr.Exit != ExitUsage {
		t.Fatalf("exit = %d, want %d", cliErr.Exit, ExitUsage)
	}
}

func TestProfileSelectionOrder(t *testing.T) {
	store := NewConfigStore()
	store.CurrentProfile = "current"

	if got := selectProfile("flag", store); got != "flag" {
		t.Fatalf("flag profile = %q", got)
	}
	t.Setenv("BIKEBOOK_PROFILE", "env")
	if got := selectProfile("", store); got != "env" {
		t.Fatalf("env profile = %q", got)
	}
	t.Setenv("BIKEBOOK_PROFILE", "")
	if got := selectProfile("", store); got != "current" {
		t.Fatalf("current profile = %q", got)
	}
}
