package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// AccountUATKey / AccountPartnerKey / ProfileStoreKey build namespaced
// keychain account names for v2 multi-tenant storage. Identifiers are
// lowercased so lookups are case-insensitive regardless of input casing.
func AccountUATKey(email string) string {
	return "account:" + strings.ToLower(email) + ":uat"
}

func AccountPartnerKey(email string) string {
	return "account:" + strings.ToLower(email) + ":partner"
}

func ProfileStoreKey(name string) string {
	return "profile:" + strings.ToLower(name) + ":store"
}

// AccountMeta is the v2 auth/_accounts/<email>.json shape (shared with migrate).
type AccountMeta struct {
	UserID           string   `json:"user_id,omitempty"`
	UATExpiresAt     string   `json:"uat_expires_at,omitempty"`
	PartnerExpiresAt string   `json:"partner_expires_at,omitempty"`
	GrantedScopes    []string `json:"granted_scopes,omitempty"`
}

// ProfileMeta is the v2 auth/<name>.json shape.
type ProfileMeta struct {
	StoreID       string   `json:"store_id,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	GrantedScopes []string `json:"granted_scopes,omitempty"`
}

// AuthDir returns the v2 auth metadata directory next to the config file.
func AuthDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "auth")
}

func accountMetaPath(authDir, email string) string {
	return filepath.Join(authDir, "_accounts", strings.ToLower(email)+".json")
}

func profileMetaPath(authDir, name string) string {
	return filepath.Join(authDir, strings.ToLower(name)+".json")
}

func LoadAccountMeta(authDir, email string) (AccountMeta, error) {
	var m AccountMeta
	if err := loadJSON(accountMetaPath(authDir, email), &m); err != nil {
		return AccountMeta{}, err
	}
	return m, nil
}

func SaveAccountMeta(authDir, email string, m AccountMeta) error {
	return saveJSON(accountMetaPath(authDir, email), m)
}

func LoadProfileMeta(authDir, name string) (ProfileMeta, error) {
	var m ProfileMeta
	if err := loadJSON(profileMetaPath(authDir, name), &m); err != nil {
		return ProfileMeta{}, err
	}
	return m, nil
}

func SaveProfileMeta(authDir, name string, m ProfileMeta) error {
	return saveJSON(profileMetaPath(authDir, name), m)
}

func RemoveProfileMeta(authDir, name string) error {
	err := os.Remove(profileMetaPath(authDir, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// loadJSON reads and decodes path into v; missing file leaves v at its zero value.
func loadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, v)
}

// saveJSON writes v to path atomically: MkdirAll 0o700, write tmp 0o600, rename.
func saveJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
