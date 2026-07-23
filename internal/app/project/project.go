// Package project models an app project: its root, multi-toml configs, and the
// project-level .shoplazza/app-state.json that records the active config.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Shoplazza/shoplazza-cli/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

const (
	stateDir      = ".shoplazza"
	stateFile     = "app-state.json"
	defaultConfig = "shoplazza.app.toml"
	ExtensionsDir = "extensions"
)

type Config struct {
	ClientID string `toml:"client_id"`
	// PartnerID is the owning partner/organization id, stored with the app since
	// partner↔app is many-to-one and immutable.
	PartnerID string `toml:"partner_id"`
	// Scopes is the space-separated OAuth scope string, e.g.
	// "read_customer write_cart_transform". A legacy TOML array is also accepted.
	Scopes string `toml:"scopes,omitempty"`
}

// DefaultScopes is the scope string the app template ships. `app config link`
// writes it when neither the Dashboard nor the target config supplies scopes,
// so a linked config isn't left without any (matching `app init`).
const DefaultScopes = "read_customer write_cart_transform"

type state struct {
	ActiveConfig string `json:"active_config"`
	ClientID     string `json:"client_id,omitempty"`
}

type Project struct {
	Root string
}

// Resolve returns the project root = resolve(cwd, path). path defaults to ".".
func Resolve(cwd, path string) string {
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func Open(root string) (*Project, error) {
	if root == "" {
		return nil, errors.New("empty project root")
	}
	return &Project{Root: root}, nil
}

func (p *Project) statePath() string { return filepath.Join(p.Root, stateDir, stateFile) }

func (p *Project) loadState() (state, error) {
	data, err := os.ReadFile(p.statePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state{ActiveConfig: defaultConfig}, nil
		}
		return state{}, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return state{}, err
	}
	if s.ActiveConfig == "" {
		s.ActiveConfig = defaultConfig
	}
	return s, nil
}

// SetActiveConfig records the active toml + cached client_id.
func (p *Project) SetActiveConfig(tomlName, clientID string) error {
	if err := validateConfigName(tomlName); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(p.Root, stateDir), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state{ActiveConfig: tomlName, ClientID: clientID}, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(p.statePath(), data, 0o600)
}

// validateConfigName rejects config names that are not bare file names: a value
// like "../evil.toml" would otherwise be joined onto the project root and
// read/write files outside it.
func validateConfigName(tomlName string) error {
	// "." and ".." are their own filepath.Base, so reject them explicitly.
	if tomlName == "" || tomlName == "." || tomlName == ".." || filepath.Base(tomlName) != tomlName {
		return output.ErrValidation("invalid config name %q: must be a bare file name without path separators", tomlName)
	}
	return nil
}

func (p *Project) ActiveConfigName() (string, error) {
	s, err := p.loadState()
	if err != nil {
		return "", err
	}
	return s.ActiveConfig, nil
}

func (p *Project) ActiveConfig() (Config, error) {
	name, err := p.ActiveConfigName()
	if err != nil {
		return Config{}, err
	}
	return p.ReadConfig(name)
}

func (p *Project) ReadConfig(tomlName string) (Config, error) {
	if err := validateConfigName(tomlName); err != nil {
		return Config{}, err
	}
	path := filepath.Join(p.Root, tomlName)
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		// A legacy `scopes` TOML array is accepted by joining with spaces
		// instead of failing on string-vs-slice.
		if joined, ok := scopesFromArray(path); ok {
			var legacy struct {
				ClientID  string `toml:"client_id"`
				PartnerID string `toml:"partner_id"`
			}
			if _, lErr := toml.DecodeFile(path, &legacy); lErr == nil {
				return Config{ClientID: legacy.ClientID, PartnerID: legacy.PartnerID, Scopes: joined}, nil
			}
		}
		return Config{}, err
	}
	return cfg, nil
}

// scopesFromArray reads the toml's `scopes` key as a legacy array and joins it
// with spaces. ok=false when the key is absent, not an array, or unreadable.
func scopesFromArray(path string) (string, bool) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return "", false
	}
	arr, isArr := raw["scopes"].([]any)
	if !isArr {
		return "", false
	}
	parts := make([]string, 0, len(arr))
	for _, v := range arr {
		s, isStr := v.(string)
		if !isStr {
			return "", false
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " "), true
}

func (p *Project) WriteConfig(tomlName string, cfg Config) error {
	if err := validateConfigName(tomlName); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return fsx.WriteFileAtomic(filepath.Join(p.Root, tomlName), buf.Bytes(), 0o644)
}

// UpdateConfig sets the given keys in the toml file, preserving everything else
// in it (the app template ships defaults — e.g. scopes — that a full overwrite
// would erase). A missing file starts from empty.
func (p *Project) UpdateConfig(tomlName string, set map[string]any) error {
	if err := validateConfigName(tomlName); err != nil {
		return err
	}
	path := filepath.Join(p.Root, tomlName)
	raw := map[string]any{}
	if _, err := toml.DecodeFile(path, &raw); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for k, v := range set {
		raw[k] = v
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return err
	}
	return fsx.WriteFileAtomic(path, buf.Bytes(), 0o644)
}
