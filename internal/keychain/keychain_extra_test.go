package keychain_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
)

// ── Corrupted master key ──────────────────────────────────────────────────────

func TestGet_CorruptedMasterKey(t *testing.T) {
	usesTempDir(t)

	// Create a real entry first (also creates a valid master key), so Get
	// has something to decrypt once the key below is corrupted.
	const account = "access_token"
	if err := keychain.Set(keychain.ShoplazzaCliService, account, "val"); err != nil {
		t.Fatalf("setup Set: %v", err)
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine UserConfigDir:", err)
	}
	// Overwrite the master key with wrong length (not 32 bytes).
	keyPath := filepath.Join(cfgDir, "shoplazza-cli", "keychain.key")
	if err := os.WriteFile(keyPath, []byte("tooshort"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = keychain.Get(keychain.ShoplazzaCliService, account)
	if err == nil {
		t.Error("expected error for corrupted master key, got nil")
	}
}

// ── Corrupted / too-short ciphertext ─────────────────────────────────────────

func TestGet_CorruptedCiphertext(t *testing.T) {
	usesTempDir(t)

	// Create a real entry so its hashed filename exists on disk, then
	// overwrite that file's contents with too-short ciphertext.
	const account = "corrupted_account"
	if err := keychain.Set(keychain.ShoplazzaCliService, account, "val"); err != nil {
		t.Fatalf("setup Set: %v", err)
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine UserConfigDir:", err)
	}
	kcDir := filepath.Join(cfgDir, "shoplazza-cli", "keychain")
	entries, err := os.ReadDir(kcDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in fresh keychain dir, got %d", len(entries))
	}

	// Overwrite with a ciphertext that is too short (< 28 bytes = 12 IV + 16 GCM tag).
	encPath := filepath.Join(kcDir, entries[0].Name())
	if err := os.WriteFile(encPath, []byte("tooshort"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = keychain.Get(keychain.ShoplazzaCliService, account)
	if err == nil {
		t.Error("expected error for too-short ciphertext, got nil")
	}
}

// ── Multiple accounts, isolated storage ──────────────────────────────────────

func TestSet_MultipleAccounts(t *testing.T) {
	usesTempDir(t)

	accounts := []struct{ account, secret string }{
		{"uat:store1.myshoplazza.com", "uat_111"},
		{"access_token:store2.myshoplazza.com", "at_222"},
		{"uat:store3.myshoplazza.com", "uat_333"},
	}

	for _, a := range accounts {
		if err := keychain.Set(keychain.ShoplazzaCliService, a.account, a.secret); err != nil {
			t.Fatalf("Set(%q): %v", a.account, err)
		}
	}
	for _, a := range accounts {
		got, err := keychain.Get(keychain.ShoplazzaCliService, a.account)
		if err != nil {
			t.Fatalf("Get(%q): %v", a.account, err)
		}
		if got != a.secret {
			t.Errorf("Get(%q) = %q, want %q", a.account, got, a.secret)
		}
	}

	// Removing one account should not affect others.
	if err := keychain.Remove(keychain.ShoplazzaCliService, accounts[0].account); err != nil {
		t.Fatal(err)
	}
	for _, a := range accounts[1:] {
		got, err := keychain.Get(keychain.ShoplazzaCliService, a.account)
		if err != nil {
			t.Fatalf("Get(%q) after partial remove: %v", a.account, err)
		}
		if got != a.secret {
			t.Errorf("Get(%q) = %q, want %q", a.account, got, a.secret)
		}
	}
}

// ── Windows-illegal characters in the on-disk filename ───────────────────────

// The "store:"/"app:" prefix must not leave a ':' in the .enc filename — it's
// illegal on Windows and breaks Set with "The parameter is incorrect."
func TestSet_FileNameIsWindowsSafe(t *testing.T) {
	usesTempDir(t)

	account := "store:ceshi1.myshoplazza.com"
	if err := keychain.Set(keychain.ShoplazzaCliService, account, "tok"); err != nil {
		t.Fatalf("Set(%q): %v", account, err)
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine UserConfigDir:", err)
	}
	entries, err := os.ReadDir(filepath.Join(cfgDir, "shoplazza-cli", "keychain"))
	if err != nil {
		t.Fatal(err)
	}
	// Characters Windows forbids in a path component.
	const reserved = `<>:"/\|?*`
	for _, e := range entries {
		if i := strings.IndexAny(e.Name(), reserved); i >= 0 {
			t.Errorf("keychain file %q contains %q, illegal in a Windows filename", e.Name(), e.Name()[i])
		}
	}
}

// ── safeFileName special characters ──────────────────────────────────────────

func TestSet_AccountWithSpecialChars(t *testing.T) {
	usesTempDir(t)

	// Account names with special chars should be sanitized to a safe filename.
	account := "access_token:store.myshoplazza.com/path?query=1"
	if err := keychain.Set(keychain.ShoplazzaCliService, account, "secret_val"); err != nil {
		t.Fatalf("Set with special chars: %v", err)
	}
	got, err := keychain.Get(keychain.ShoplazzaCliService, account)
	if err != nil {
		t.Fatalf("Get with special chars: %v", err)
	}
	if got != "secret_val" {
		t.Errorf("Get = %q, want secret_val", got)
	}
}
