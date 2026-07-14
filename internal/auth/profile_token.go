package auth

import (
	"context"
	"errors"
	"strings"

	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
)

// AccountUAT reads the v2-namespaced UAT for email.
func (m *Manager) AccountUAT(email string) (string, error) {
	v, err := keychain.Get(keychain.ShoplazzaCliService, AccountUATKey(email))
	if err != nil || v == "" {
		return "", errors.New("no UAT available — please run 'shoplazza auth login'")
	}
	return v, nil
}

// ExchangeForProfile mints a store AT for p (scopes = p.Scopes; empty means
// full grant), persists the token to keychain[ProfileStoreKey] plus its
// ProfileMeta, and returns the token.
func (m *Manager) ExchangeForProfile(ctx context.Context, authDir string, p core.ProfileConfig) (string, error) {
	uat, err := m.AccountUAT(p.Account)
	if err != nil {
		return "", err
	}
	block, err := m.exchangeStoreATScoped(ctx, uat, p.StoreDomain, p.Scopes)
	if err != nil {
		return "", err
	}
	if err := PersistProfileToken(authDir, p.Name, &block); err != nil {
		return "", err
	}
	return block.AccessToken, nil
}

// PersistProfileToken stores a minted store token under the profile's
// keychain key and writes its meta. Shared by ExchangeForProfile and the
// login flow (which mints before the profile row exists).
func PersistProfileToken(authDir, profileName string, block *storeATBlock) error {
	if err := keychain.Set(keychain.ShoplazzaCliService, ProfileStoreKey(profileName), block.AccessToken); err != nil {
		return err
	}
	return SaveProfileMeta(authDir, strings.ToLower(profileName), ProfileMeta{
		StoreID: block.StoreID, ExpiresAt: block.ATExpiresAt, GrantedScopes: block.GrantedScopes,
	})
}

// ExchangeEphemeral mints a token for an arbitrary owned domain WITHOUT any
// persistence (te -s ad-hoc).
func (m *Manager) ExchangeEphemeral(ctx context.Context, storeDomain string) (string, error) {
	acct := m.Config.Account()
	if acct == nil {
		return "", errors.New("not logged in")
	}
	uat, err := m.AccountUAT(acct.Name)
	if err != nil {
		return "", err
	}
	block, err := m.exchangeStoreATScoped(ctx, uat, storeDomain, nil)
	if err != nil {
		return "", err
	}
	return block.AccessToken, nil
}
