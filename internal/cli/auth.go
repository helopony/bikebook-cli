package cli

import (
	"errors"
	"os"
	"strings"
)

const (
	EnvAuto = "auto"
	EnvLive = "live"
	EnvTest = "test"
)

type AuthSource string

const (
	AuthSourceNone   AuthSource = "none"
	AuthSourceFlag   AuthSource = "flag"
	AuthSourceEnv    AuthSource = "env"
	AuthSourceConfig AuthSource = "config"
)

type ResolvedAuth struct {
	APIKey   string     `json:"-"`
	Redacted string     `json:"api_key,omitempty"`
	Env      string     `json:"env"`
	Profile  string     `json:"profile,omitempty"`
	Source   AuthSource `json:"source"`
}

func ResolveAuth(opts rootOptions) (ResolvedAuth, error) {
	store, err := LoadConfig()
	if err != nil {
		return ResolvedAuth{}, err
	}

	profile := selectProfile(opts.profile, store)
	auth := ResolvedAuth{
		Env:     opts.env,
		Profile: profile,
		Source:  AuthSourceNone,
	}

	switch {
	case opts.apiKey != "":
		auth.APIKey = opts.apiKey
		auth.Source = AuthSourceFlag
	case os.Getenv("BIKEBOOK_API_KEY") != "":
		auth.APIKey = os.Getenv("BIKEBOOK_API_KEY")
		auth.Source = AuthSourceEnv
	case store.Profiles[profile].APIKey != "":
		auth.APIKey = store.Profiles[profile].APIKey
		auth.Source = AuthSourceConfig
	}

	if auth.APIKey == "" {
		if auth.Env == "" || auth.Env == EnvAuto {
			auth.Env = EnvAuto
		}
		return auth, nil
	}

	keyEnv, ok := EnvFromAPIKey(auth.APIKey)
	if !ok {
		return ResolvedAuth{}, NewLocalError(ExitUsage, "cli_invalid_api_key", "API key must start with bbk_live_ or bbk_test_", "api_key", "", "")
	}
	if auth.Env == "" || auth.Env == EnvAuto {
		auth.Env = keyEnv
	} else if auth.Env != keyEnv {
		return ResolvedAuth{}, NewLocalError(ExitUsage, "cli_env_key_mismatch", "resolved API key prefix does not match --env", "env", "use a matching --env value or API key prefix", "")
	}
	auth.Redacted = RedactAPIKey(auth.APIKey)
	return auth, nil
}

func selectProfile(flagProfile string, store ConfigStore) string {
	switch {
	case flagProfile != "":
		return flagProfile
	case os.Getenv("BIKEBOOK_PROFILE") != "":
		return os.Getenv("BIKEBOOK_PROFILE")
	case store.CurrentProfile != "":
		return store.CurrentProfile
	default:
		return DefaultProfile
	}
}

func EnvFromAPIKey(key string) (string, bool) {
	switch {
	case strings.HasPrefix(key, "bbk_live_"):
		return EnvLive, true
	case strings.HasPrefix(key, "bbk_test_"):
		return EnvTest, true
	default:
		return "", false
	}
}

func RedactAPIKey(key string) string {
	switch {
	case strings.HasPrefix(key, "bbk_live_"):
		return "bbk_live_***"
	case strings.HasPrefix(key, "bbk_test_"):
		return "bbk_test_***"
	case key == "":
		return ""
	default:
		return "***"
	}
}

func requireKnownConfigKey(key string) error {
	switch key {
	case "api-key", "profile", "current-profile":
		return nil
	default:
		return NewLocalError(ExitUsage, "cli_invalid_config_key", "unknown config key", "key", "supported keys: api-key, profile, current-profile", "")
	}
}

func requireProfileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return NewLocalError(ExitUsage, "cli_invalid_profile", "profile name is required", "profile", "", "")
	}
	if strings.ContainsAny(name, "\r\n\t ") {
		return NewLocalError(ExitUsage, "cli_invalid_profile", "profile name must not contain whitespace", "profile", "", "")
	}
	return nil
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
