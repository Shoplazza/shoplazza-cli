package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"shoplazza-cli-v2/internal/app/project"
	"shoplazza-cli-v2/internal/output"
)

// scanExtToml is the subset of shoplazza.extension.toml the scanner reads.
type scanExtToml struct {
	ID      string `toml:"id"`
	Name    string `toml:"name"`
	Type    string `toml:"type"`
	Version string `toml:"version"`
}

// v1ExtConfig is the subset of the legacy v1 extension.config.json the scanner
// reads as a fallback when no v2 toml is present.
type v1ExtConfig struct {
	ExtensionID   string `json:"extensionId"`
	AppID         string `json:"appId"`
	ExtensionName string `json:"extensionName"`
	Version       string `json:"version"`
	Type          string `json:"type"`
}

// ScanLocalExtensions reads <root>/extensions/*/shoplazza.extension.toml and
// returns one LocalExt per subdir that has an extension toml. A missing
// extensions/ dir is tolerated (returns nil, nil), and subdirs WITHOUT a toml
// are skipped (not extension dirs). A toml that exists but fails to decode is a
// validation error — silently skipping it would make deploy/dev quietly ignore
// the extension.
func ScanLocalExtensions(root string) ([]LocalExt, *output.ExitError) {
	dir := filepath.Join(root, project.ExtensionsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, output.ErrInternal("failed to scan extensions: %v", err)
	}
	var out []LocalExt
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var et scanExtToml
		_, err := toml.DecodeFile(filepath.Join(dir, e.Name(), "shoplazza.extension.toml"), &et)
		if err == nil {
			out = append(out, LocalExt{
				Dir:         e.Name(),
				Name:        et.Name,
				Type:        et.Type,
				Version:     et.Version,
				ExtensionID: et.ID,
			})
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, output.ErrValidation("extensions/%s/shoplazza.extension.toml is malformed: %v", e.Name(), err)
		}

		// No v2 toml — fall back to the legacy v1 extension.config.json.
		v1, vErr := readV1ExtConfig(filepath.Join(dir, e.Name(), "extension.config.json"))
		if vErr != nil {
			if errors.Is(vErr, os.ErrNotExist) {
				continue // neither v2 nor v1: not an extension dir, skip
			}
			return nil, output.ErrValidation("extensions/%s/extension.config.json is malformed: %v", e.Name(), vErr)
		}
		out = append(out, LocalExt{
			Dir:         e.Name(),
			Name:        v1.ExtensionName,
			Type:        v1.Type,
			Version:     v1.Version,
			ExtensionID: v1.ExtensionID,
			AppID:       v1.AppID,
		})
	}
	return out, nil
}

// readV1ExtConfig reads a legacy v1 extension.config.json. A missing file
// returns an error satisfying os.ErrNotExist (so the caller can skip).
func readV1ExtConfig(path string) (v1ExtConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return v1ExtConfig{}, err
	}
	var c v1ExtConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return v1ExtConfig{}, err
	}
	return c, nil
}
