// Package migrate performs the one-time v1 → v2 config/credential migration.
// Principle: migrate only non-regenerable credentials (uat, partner); derived
// tokens (store/app) are dropped and lazily re-minted. v1 files stay on disk
// so users can downgrade (cleanup in a later release).
package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/lockfile"
)

// legacyAuthMeta mirrors the v1 auth.json shape (only the fields migration reads).
type legacyAuthMeta struct {
	Account       string   `json:"account"`
	UserID        string   `json:"user_id"`
	UATExpiresAt  string   `json:"uat_expires_at"`
	GrantedScopes []string `json:"granted_scopes"`
}

// accountMeta is the v2 auth/_accounts/<email>.json shape.
type accountMeta struct {
	UserID        string   `json:"user_id,omitempty"`
	UATExpiresAt  string   `json:"uat_expires_at,omitempty"`
	GrantedScopes []string `json:"granted_scopes,omitempty"`
}

// Run migrates once. Fast path: configVersion >= 2 → no-op without locking.
func Run(configPath string) error {
	cfg, err := core.LoadConfig(configPath)
	if err != nil {
		return err // a corrupt config errors loudly — no partial migration
	}
	if cfg.ConfigVersion >= 2 {
		return nil
	}
	release, err := lockfile.Acquire(filepath.Join(core.LocksDir(configPath), "config.lock"), core.ConfigLockTimeout)
	if err != nil {
		return err
	}
	defer release()
	// double-check: another process may have finished migrating
	if cfg, err = core.LoadConfig(configPath); err != nil {
		return err
	}
	if cfg.ConfigVersion >= 2 {
		return nil
	}
	return doMigrate(configPath)
}

func doMigrate(configPath string) error {
	dir := filepath.Dir(configPath)
	out := core.CliConfig{ConfigVersion: 2}

	// 1) Account: v1 auth.json is the source of truth; absent means not
	// logged in, so just bump the version number.
	meta, ok := readLegacyAuthMeta(filepath.Join(dir, "auth.json"))
	if ok && meta.Account != "" {
		email := strings.ToLower(meta.Account)
		out.Accounts = []core.AccountConfig{{Name: email, GrantedScopes: meta.GrantedScopes}}
		// 2) keychain: migrate only uat/partner (GetLegacy -> Set under the
		// new naming; a missing entry is tolerated).
		if v, err := keychain.GetLegacy(keychain.ShoplazzaCliService, "uat"); err == nil && v != "" {
			if err := keychain.Set(keychain.ShoplazzaCliService, auth.AccountUATKey(email), v); err != nil {
				return err
			}
		}
		if v, err := keychain.GetLegacy(keychain.ShoplazzaCliService, "partner"); err == nil && v != "" {
			if err := keychain.Set(keychain.ShoplazzaCliService, auth.AccountPartnerKey(email), v); err != nil {
				return err
			}
		}
		// 3) v2 account metadata
		if err := writeAccountMeta(dir, email, accountMeta{
			UserID: meta.UserID, UATExpiresAt: meta.UATExpiresAt, GrantedScopes: meta.GrantedScopes,
		}); err != nil {
			return err
		}
		// 4) current store -> the sole profile (no token migrated; other
		// stores dropped). store_domain is no longer on core.CliConfig, so
		// read it straight from the raw JSON.
		if storeDomain := readLegacyStoreDomain(configPath); storeDomain != "" {
			name := core.DeriveProfileName(storeDomain, func(string) bool { return false })
			out.Profiles = []core.ProfileConfig{{Name: name, Account: email, StoreDomain: storeDomain}}
			out.CurrentProfile = name
		}
	}

	// 5) back up the v1 config before overwriting (only if it really exists)
	if raw, err := os.ReadFile(configPath); err == nil {
		if err := os.WriteFile(configPath+".v1.bak", raw, 0o600); err != nil {
			return err
		}
	}
	return core.SaveConfig(configPath, out)
}

// readLegacyStoreDomain reads the v1 config.json's store_domain field
// directly, since core.CliConfig no longer carries it.
func readLegacyStoreDomain(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var v1 struct {
		StoreDomain string `json:"store_domain"`
	}
	if json.Unmarshal(data, &v1) != nil {
		return ""
	}
	return v1.StoreDomain
}

func readLegacyAuthMeta(path string) (legacyAuthMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return legacyAuthMeta{}, false
	}
	var m legacyAuthMeta
	if json.Unmarshal(data, &m) != nil {
		return legacyAuthMeta{}, false
	}
	return m, true
}

func writeAccountMeta(configDir, email string, m accountMeta) error {
	p := filepath.Join(configDir, "auth", "_accounts", email+".json")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
