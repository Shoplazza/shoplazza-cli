// Package keychain provides secure storage for CLI secrets (access tokens,
// UAT tokens). Secrets are encrypted with AES-256-GCM using a per-installation
// master key stored alongside the credentials directory.
//
// Storage layout (under os.UserConfigDir()/shoplazza-cli/):
//
//	keychain.key        — 32-byte random master key (0600)
//	keychain/<hash>.enc — AES-256-GCM ciphertext of {"k":..,"v":..} (0600)
package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
)

const (
	// ShoplazzaCliService is the service namespace for all CLI secrets.
	ShoplazzaCliService = "shoplazza-cli"

	masterKeyLen = 32 // AES-256
	ivLen        = 12 // GCM standard nonce
	gcmTagLen    = 16 // GCM authentication tag
)

// ErrNotFound is returned by Get when the requested secret does not exist.
var ErrNotFound = errors.New("keychain: item not found")

// ':' is excluded on purpose — it's illegal in Windows filenames, and account
// names carry a "store:"/"app:" prefix.
var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// safeFileName converts an account key to a safe file name. Used only by
// GetLegacy, which reads the pre-v2 on-disk layout.
func safeFileName(account string) string {
	return safeNameRe.ReplaceAllString(account, "_") + ".enc"
}

// entryFileName is the v2 on-disk name: hex(sha256(service+"\x00"+account)),
// truncated to 32 hex chars. Key material never appears in filenames
// (Windows-safe, collision-resistant, length-safe).
func entryFileName(service, account string) string {
	sum := sha256.Sum256([]byte(service + "\x00" + account))
	return hex.EncodeToString(sum[:16]) + ".enc"
}

// payload wraps the secret with its key for post-decrypt verification,
// guarding against a hash collision reading the wrong entry.
type payload struct {
	K string `json:"k"`
	V string `json:"v"`
}

// baseDir returns the directory that holds the master key and encrypted entries.
func baseDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("keychain: cannot locate config dir: %w", err)
	}
	return filepath.Join(cfg, "shoplazza-cli"), nil
}

// keychainDir returns the directory for encrypted secret files.
func keychainDir() (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "keychain"), nil
}

// masterKeyPath returns the path to the master key file.
func masterKeyPath() (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "keychain.key"), nil
}

// getMasterKey loads the master key, creating it on first use when allowCreate
// is true. Returns ErrNotFound when the key is absent and allowCreate is false.
func getMasterKey(allowCreate bool) ([]byte, error) {
	path, err := masterKeyPath()
	if err != nil {
		return nil, err
	}

	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != masterKeyLen {
			return nil, errors.New("keychain: master key is corrupted")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("keychain: read master key: %w", err)
	}
	if !allowCreate {
		return nil, ErrNotFound
	}

	// First use — generate and persist a master key atomically.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("keychain: create config dir: %w", err)
	}
	key = make([]byte, masterKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("keychain: generate master key: %w", err)
	}
	if err := fsx.WriteFileAtomic(path, key, 0o600); err != nil {
		// Another concurrent process may have won the race.
		if existing, rerr := os.ReadFile(path); rerr == nil && len(existing) == masterKeyLen {
			return existing, nil
		}
		return nil, fmt.Errorf("keychain: install master key: %w", err)
	}
	return key, nil
}

// encrypt returns IV || ciphertext+tag using AES-256-GCM.
func encrypt(plaintext string, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, ivLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), nil)
	out := make([]byte, 0, ivLen+len(ciphertext))
	out = append(out, iv...)
	out = append(out, ciphertext...)
	return out, nil
}

// decrypt reverses encrypt.
func decrypt(data, key []byte) (string, error) {
	if len(data) < ivLen+gcmTagLen {
		return "", errors.New("keychain: ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, data[:ivLen], data[ivLen:], nil)
	if err != nil {
		return "", errors.New("keychain: decryption failed (key mismatch or corruption)")
	}
	return string(plaintext), nil
}

// Get retrieves a secret stored under (service, account).
// Returns ("", nil) when the item does not exist.
func Get(service, account string) (string, error) {
	dir, err := keychainDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, entryFileName(service, account))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("keychain Get: %w", err)
	}
	key, err := getMasterKey(false)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("keychain Get: %w", err)
	}
	plaintext, err := decrypt(data, key)
	if err != nil {
		return "", err
	}
	var p payload
	if err := json.Unmarshal([]byte(plaintext), &p); err != nil {
		return "", fmt.Errorf("keychain Get: corrupted payload: %w", err)
	}
	if p.K != service+":"+account {
		return "", fmt.Errorf("keychain: key mismatch (hash collision?)")
	}
	return p.V, nil
}

// Set stores a secret under (service, account), overwriting any existing entry.
func Set(service, account, value string) error {
	key, err := getMasterKey(true)
	if err != nil {
		return fmt.Errorf("keychain Set: %w", err)
	}
	dir, err := keychainDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("keychain Set: mkdir: %w", err)
	}
	body, err := json.Marshal(payload{K: service + ":" + account, V: value})
	if err != nil {
		return fmt.Errorf("keychain Set: marshal: %w", err)
	}
	ciphertext, err := encrypt(string(body), key)
	if err != nil {
		return fmt.Errorf("keychain Set: encrypt: %w", err)
	}
	target := filepath.Join(dir, entryFileName(service, account))
	if err := fsx.WriteFileAtomic(target, ciphertext, 0o600); err != nil {
		return fmt.Errorf("keychain Set: write: %w", err)
	}
	return nil
}

// Remove deletes a secret. No-op if the item does not exist.
func Remove(service, account string) error {
	dir, err := keychainDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, entryFileName(service, account))
	err = os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("keychain Remove: %w", err)
	}
	return nil
}

// GetLegacy reads an entry stored under the pre-v2 sanitized filename with the
// raw (non-JSON) plaintext format. Migration-only; never used on the hot path.
func GetLegacy(service, account string) (string, error) {
	dir, err := keychainDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, service+"_"+safeFileName(account)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", err
	}
	key, err := getMasterKey(false)
	if err != nil {
		return "", err
	}
	return decrypt(data, key)
}

// SetLegacy writes an entry using the pre-v2 sanitized filename and raw
// (non-JSON) plaintext format that GetLegacy reads. Test-support only: it
// exists so migration tests outside this package can build v1 keychain
// fixtures; never used on the hot path.
func SetLegacy(service, account, value string) error {
	dir, err := keychainDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("keychain SetLegacy: mkdir: %w", err)
	}
	key, err := getMasterKey(true)
	if err != nil {
		return fmt.Errorf("keychain SetLegacy: %w", err)
	}
	ciphertext, err := encrypt(value, key)
	if err != nil {
		return fmt.Errorf("keychain SetLegacy: encrypt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, service+"_"+safeFileName(account)), ciphertext, 0o600); err != nil {
		return fmt.Errorf("keychain SetLegacy: write: %w", err)
	}
	return nil
}
