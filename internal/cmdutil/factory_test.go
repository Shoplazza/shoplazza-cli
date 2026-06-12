package cmdutil

import (
	"path/filepath"
	"testing"
)

func TestDefaultFactory_AuthBaseURL_FixedDefault(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_AUTH_BASE_URL", "")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	f := NewDefaultFactory()
	if f.AuthClient == nil {
		t.Fatal("AuthClient is nil")
	}
	// Default auth base URL is the prod partners host, overridable at runtime
	// via SHOPLAZZA_CLI_AUTH_BASE_URL.
	if f.AuthClient.BaseURL != "https://partners.shoplazza.com" {
		t.Errorf("auth base URL = %q, want https://partners.shoplazza.com", f.AuthClient.BaseURL)
	}
}

func TestDefaultFactory_AuthBaseURL_EnvOverride(t *testing.T) {
	// Override with a host DISTINCT from the prod default so this genuinely
	// proves the env var wins — using the default value here would make the
	// test pass even if the override code were removed.
	t.Setenv("SHOPLAZZA_CLI_AUTH_BASE_URL", "https://partners.example.com")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	f := NewDefaultFactory()
	if f.AuthClient.BaseURL != "https://partners.example.com" {
		t.Errorf("auth base URL = %q, want env override https://partners.example.com", f.AuthClient.BaseURL)
	}
}
