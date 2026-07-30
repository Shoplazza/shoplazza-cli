package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects osUserConfigDir to a throwaway dir so no test in this
// package ever reads a real user's downloaded metadata cache.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		os.Exit(1)
	}
	osUserConfigDir = func() (string, error) { return dir, nil }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// useTempConfigDir points osUserConfigDir at a fresh per-test dir and resets
// the memoized spec before and after.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	saved := osUserConfigDir
	osUserConfigDir = func() (string, error) { return dir, nil }
	resetLoadSpec()
	t.Cleanup(func() {
		osUserConfigDir = saved
		resetLoadSpec()
	})
	return dir
}

func writeCachedSpec(t *testing.T, configDir, content string) {
	t.Helper()
	dir := filepath.Join(configDir, "shoplazza-cli", "meta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cli_meta.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSpec_CacheSelection(t *testing.T) {
	cases := []struct {
		name       string
		cached     string // "" = no cache file
		wantSource string
		wantModule string // module that must exist in the active spec
	}{
		{
			name:       "no cache file uses embedded",
			cached:     "",
			wantSource: SourceEmbedded,
		},
		{
			name:       "newer cache adopted",
			cached:     `{"version":"v9","generated_at":"9999-01-01T00:00:00Z","modules":[{"name":"zz-cache-probe","commands":[]}]}`,
			wantSource: SourceCached,
			wantModule: "zz-cache-probe",
		},
		{
			name:       "older cache ignored",
			cached:     `{"version":"v0","generated_at":"1970-01-01T00:00:00Z","modules":[{"name":"zz-cache-probe","commands":[]}]}`,
			wantSource: SourceEmbedded,
		},
		{
			name:       "corrupt cache ignored",
			cached:     `{not valid json`,
			wantSource: SourceEmbedded,
		},
		{
			name:       "cache with empty modules ignored",
			cached:     `{"version":"v9","generated_at":"9999-01-01T00:00:00Z","modules":[]}`,
			wantSource: SourceEmbedded,
		},
		{
			name:       "cache without generated_at ignored",
			cached:     `{"version":"v9","modules":[{"name":"zz-cache-probe","commands":[]}]}`,
			wantSource: SourceEmbedded,
		},
		{
			name:       "cache with non-canonical generated_at ignored",
			cached:     `{"version":"v9","generated_at":"9999-01-01T00:00:00+08:00","modules":[{"name":"zz-cache-probe","commands":[]}]}`,
			wantSource: SourceEmbedded,
		},
		{
			name:       "cache without version ignored",
			cached:     `{"generated_at":"9999-01-01T00:00:00Z","modules":[{"name":"zz-cache-probe","commands":[]}]}`,
			wantSource: SourceEmbedded,
		},
		{
			name:       "cache with duplicate module names ignored",
			cached:     `{"version":"v9","generated_at":"9999-01-01T00:00:00Z","modules":[{"name":"zz","commands":[]},{"name":"zz","commands":[]}]}`,
			wantSource: SourceEmbedded,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := useTempConfigDir(t)
			if tc.cached != "" {
				writeCachedSpec(t, dir, tc.cached)
			}
			s := LoadSpec()
			if s == nil {
				t.Fatal("LoadSpec must never return nil")
			}
			if got := SpecSource(); got != tc.wantSource {
				t.Fatalf("SpecSource() = %q, want %q", got, tc.wantSource)
			}
			if tc.wantSource == SourceEmbedded && len(s.Modules) == 0 {
				t.Fatal("embedded fallback must keep the embedded modules")
			}
			if tc.wantModule != "" {
				if _, ok := s.moduleIndex[tc.wantModule]; !ok {
					t.Fatalf("adopted cache must expose module %q", tc.wantModule)
				}
			}
		})
	}
}

// TestLoadSpec_CachedRescuesCorruptEmbedded: a valid newer cache wins even
// when the embedded payload is corrupt (peek of embedded generated_at is "").
func TestLoadSpec_CachedRescuesCorruptEmbedded(t *testing.T) {
	dir := useTempConfigDir(t)
	saved := Embedded
	t.Cleanup(func() {
		Embedded = saved
		resetLoadSpec()
	})
	Embedded = []byte("{not valid json")
	writeCachedSpec(t, dir, `{"version":"v9","generated_at":"2030-01-01T00:00:00Z","modules":[{"name":"zz-cache-probe","commands":[]}]}`)
	resetLoadSpec()

	s := LoadSpec()
	if got := SpecSource(); got != SourceCached {
		t.Fatalf("SpecSource() = %q, want %q", got, SourceCached)
	}
	if len(s.Modules) != 1 || s.Modules[0].Name != "zz-cache-probe" {
		t.Fatalf("unexpected modules: %+v", s.Modules)
	}
}

// An invalid cache file must not raise the local revision — otherwise it
// would gate off the re-download that repairs it.
func TestNewestLocalRevision_IgnoresInvalidCache(t *testing.T) {
	dir := useTempConfigDir(t)
	embedded := NewestLocalRevision()
	if embedded == "" {
		t.Fatal("embedded revision must be non-empty")
	}
	writeCachedSpec(t, dir, `{"generated_at":"9999-01-01T00:00:00Z","modules":[]}`)
	if got := NewestLocalRevision(); got != embedded {
		t.Fatalf("NewestLocalRevision() = %q, want embedded %q", got, embedded)
	}
	writeCachedSpec(t, dir, `{"version":"v9","generated_at":"9999-01-01T00:00:00Z","modules":[{"name":"zz","commands":[]}]}`)
	if got := NewestLocalRevision(); got != "9999-01-01T00:00:00Z" {
		t.Fatalf("NewestLocalRevision() = %q, want valid cache revision", got)
	}
}

func TestCachedSpecPath(t *testing.T) {
	dir := useTempConfigDir(t)
	p, err := CachedSpecPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "shoplazza-cli", "meta", "cli_meta.json")
	if p != want {
		t.Fatalf("CachedSpecPath() = %q, want %q", p, want)
	}
}
