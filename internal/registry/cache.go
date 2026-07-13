package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Spec provenance values reported by SpecSource.
const (
	SourceEmbedded = "embedded"
	SourceCached   = "cached"
)

// osUserConfigDir is overridable in tests.
var osUserConfigDir = os.UserConfigDir

// CacheDir returns the directory holding downloaded metadata.
func CacheDir() (string, error) {
	dir, err := osUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shoplazza-cli", "meta"), nil
}

// CachedSpecPath is the downloaded spec location: metasync writes it,
// LoadSpec reads it. The path is owned here to avoid an import cycle.
func CachedSpecPath() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli_meta.json"), nil
}

// peekGeneratedAt extracts only generated_at, avoiding a full Spec parse.
// Returns "" on any error.
func peekGeneratedAt(data []byte) string {
	var h struct {
		GeneratedAt string `json:"generated_at"`
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return ""
	}
	return h.GeneratedAt
}

// NewestLocalRevision returns the generated_at of the newest locally
// available spec — embedded or downloaded cache — reading the cache file
// fresh (no memoization). Metasync uses it to gate downloads.
func NewestLocalRevision() string {
	rev := peekGeneratedAt(Embedded)
	path, err := CachedSpecPath()
	if err != nil {
		return rev
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return rev
	}
	if r := peekGeneratedAt(data); r > rev {
		return r
	}
	return rev
}

// loadCachedSpec returns the downloaded spec when it is valid and strictly
// newer than the embedded one (RFC3339 lexical compare); nil otherwise.
// The file is never deleted on failure — a later refresh overwrites it.
func loadCachedSpec() *Spec {
	path, err := CachedSpecPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if peekGeneratedAt(data) <= peekGeneratedAt(Embedded) {
		return nil
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil
	}
	if len(spec.Modules) == 0 || spec.GeneratedAt == "" {
		return nil
	}
	return &spec
}
