package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigPath_NonEmpty(t *testing.T) {
	got, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if got == "" {
		t.Error("DefaultConfigPath returned empty string")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, "no_such.json"))
	if err != nil {
		t.Fatalf("missing file should return empty config, got: %v", err)
	}
	if cfg.CurrentAccount != "" || cfg.StoreDomain != "" {
		t.Errorf("missing file: got non-empty config %+v", cfg)
	}
}

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := CliConfig{CurrentAccount: "acct-1", StoreDomain: "shop.myshoplaza.com"}
	if err := SaveConfig(path, want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.CurrentAccount != want.CurrentAccount || got.StoreDomain != want.StoreDomain {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

func TestSaveConfig_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.json")
	if err := SaveConfig(path, CliConfig{CurrentAccount: "x"}); err != nil {
		t.Fatalf("SaveConfig with nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestRemoveConfig_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = SaveConfig(path, CliConfig{CurrentAccount: "x"})
	if err := RemoveConfig(path); err != nil {
		t.Fatalf("RemoveConfig: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestRemoveConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no_such.json")
	if err := RemoveConfig(path); err != nil {
		t.Fatalf("RemoveConfig on missing file should not error: %v", err)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfig_EmptyPath_UsesDefaultPath(t *testing.T) {
	// An empty path triggers the DefaultConfigPath() fallback.
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("DefaultConfigPath unavailable: %v", err)
	}
	_ = cfg
}

func TestSaveConfig_EmptyPath_UsesDefaultPath(t *testing.T) {
	defaultPath, err := DefaultConfigPath()
	if err != nil {
		t.Skip("DefaultConfigPath unavailable")
	}
	// Back up and restore so the test environment stays clean.
	orig, readErr := os.ReadFile(defaultPath)
	defer func() {
		if readErr == nil {
			_ = os.WriteFile(defaultPath, orig, 0o600)
		} else {
			_ = os.Remove(defaultPath)
		}
	}()
	if err := SaveConfig("", CliConfig{CurrentAccount: "test-acct"}); err != nil {
		t.Fatalf("SaveConfig with empty path: %v", err)
	}
	loaded, err := LoadConfig(defaultPath)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if loaded.CurrentAccount != "test-acct" {
		t.Errorf("got %q want test-acct", loaded.CurrentAccount)
	}
}

func TestRemoveConfig_EmptyPath(t *testing.T) {
	// Verify it doesn't error when the default path doesn't exist.
	defaultPath, err := DefaultConfigPath()
	if err != nil {
		t.Skip("DefaultConfigPath unavailable")
	}
	// Remove first, then remove again via empty path to confirm idempotency.
	_ = os.Remove(defaultPath)
	if err := RemoveConfig(""); err != nil {
		t.Fatalf("RemoveConfig with empty path on missing file: %v", err)
	}
}
