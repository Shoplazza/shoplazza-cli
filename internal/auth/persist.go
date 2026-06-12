package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
)

const (
	kcUAT     = "uat"
	kcPartner = "partner"
)

// storeKcKey / appKcKey build the resource-scoped keychain account names.
// Account-level tokens (uat, partner) use the bare kind as the account name;
// resource-level tokens carry a "<kind>:<id>" suffix so one host can hold
// many stores / apps without collision.
func storeKcKey(domain string) string { return "store:" + domain }
func appKcKey(clientID string) string { return "app:" + clientID }

func (m *Manager) persistState(state AuthState) error {
	meta := authMeta{
		Account:          state.Account,
		UserID:           state.UserID,
		UATExpiresAt:     state.UATExpiresAt,
		PartnerExpiresAt: state.PartnerExpiresAt,
		GrantedScopes:    state.GrantedScopes,
		Stores:           map[string]StoreTokenMeta{},
		Apps:             map[string]AppTokenMeta{},
	}
	for dom, s := range state.Stores {
		meta.Stores[dom] = StoreTokenMeta{StoreID: s.StoreID, ExpiresAt: s.ExpiresAt, GrantedScopes: s.GrantedScopes}
	}
	for id, a := range state.Apps {
		meta.Apps[id] = AppTokenMeta{ExpiresAt: a.ExpiresAt}
	}
	if err := saveAuthMeta(m.AuthPath, meta); err != nil {
		return err
	}
	if state.UAT != "" {
		if err := keychain.Set(keychain.ShoplazzaCliService, kcUAT, state.UAT); err != nil {
			return err
		}
	}
	if state.Partner != "" {
		if err := keychain.Set(keychain.ShoplazzaCliService, kcPartner, state.Partner); err != nil {
			return err
		}
	} else {
		// A login that yields no partner token (e.g. --uat, or a failed
		// best-effort mint) must not leave a stale partner credential behind:
		// LoadState reads kcPartner unconditionally, so a lingering entry would be
		// resurrected. Store-token operations (UseStore/RefreshAccessToken)
		// LoadState first, so a genuinely-present partner is preserved, not dropped.
		_ = keychain.Remove(keychain.ShoplazzaCliService, kcPartner)
	}
	for dom, s := range state.Stores {
		if s.Token != "" {
			if err := keychain.Set(keychain.ShoplazzaCliService, storeKcKey(dom), s.Token); err != nil {
				return err
			}
		}
	}
	for id, a := range state.Apps {
		if a.Token != "" {
			if err := keychain.Set(keychain.ShoplazzaCliService, appKcKey(id), a.Token); err != nil {
				return err
			}
		}
	}
	cfg := m.Config
	// Pure-account login (no store) leaves CurrentStore == "" → do NOT clobber an
	// existing current store. Only logout clears it (see Logout).
	if state.CurrentStore != "" {
		cfg.StoreDomain = state.CurrentStore
		if err := core.SaveConfig(m.ConfigPath, cfg); err != nil {
			return err
		}
		m.Config = cfg
	}
	return nil
}

func defaultAuthMetaPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "shoplazza-cli", "auth.json"), nil
}

func loadAuthMeta(path string) (authMeta, error) {
	if path == "" {
		var err error
		path, err = defaultAuthMetaPath()
		if err != nil {
			return authMeta{}, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return authMeta{}, nil
		}
		return authMeta{}, err
	}
	var meta authMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return authMeta{}, err
	}
	return meta, nil
}

func saveAuthMeta(path string, meta authMeta) error {
	if path == "" {
		var err error
		path, err = defaultAuthMetaPath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func removeAuthMeta(path string) error {
	if path == "" {
		var err error
		path, err = defaultAuthMetaPath()
		if err != nil {
			return err
		}
	}
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
