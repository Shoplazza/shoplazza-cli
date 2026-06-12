package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// CliConfig stores the minimal persisted CLI configuration.
type CliConfig struct {
	CurrentAccount string `json:"current_account,omitempty"`
	StoreDomain    string `json:"store_domain,omitempty"`
}

// RuntimeContext carries resolved runtime state for a command execution.
type RuntimeContext struct {
	AccountName string
	StoreDomain string
}

// DefaultConfigPath returns the default local JSON config path.
func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "shoplazza-cli", "config.json"), nil
}

// LoadConfig loads config from the provided path.
func LoadConfig(path string) (CliConfig, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return CliConfig{}, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CliConfig{}, nil
		}
		return CliConfig{}, err
	}

	var cfg CliConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return CliConfig{}, err
	}
	return cfg, nil
}

// SaveConfig persists config to the provided path.
func SaveConfig(path string, cfg CliConfig) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// RemoveConfig deletes the persisted config file if it exists.
func RemoveConfig(path string) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}

	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
