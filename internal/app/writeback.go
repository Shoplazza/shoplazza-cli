package app

import (
	"bytes"
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
