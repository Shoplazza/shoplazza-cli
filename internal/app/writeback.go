package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"shoplazza-cli-v2/internal/app/project"
	"shoplazza-cli-v2/internal/fsx"
)

// WriteBackExtensionVersion updates id/version in
// extensions/<dir>/shoplazza.extension.toml, preserving the other keys. The toml
// is a CACHE; the truth source is the remote GetExtensionVersions diff (so a
// stale/missing id self-heals on the next deploy/dev/release). v1 parity:
// deploy.js:156 / dev.js:154 write the id back after every upsert — without it
// the next run cannot id-match and falls back to name matching or re-creating
// the extension.
func WriteBackExtensionVersion(root, dir, id, version string) error {
	path := filepath.Join(root, project.ExtensionsDir, dir, "shoplazza.extension.toml")
	var m map[string]any
	if _, err := toml.DecodeFile(path, &m); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no toml → nothing to cache (the file is the cache, not the truth)
		}
		return err
	}
	if m == nil {
		m = map[string]any{}
	}
	if id != "" {
		m["id"] = id
	}
	if version != "" {
		m["version"] = version
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return err
	}
	return fsx.WriteFileAtomic(path, buf.Bytes(), 0o644)
}

// deprecationNotice marks a migrated v1 extension.config.json (JSON has no comments).
const deprecationNotice = "DEPRECATED: superseded by shoplazza.extension.toml (v2). The CLI no longer reads this file."

// MigrateV1Extension writes a v2 shoplazza.extension.toml (with the deployed id)
// for a legacy extension.config.json and marks the json deprecated (kept, not
// deleted). appId/partnerId are not carried over. No-op when there's no v1 json.
func MigrateV1Extension(root, dir, id, name, typ, version string) error {
	extDir := filepath.Join(root, project.ExtensionsDir, dir)
	jsonPath := filepath.Join(extDir, "extension.config.json")
	if _, err := os.Stat(jsonPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no v1 json → nothing to migrate
		}
		return err
	}

	// Write the v2 toml unless one already exists (then the write-back owns it).
	tomlPath := filepath.Join(extDir, "shoplazza.extension.toml")
	if _, err := os.Stat(tomlPath); errors.Is(err, os.ErrNotExist) {
		m := map[string]any{"name": name, "type": typ}
		if id != "" {
			m["id"] = id
		}
		if version != "" {
			m["version"] = version
		}
		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(m); err != nil {
			return err
		}
		if err := fsx.WriteFileAtomic(tomlPath, buf.Bytes(), 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return deprecateV1Config(jsonPath)
}

// deprecateV1Config adds a `_deprecated` notice to the v1 json, preserving its
// other keys. Idempotent: an already-marked file is left untouched.
func deprecateV1Config(jsonPath string) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if _, ok := m["_deprecated"]; ok {
		return nil
	}
	m["_deprecated"] = deprecationNotice
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(jsonPath, append(out, '\n'), 0o644)
}
