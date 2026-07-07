package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

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
	// A login/refresh that carries no partner token must not silently drop an
	// existing one for the SAME account. A routine store-scoped login (whose poll
	// returns no partner_token) or a --uat refresh would otherwise cost you your
	// partner session and force an interactive re-login for every app command.
	// Preserve it here; it is only cleared on an account switch (below — else
	// LoadState would resurrect another account's partner token) or on explicit
	// logout (Logout removes it directly).
	partner, partnerExpiresAt := state.Partner, state.PartnerExpiresAt
	if partner == "" {
		// Match case-insensitively: poll and Me may echo the same email in
		// different casing, and a mismatch would wrongly wipe a valid token.
		if prev, err := loadAuthMeta(m.AuthPath); err == nil && prev.Account != "" && strings.EqualFold(prev.Account, state.Account) {
			if existing, gerr := keychain.Get(keychain.ShoplazzaCliService, kcPartner); gerr == nil && existing != "" {
				partner, partnerExpiresAt = existing, prev.PartnerExpiresAt
			}
		}
	}
	meta := authMeta{
		Account:          state.Account,
		UserID:           state.UserID,
		UATExpiresAt:     state.UATExpiresAt,
		PartnerExpiresAt: partnerExpiresAt,
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
	if partner != "" {
		if err := keychain.Set(keychain.ShoplazzaCliService, kcPartner, partner); err != nil {
			return err
		}
	} else {
		// Empty here means either a first login with no partner token, or an
		// account switch — drop any lingering entry so LoadState can't resurrect
		// a different account's partner token.
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
