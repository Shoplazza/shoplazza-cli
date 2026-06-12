// Package devstate persists the per-directory development-theme registry
// used by `themes serve` when --theme-id is omitted: a map of store host →
// dev theme id, stored in <theme-dir>/.shoplazza/theme-state.json. The dot
// prefix keeps it out of theme zips and the serve watcher. Stdlib-only
// (plus internal/fsx for atomic writes) to satisfy the themes import guard.
package devstate

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"shoplazza-cli-v2/internal/fsx"
)

const (
	stateDir  = ".shoplazza"
	stateFile = "theme-state.json"
)

// state is the on-disk shape. Keyed by store host so one theme directory
// can hold an independent dev theme per store (auth store switching).
type state struct {
	DevThemes map[string]string `json:"dev_themes"`
}

// Path returns the state file location for a theme directory root.
func Path(root string) string {
	return filepath.Join(root, stateDir, stateFile)
}

// Load returns the dev theme id recorded for storeKey, if any. A missing
// or unreadable/corrupt file degrades to not-found — serve then creates a
// fresh dev theme and Save rewrites the file.
func Load(root, storeKey string) (string, bool) {
	data, err := os.ReadFile(Path(root))
	if err != nil {
		return "", false
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return "", false
	}
	id, ok := s.DevThemes[storeKey]
	return id, ok && id != ""
}

// Save records storeKey → themeID, merging with any existing entries for
// other stores. Corrupt existing content is discarded (same degradation as
// Load) rather than failing the write.
func Save(root, storeKey, themeID string) error {
	s := state{DevThemes: map[string]string{}}
	if data, err := os.ReadFile(Path(root)); err == nil {
		var prev state
		if json.Unmarshal(data, &prev) == nil && prev.DevThemes != nil {
			s = prev
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.DevThemes[storeKey] = themeID

	if err := os.MkdirAll(filepath.Join(root, stateDir), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write (temp + rename) so a crash mid-write can't leave a
	// truncated state file that Load would treat as corrupt.
	return fsx.WriteFileAtomic(Path(root), data, 0o600)
}

// StoreKey derives the state-map key from a client base URL
// (e.g. "https://xjn-dev.myshoplaza.com" → "xjn-dev.myshoplaza.com").
// Unparseable or empty input degrades to "default" so tests and unusual
// configs still get a stable key instead of an empty-string entry.
func StoreKey(baseURL string) string {
	if baseURL == "" {
		return "default"
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return "default"
	}
	return u.Host
}
