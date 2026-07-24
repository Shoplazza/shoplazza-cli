package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
)

// CliConfig stores the persisted CLI configuration (v2: accounts + profiles).
type CliConfig struct {
	ConfigVersion   int             `json:"configVersion,omitempty"`
	CurrentProfile  string          `json:"currentProfile,omitempty"`
	PreviousProfile string          `json:"previousProfile,omitempty"`
	Accounts        []AccountConfig `json:"accounts,omitempty"`
	Profiles        []ProfileConfig `json:"profiles,omitempty"`
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
// for platform domains (*.myshoplaza.com, including env segments like
// xxx.stg.myshoplaza.com), full host for custom domains; "-2"/"-3"… on conflict.
func DeriveProfileName(domain string, taken func(string) bool) string {
	base := domain
	for _, suffix := range []string{".myshoplaza.com", ".myshoplazza.com"} {
		if b, ok := strings.CutSuffix(domain, suffix); ok {
			base = b
			// Drop env segments (neymar.stg → neymar).
			if i := strings.IndexByte(base, '.'); i > 0 {
				base = base[:i]
			}
			break
		}
	}
	// A subdomain that is a Windows-reserved device name (con/nul/com3…) or is
	// otherwise not a valid profile name must not become one verbatim: the
	// auto-created profile's meta file would be unusable. Suffix to make it valid
	// while keeping it recognizable.
	if ValidateProfileName(base) != nil {
		base += "-store"
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

// CurrentStoreDomain returns the current profile's store domain, or "" if
// no profile is selected.
func (c *CliConfig) CurrentStoreDomain() string {
	if p := c.Current(); p != nil {
		return p.StoreDomain
	}
	return ""
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

	return fsx.WriteFileAtomic(path, data, 0o600)
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
