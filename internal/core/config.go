package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CliConfig stores the persisted CLI configuration (v2: accounts + profiles).
type CliConfig struct {
	ConfigVersion   int             `json:"configVersion,omitempty"`
	CurrentProfile  string          `json:"currentProfile,omitempty"`
	PreviousProfile string          `json:"previousProfile,omitempty"`
	Accounts        []AccountConfig `json:"accounts,omitempty"`
	Profiles        []ProfileConfig `json:"profiles,omitempty"`
	// legacy v1 字段，T15 删除；过渡期只读不写
	CurrentAccount string `json:"current_account,omitempty"`
	StoreDomain    string `json:"store_domain,omitempty"`
}

// AccountConfig stores a saved auth account.
type AccountConfig struct {
	Name          string   `json:"name"`
	GrantedScopes []string `json:"grantedScopes,omitempty"`
}

// ProfileConfig binds a named profile to an account and store domain.
type ProfileConfig struct {
	Name        string   `json:"name"`
	Account     string   `json:"account"`
	StoreDomain string   `json:"storeDomain"`
	StoreID     string   `json:"storeId,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

var profileNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// ValidateProfileName enforces the profile-name naming contract.
func ValidateProfileName(name string) error {
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must match ^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$", name)
	}
	if windowsReserved[strings.ToLower(name)] {
		return fmt.Errorf("invalid profile name %q: reserved device name on Windows", name)
	}
	return nil
}

// DeriveProfileName maps a store domain to a default profile name: first label
// for *.myshoplazza.com, full host for custom domains; "-2"/"-3"… on conflict.
func DeriveProfileName(domain string, taken func(string) bool) string {
	base := domain
	if strings.HasSuffix(domain, ".myshoplazza.com") {
		base = strings.TrimSuffix(domain, ".myshoplazza.com")
	}
	name := base
	for i := 2; taken(name); i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// FindProfile looks up a profile by name (case-insensitive).
func (c *CliConfig) FindProfile(name string) *ProfileConfig {
	for i := range c.Profiles {
		if strings.EqualFold(c.Profiles[i].Name, name) {
			return &c.Profiles[i]
		}
	}
	return nil
}

// FindProfileByStore looks up a profile by store domain (case-insensitive).
func (c *CliConfig) FindProfileByStore(domain string) *ProfileConfig {
	for i := range c.Profiles {
		if strings.EqualFold(c.Profiles[i].StoreDomain, domain) {
			return &c.Profiles[i]
		}
	}
	return nil
}

// Current returns the profile named by CurrentProfile, or nil if unset/unknown.
func (c *CliConfig) Current() *ProfileConfig { return c.FindProfile(c.CurrentProfile) }

// Account returns the single saved account (first entry), or nil if none.
func (c *CliConfig) Account() *AccountConfig {
	if len(c.Accounts) == 0 {
		return nil
	}
	return &c.Accounts[0]
}

// CurrentStoreDomain bridges v1 consumers during the transition: prefer the
// current profile's domain, fall back to the legacy top-level field. T15
// removes the fallback together with the legacy fields.
func (c *CliConfig) CurrentStoreDomain() string {
	if p := c.Current(); p != nil {
		return p.StoreDomain
	}
	return c.StoreDomain
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

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
