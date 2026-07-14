// Package migrate performs the one-time v1 → v2 config/credential migration.
// Principle: migrate only non-regenerable credentials (uat, partner); derived
// tokens (store/app) are dropped and lazily re-minted. Store CONTEXTS are
// preserved: every v1 store becomes a profile (with its store_id). v1 files
// stay on disk so users can downgrade (cleanup in a later release).
package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/fsx"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/lockfile"
)

// legacyAuthMeta mirrors the v1 auth.json shape (only the fields migration reads).
type legacyAuthMeta struct {
	Account       string                     `json:"account"`
	UserID        string                     `json:"user_id"`
	UATExpiresAt  string                     `json:"uat_expires_at"`
	GrantedScopes []string                   `json:"granted_scopes"`
	Stores        map[string]legacyStoreMeta `json:"stores"`
}

// legacyStoreMeta is the per-store slice of v1 auth.json migration keeps.
type legacyStoreMeta struct {
	StoreID string `json:"store_id"`
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
		// 2) keychain: migrate only uat/partner, never overwriting a v2 entry.
		if err := copyLegacyIfAbsent("uat", auth.AccountUATKey(email)); err != nil {
			return err
		}
		if err := copyLegacyIfAbsent("partner", auth.AccountPartnerKey(email)); err != nil {
			return err
		}
		// 3) v2 account metadata
		if err := writeAccountMeta(dir, email, accountMeta{
			UserID: meta.UserID, UATExpiresAt: meta.UATExpiresAt, GrantedScopes: meta.GrantedScopes,
		}); err != nil {
			return err
		}
		// 4) profiles: one per v1 store (auth.json stores map, sorted for
		// deterministic naming) plus the legacy current store. Tokens are NOT
		// migrated — they re-mint lazily on first use; store_id rides along.
		taken := func(n string) bool {
			for i := range out.Profiles {
				if strings.EqualFold(out.Profiles[i].Name, n) {
					return true
				}
			}
			return false
		}
		addProfile := func(domain, storeID string) string {
			for i := range out.Profiles {
				if strings.EqualFold(out.Profiles[i].StoreDomain, domain) {
					if out.Profiles[i].StoreID == "" {
						out.Profiles[i].StoreID = storeID
					}
					return out.Profiles[i].Name
				}
			}
			name := core.DeriveProfileName(domain, taken)
			out.Profiles = append(out.Profiles, core.ProfileConfig{Name: name, Account: email, StoreDomain: domain, StoreID: storeID})
			return name
		}
		domains := make([]string, 0, len(meta.Stores))
		for domain := range meta.Stores {
			if domain != "" {
				domains = append(domains, domain)
			}
		}
		sort.Strings(domains)
		for _, domain := range domains {
			addProfile(domain, meta.Stores[domain].StoreID)
		}
		// The legacy current store stays current (store_domain is no longer on
		// core.CliConfig, so read it straight from the raw JSON); without one,
		// a sole migrated store becomes current, mirroring 'profile add'.
		if storeDomain := readLegacyStoreDomain(configPath); storeDomain != "" {
			out.CurrentProfile = addProfile(storeDomain, "")
		} else if len(out.Profiles) == 1 {
			out.CurrentProfile = out.Profiles[0].Name
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

// copyLegacyIfAbsent copies a legacy keychain entry to its v2 key unless the
// v2 key already holds a value. Missing legacy entries are tolerated.
func copyLegacyIfAbsent(legacyAccount, v2Account string) error {
	if existing, err := keychain.Get(keychain.ShoplazzaCliService, v2Account); err == nil && existing != "" {
		return nil
	}
	v, err := keychain.GetLegacy(keychain.ShoplazzaCliService, legacyAccount)
	if err != nil || v == "" {
		return nil
	}
	return keychain.Set(keychain.ShoplazzaCliService, v2Account, v)
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
	return fsx.WriteFileAtomic(p, data, 0o600)
}
