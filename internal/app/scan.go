package app

import (
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
		if _, err := toml.DecodeFile(filepath.Join(dir, e.Name(), "shoplazza.extension.toml"), &et); err != nil {
			// toml.DecodeFile on a missing file returns a *fs.PathError that
			// satisfies os.ErrNotExist — that's a non-extension dir, skip it.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, output.ErrValidation("extensions/%s/shoplazza.extension.toml is malformed: %v", e.Name(), err)
		}
		out = append(out, LocalExt{
			Dir:         e.Name(),
			Name:        et.Name,
			Type:        et.Type,
			Version:     et.Version,
			ExtensionID: et.ID,
		})
	}
	return out, nil
}
