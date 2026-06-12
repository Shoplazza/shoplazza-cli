package keychain_test

import (
	"os"
	"path/filepath"
	"testing"

	"shoplazza-cli-v2/internal/keychain"
)

// usesTempDir redirects the OS config dir to a temp directory for the test.
func usesTempDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// macOS uses ~/Library/..., but os.UserConfigDir on macOS reads
	// $HOME/Library/Application Support. On Linux it reads $XDG_CONFIG_HOME or $HOME/.config.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	// Unset any leftover env that might point to a real config dir.
	t.Setenv("AppData", "")
}

func TestGetSetRemove(t *testing.T) {
	usesTempDir(t)

	const (
		svc     = keychain.ShoplazzaCliService
		account = "access_token:test.myshoplazza.com"
		secret  = "tok_abc123"
	)

	// Get from empty store returns "" without error.
	got, err := keychain.Get(svc, account)
	if err != nil {
		t.Fatalf("Get on empty store: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	// Set and Get round-trips the secret.
	if err := keychain.Set(svc, account, secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = keychain.Get(svc, account)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != secret {
		t.Errorf("Get = %q, want %q", got, secret)
	}

	// Remove clears the entry.
	if err := keychain.Remove(svc, account); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, err = keychain.Get(svc, account)
	if err != nil {
		t.Fatalf("Get after Remove: %v", err)
	}
	if got != "" {
		t.Errorf("Get after Remove = %q, want empty", got)
	}

	// Second Remove is a no-op.
	if err := keychain.Remove(svc, account); err != nil {
		t.Errorf("second Remove: %v", err)
	}
}

func TestSetOverwrite(t *testing.T) {
	usesTempDir(t)

	const svc, account = keychain.ShoplazzaCliService, "uat:test.myshoplazza.com"

	if err := keychain.Set(svc, account, "first"); err != nil {
		t.Fatal(err)
	}
	if err := keychain.Set(svc, account, "second"); err != nil {
		t.Fatal(err)
	}
	got, err := keychain.Get(svc, account)
	if err != nil {
		t.Fatal(err)
	}
	if got != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

func TestMasterKeyFilePermissions(t *testing.T) {
	usesTempDir(t)

	// Trigger master key creation via Set.
	if err := keychain.Set(keychain.ShoplazzaCliService, "test", "value"); err != nil {
		t.Fatal(err)
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine UserConfigDir:", err)
	}
	keyPath := filepath.Join(cfgDir, "shoplazza-cli", "keychain.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("master key not found at %s: %v", keyPath, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("master key permissions = %o, want 0600", perm)
	}
}

func TestEncryptedFilePermissions(t *testing.T) {
	usesTempDir(t)

	if err := keychain.Set(keychain.ShoplazzaCliService, "token", "mytoken"); err != nil {
		t.Fatal(err)
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine UserConfigDir:", err)
	}
	dir := filepath.Join(cfgDir, "shoplazza-cli", "keychain")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("keychain dir: %v", err)
	}
	for _, e := range entries {
		info, _ := e.Info()
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file %s: permissions = %o, want 0600", e.Name(), perm)
		}
	}
}
