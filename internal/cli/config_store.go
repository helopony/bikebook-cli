package cli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const DefaultProfile = "default"

type ConfigStore struct {
	CurrentProfile string                  `toml:"current_profile,omitempty" json:"current_profile,omitempty"`
	Profiles       map[string]ProfileStore `toml:"profiles,omitempty" json:"profiles"`
}

type ProfileStore struct {
	APIKey string `toml:"api_key,omitempty" json:"api_key,omitempty"`
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bikebook"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func LoadConfig() (ConfigStore, error) {
	path, err := ConfigPath()
	if err != nil {
		return ConfigStore{}, err
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		if isNotExist(err) {
			return NewConfigStore(), nil
		}
		return ConfigStore{}, err
	}

	store := NewConfigStore()
	if len(bytes) > 0 {
		if err := toml.Unmarshal(bytes, &store); err != nil {
			return ConfigStore{}, err
		}
	}
	store.ensure()
	return store, nil
}

func SaveConfig(store ConfigStore) error {
	store.ensure()
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	bytes, err := toml.Marshal(store)
	if err != nil {
		return err
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func NewConfigStore() ConfigStore {
	store := ConfigStore{
		CurrentProfile: DefaultProfile,
		Profiles: map[string]ProfileStore{
			DefaultProfile: {},
		},
	}
	return store
}

func (c *ConfigStore) ensure() {
	if c.CurrentProfile == "" {
		c.CurrentProfile = DefaultProfile
	}
	if c.Profiles == nil {
		c.Profiles = map[string]ProfileStore{}
	}
	if _, ok := c.Profiles[DefaultProfile]; !ok {
		c.Profiles[DefaultProfile] = ProfileStore{}
	}
}

func (c *ConfigStore) setAPIKey(profileName, apiKey string) error {
	if err := requireProfileName(profileName); err != nil {
		return err
	}
	c.ensure()
	profile := c.Profiles[profileName]
	profile.APIKey = apiKey
	c.Profiles[profileName] = profile
	return nil
}

func (c *ConfigStore) unsetProfile(profileName string) error {
	if profileName == DefaultProfile {
		return NewLocalError(ExitUsage, "cli_invalid_profile", "default profile cannot be removed", "profile", "", "")
	}
	if _, ok := c.Profiles[profileName]; !ok {
		return nil
	}
	delete(c.Profiles, profileName)
	if c.CurrentProfile == profileName {
		c.CurrentProfile = DefaultProfile
	}
	return nil
}

func (c *ConfigStore) useProfile(profileName string) error {
	if err := requireProfileName(profileName); err != nil {
		return err
	}
	c.ensure()
	if _, ok := c.Profiles[profileName]; !ok {
		c.Profiles[profileName] = ProfileStore{}
	}
	c.CurrentProfile = profileName
	return nil
}

func fileMode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

func configExists() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
