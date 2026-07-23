// Package te holds the te (theme-extension) leg's business logic: project config
// (the extension_id truth source), store-openapi calls, and the
// theme-app/-wrapped scaffold.
package theme_extension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// configFile is the standalone theme-extension project config. It uses v1's
// file name and field names (extension.config.json) so v1 projects and tooling
// stay compatible. (App-managed extensions still use shoplazza.extension.toml.)
const configFile = "extension.config.json"

// Config is the te project's persisted state. extension_id is the single
// cross-process truth source for the binding chain (connect/deploy/release run
// in separate processes and cannot share memory). client_secret is never stored.
// On-disk JSON keys match v1 (see rawConfig); Type/Subtype are v2 additions.
type Config struct {
	ExtensionID string // extensionId — truth source
	ClientID    string // appId — the bound app's client_id (written by connect)
	PartnerID   string // partnerId — owning partner of the bound app (written by connect; lets release skip the partner lookup)
	Name        string // extensionName (basic) / extensionTitle (embed)
	Version     string // version — last-built semver (te build writeback); te deploy's default target
	Type        string // type: theme
	Subtype     string // subtype: basic | embed
}

// rawConfig is the on-disk shape, using v1's extension.config.json field names.
// v1 wrote the project name under "extensionName" for the basic template and
// "extensionTitle" for embed; both map to Config.Name.
type rawConfig struct {
	ExtensionID    string `json:"extensionId"`
	AppID          string `json:"appId,omitempty"`
	PartnerID      string `json:"partnerId,omitempty"`
	ExtensionName  string `json:"extensionName,omitempty"`
	ExtensionTitle string `json:"extensionTitle,omitempty"`
	Version        string `json:"version,omitempty"`
	Type           string `json:"type,omitempty"`
	Subtype        string `json:"subtype,omitempty"`
}

func configPath(root string) string { return filepath.Join(root, configFile) }

// ReadConfig loads extension.config.json from root. A missing file keeps its
// fs.ErrNotExist identity (callers branch with errors.Is — "not a te project");
// a present-but-undecodable file is a distinct "malformed" error so callers
// never tell the user to re-register (which would orphan the extension_id the
// corrupt file still holds).
func ReadConfig(root string) (Config, error) {
	data, err := os.ReadFile(configPath(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("%s is malformed: %v", configFile, err)
	}
	var r rawConfig
	if err := json.Unmarshal(data, &r); err != nil {
		return Config{}, fmt.Errorf("%s is malformed: %v", configFile, err)
	}
	name := r.ExtensionName
	if name == "" {
		name = r.ExtensionTitle
	}
	return Config{
		ExtensionID: r.ExtensionID,
		ClientID:    r.AppID,
		PartnerID:   r.PartnerID,
		Name:        name,
		Version:     r.Version,
		Type:        r.Type,
		Subtype:     r.Subtype,
	}, nil
}

// WriteConfig writes the config atomically (unique temp + rename) as v1-style
// extension.config.json — the only persistence of extension_id across processes.
func WriteConfig(root string, c Config) error {
	r := rawConfig{
		ExtensionID: c.ExtensionID,
		AppID:       c.ClientID,
		PartnerID:   c.PartnerID,
		Version:     c.Version,
		Type:        c.Type,
		Subtype:     c.Subtype,
	}
	// v1 quirk preserved: the basic template stores the name under
	// "extensionName", the embed template under "extensionTitle".
	if c.Subtype == "embed" {
		r.ExtensionTitle = c.Name
	} else {
		r.ExtensionName = c.Name
	}
	buf, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(configPath(root), append(buf, '\n'), 0o644)
}

// RequireExtensionID reads the config and returns a validation error when
// extension_id is absent — connect/deploy/release/versions must not silently
// fail. Missing file / empty id → hint to register; malformed file → the
// malformed message (not the register hint, see ReadConfig).
func RequireExtensionID(root string) (Config, *output.ExitError) {
	c, err := ReadConfig(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, errNoExtensionID()
		}
		return Config{}, output.ErrValidation("%v", err)
	}
	if c.ExtensionID == "" {
		return Config{}, errNoExtensionID()
	}
	return c, nil
}

func errNoExtensionID() *output.ExitError {
	return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no extension_id for this te project",
		"register first with 'te build' / 'te serve', or recover the id with 'te list'")
}
