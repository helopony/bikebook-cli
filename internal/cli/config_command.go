package cli

import (
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type configValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Profile string `json:"profile,omitempty"`
	Source  string `json:"source,omitempty"`
}

type configListOutput struct {
	ConfigPath     string            `json:"config_path"`
	CurrentProfile string            `json:"current_profile"`
	Profiles       []profileListItem `json:"profiles"`
}

type profileListItem struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	APIKey  string `json:"api_key,omitempty"`
}

func newConfigCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage BikeBook CLI configuration",
	}

	cmd.AddCommand(newConfigGetCommand(opts))
	cmd.AddCommand(newConfigSetCommand(opts))
	cmd.AddCommand(newConfigListCommand(opts))
	cmd.AddCommand(newConfigUnsetCommand(opts))
	cmd.AddCommand(newConfigProfilesCommand(opts))
	return cmd
}

func newConfigGetCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if err := requireKnownConfigKey(key); err != nil {
				return err
			}
			store, err := LoadConfig()
			if err != nil {
				return err
			}
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			profile := selectProfile(opts.profile, store)

			var out configValue
			switch key {
			case "api-key":
				out = configValue{Key: key, Value: RedactAPIKey(store.Profiles[profile].APIKey), Profile: profile}
			case "profile", "current-profile":
				out = configValue{Key: key, Value: profile}
			}
			return RenderData(cmd.OutOrStdout(), contract, out)
		},
	}
}

func newConfigSetCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if key != "api-key" {
				return NewLocalError(ExitUsage, "cli_invalid_config_key", "only api-key can be set with config set", "key", "use `bikebook config profiles use <name>` to switch profiles", "")
			}

			store, err := LoadConfig()
			if err != nil {
				return err
			}
			profile := selectProfile(opts.profile, store)
			value, source, err := readAPIKeyForConfigSet(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if _, ok := EnvFromAPIKey(value); !ok {
				return NewLocalError(ExitUsage, "cli_invalid_api_key", "API key must start with bbk_live_ or bbk_test_", "api_key", "", "")
			}
			if err := store.setAPIKey(profile, value); err != nil {
				return err
			}
			if err := SaveConfig(store); err != nil {
				return err
			}

			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			return RenderData(cmd.OutOrStdout(), contract, configValue{
				Key:     "api-key",
				Value:   RedactAPIKey(value),
				Profile: profile,
				Source:  source,
			})
		},
	}
}

func newConfigListCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List config values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadConfig()
			if err != nil {
				return err
			}
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			out, err := configList(store)
			if err != nil {
				return err
			}
			return RenderData(cmd.OutOrStdout(), contract, out)
		},
	}
}

func newConfigUnsetCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Unset a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if key != "api-key" {
				return NewLocalError(ExitUsage, "cli_invalid_config_key", "only api-key can be unset with config unset", "key", "use `bikebook config profiles unset <name>` to remove a profile", "")
			}
			store, err := LoadConfig()
			if err != nil {
				return err
			}
			profile := selectProfile(opts.profile, store)
			if err := store.setAPIKey(profile, ""); err != nil {
				return err
			}
			if err := SaveConfig(store); err != nil {
				return err
			}
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			return RenderData(cmd.OutOrStdout(), contract, configValue{Key: key, Profile: profile})
		},
	}
}

func newConfigProfilesCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "Manage saved config profiles",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadConfig()
			if err != nil {
				return err
			}
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			out, err := profileList(store)
			if err != nil {
				return err
			}
			return RenderData(cmd.OutOrStdout(), contract, out)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Select the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadConfig()
			if err != nil {
				return err
			}
			if err := store.useProfile(args[0]); err != nil {
				return err
			}
			if err := SaveConfig(store); err != nil {
				return err
			}
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			return RenderData(cmd.OutOrStdout(), contract, configValue{Key: "profile", Value: store.CurrentProfile})
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unset <name>",
		Short: "Remove a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadConfig()
			if err != nil {
				return err
			}
			if err := store.unsetProfile(args[0]); err != nil {
				return err
			}
			if err := SaveConfig(store); err != nil {
				return err
			}
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			return RenderData(cmd.OutOrStdout(), contract, configValue{Key: "profile", Value: args[0]})
		},
	})

	return cmd
}

func readAPIKeyForConfigSet(r io.Reader) (string, string, error) {
	if !isTerminal(r) {
		bytes, err := io.ReadAll(r)
		if err != nil {
			return "", "", err
		}
		if value := strings.TrimSpace(string(bytes)); value != "" {
			return value, "stdin", nil
		}
	}
	if value := strings.TrimSpace(os.Getenv("BIKEBOOK_API_KEY")); value != "" {
		return value, "env", nil
	}
	return "", "", NewLocalError(ExitUsage, "cli_missing_api_key", "API key must be provided on stdin or BIKEBOOK_API_KEY", "api_key", "run `printf %s \"$BIKEBOOK_API_KEY\" | bikebook config set api-key`", "")
}

func configList(store ConfigStore) (configListOutput, error) {
	profiles, err := profileList(store)
	if err != nil {
		return configListOutput{}, err
	}
	path, err := ConfigPath()
	if err != nil {
		return configListOutput{}, err
	}
	return configListOutput{
		ConfigPath:     path,
		CurrentProfile: store.CurrentProfile,
		Profiles:       profiles,
	}, nil
}

func profileList(store ConfigStore) ([]profileListItem, error) {
	store.ensure()
	names := make([]string, 0, len(store.Profiles))
	for name := range store.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]profileListItem, 0, len(names))
	for _, name := range names {
		items = append(items, profileListItem{
			Name:    name,
			Current: name == store.CurrentProfile,
			APIKey:  RedactAPIKey(store.Profiles[name].APIKey),
		})
	}
	return items, nil
}
