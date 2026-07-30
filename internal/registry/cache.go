package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Spec provenance values reported by SpecSource.
const (
	SourceEmbedded = "embedded"
	SourceCached   = "cached"
)

// osUserConfigDir is overridable in tests.
var osUserConfigDir = os.UserConfigDir

// canonicalRevision is the only accepted generated_at form: UTC, second
// precision, Z suffix — so lexical order equals chronological order.
var canonicalRevision = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

// IsCanonicalRevision reports whether rev is in the canonical UTC form.
func IsCanonicalRevision(rev string) bool {
	return canonicalRevision.MatchString(rev)
}

// CacheDir returns the directory holding downloaded metadata.
func CacheDir() (string, error) {
	dir, err := osUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shoplazza-cli", "meta"), nil
}

// CachedSpecPath is the downloaded spec location (written by metasync, read
// by LoadSpec).
func CachedSpecPath() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli_meta.json"), nil
}

// peekGeneratedAt extracts only generated_at; "" on any error.
func peekGeneratedAt(data []byte) string {
	var h struct {
		GeneratedAt string `json:"generated_at"`
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return ""
	}
	return h.GeneratedAt
}

var (
	embeddedRevOnce sync.Once
	embeddedRev     string
)

// EmbeddedRevision returns the embedded spec's generated_at, memoized.
func EmbeddedRevision() string {
	embeddedRevOnce.Do(func() { embeddedRev = peekGeneratedAt(Embedded) })
	return embeddedRev
}

// ParseSpec unmarshals and validates a downloaded spec. It is the single
// definition of a usable remote spec, shared by the download and load paths.
func ParseSpec(data []byte) (*Spec, error) {
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	if spec.Version == "" {
		return nil, errors.New("registry: spec missing version")
	}
	if !IsCanonicalRevision(spec.GeneratedAt) {
		return nil, fmt.Errorf("registry: non-canonical generated_at %q", spec.GeneratedAt)
	}
	if len(spec.Modules) == 0 {
		return nil, errors.New("registry: spec has no modules")
	}
	seen := make(map[string]bool, len(spec.Modules))
	for _, m := range spec.Modules {
		if m.Name == "" || seen[m.Name] {
			return nil, fmt.Errorf("registry: empty or duplicate module name %q", m.Name)
		}
		seen[m.Name] = true
	}
	return &spec, nil
}

// NewestLocalRevision returns the generated_at of the newest locally usable
// spec; an invalid cache file never counts, so it can't block its own repair.
func NewestLocalRevision() string {
	if cached := loadCachedSpec(); cached != nil {
		return cached.GeneratedAt
	}
	return EmbeddedRevision()
}

// loadCachedSpec returns the downloaded spec when it is valid and strictly
// newer than the embedded one; nil otherwise. The file is never deleted on
// failure — a later refresh overwrites it.
func loadCachedSpec() *Spec {
	path, err := CachedSpecPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if peekGeneratedAt(data) <= EmbeddedRevision() {
		return nil
	}
	spec, err := ParseSpec(data)
	if err != nil {
		return nil
	}
	return spec
}
