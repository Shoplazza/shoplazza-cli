package keychain

// Internal-package tests: these need access to unexported entryFileName,
// keychainDir, getMasterKey, encrypt so they live in package keychain rather
// than keychain_test (which only sees the exported Get/Set/Remove API).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/testenv"
)

// usesTempDir redirects the OS config dir to a temp directory for the test.
func usesTempDir(t *testing.T) {
	t.Helper()
	testenv.IsolateConfigDir(t)
}

// writeLegacyEntry freezes v1 on-disk behavior as a test fixture: old path
// service+"_"+safeFileName(account), raw (non-JSON) plaintext.
func writeLegacyEntry(t *testing.T, service, account, secret string) {
	t.Helper()
	if err := SetLegacy(service, account, secret); err != nil {
		t.Fatalf("writeLegacyEntry: %v", err)
	}
}

func TestSetGet_HashedFileName(t *testing.T) {
	usesTempDir(t)
	if err := Set("svc", "profile:us:store", "tok-1"); err != nil {
		t.Fatal(err)
	}
	got, err := Get("svc", "profile:us:store")
	if err != nil || got != "tok-1" {
		t.Fatalf("roundtrip: %q, %v", got, err)
	}
	// The on-disk filename is hashed, not the key material.
	dir, _ := keychainDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "profile") {
			t.Fatalf("filename leaks key: %s", e.Name())
		}
	}
}

func TestGetLegacy_ReadsOldNaming(t *testing.T) {
	usesTempDir(t)
	// Simulate a v1 entry written under the old naming/format.
	writeLegacyEntry(t, "shoplazza-cli", "uat", "old-uat")
	got, err := GetLegacy("shoplazza-cli", "uat")
	if err != nil || got != "old-uat" {
		t.Fatalf("legacy read: %q, %v", got, err)
	}
}

// KC-02: hash collision defense — Get must catch a mismatched embedded key.
func TestGet_KeyMismatchDetected(t *testing.T) {
	usesTempDir(t)
	if err := Set("svc", "key-a", "secret-a"); err != nil {
		t.Fatal(err)
	}
	// Simulate a collision: copy key-a's ciphertext under key-b's hashed name.
	dir, _ := keychainDir()
	src := filepath.Join(dir, entryFileName("svc", "key-a"))
	dst := filepath.Join(dir, entryFileName("svc", "key-b"))
	data, _ := os.ReadFile(src)
	_ = os.WriteFile(dst, data, 0o600)
	if _, err := Get("svc", "key-b"); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("collision must be detected, got %v", err)
	}
}
