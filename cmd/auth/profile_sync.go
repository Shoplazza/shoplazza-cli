package auth

import (
	"fmt"
	"io"
	"strings"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
)

// SyncAfterLogin applies §5.1 after a successful login/store-use: same-account
// re-login keeps profiles but clears their cached store tokens (trimming any
// scope narrowed by re-login, with one stderr warning per trimmed profile);
// an account switch cascade-wipes the old account's profiles and credentials;
// and, when storeDomain is set, the target profile is created or — if it
// already exists — silently updated when the requested scope subset differs.
//
// SyncAfterLogin never mints or persists a store access token itself: that is
// the Gate's job (AccessTokenReadyForProfile), done lazily on next use.
func SyncAfterLogin(f *cmdutil.Factory, res internalauth.LoginResult, storeDomain string, scopes []string, errOut io.Writer) error {
	email := strings.ToLower(res.Status.Account)
	granted := res.Status.GrantedScopes
	return core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
		authDir := internalauth.AuthDir(f.ConfigPath)
		switch {
		case c.Account() == nil: // brand-new login
			c.Accounts = []core.AccountConfig{{Name: email, GrantedScopes: granted}}
		case c.Account().Name != email: // switching accounts: wipe the old one first
			wipeAccount(f, c)
			c.Accounts = []core.AccountConfig{{Name: email, GrantedScopes: granted}}
		default: // re-login to the same account
			c.Accounts[0].GrantedScopes = granted
			for i := range c.Profiles {
				_ = keychain.Remove(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(c.Profiles[i].Name))
				_ = internalauth.RemoveProfileMeta(authDir, strings.ToLower(c.Profiles[i].Name))
				if trimmed, changed := intersect(c.Profiles[i].Scopes, granted); changed {
					c.Profiles[i].Scopes = trimmed
					fmt.Fprintf(errOut, "warning: profile %q scopes trimmed to granted set: %s\n",
						c.Profiles[i].Name, strings.Join(trimmed, ","))
				}
			}
		}

		if storeDomain == "" {
			return nil // pure account login/re-login: no profile to touch
		}
		if p := c.FindProfileByStore(storeDomain); p != nil {
			if scopes != nil && !equalFoldSlice(p.Scopes, scopes) {
				p.Scopes = scopes // silent update: no output, just clear the cached AT
				_ = keychain.Remove(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(p.Name))
				_ = internalauth.RemoveProfileMeta(authDir, strings.ToLower(p.Name))
			}
			c.PreviousProfile, c.CurrentProfile = c.CurrentProfile, p.Name
			return nil
		}
		name := core.DeriveProfileName(storeDomain, func(n string) bool { return c.FindProfile(n) != nil })
		c.Profiles = append(c.Profiles, core.ProfileConfig{
			Name: name, Account: email, StoreDomain: storeDomain, Scopes: scopes,
		})
		c.PreviousProfile, c.CurrentProfile = c.CurrentProfile, name
		return nil
	})
}

// wipeAccount cascades an account switch or logout: clears every profile's
// cached store token/meta, then the current account's credentials/meta, then
// blanks the account/profile lists. Caller installs a new account afterward
// (account switch) or leaves it blank (logout).
func wipeAccount(f *cmdutil.Factory, c *core.CliConfig) {
	authDir := internalauth.AuthDir(f.ConfigPath)
	for i := range c.Profiles {
		_ = keychain.Remove(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(c.Profiles[i].Name))
		_ = internalauth.RemoveProfileMeta(authDir, strings.ToLower(c.Profiles[i].Name))
	}
	if old := c.Account(); old != nil {
		_ = keychain.Remove(keychain.ShoplazzaCliService, internalauth.AccountUATKey(old.Name))
		_ = keychain.Remove(keychain.ShoplazzaCliService, internalauth.AccountPartnerKey(old.Name))
		_ = internalauth.RemoveAccountMeta(authDir, strings.ToLower(old.Name))
	}
	c.Accounts = nil
	c.Profiles = nil
	c.CurrentProfile = ""
	c.PreviousProfile = ""
}

// wipeV2OnLogout clears the v2 model on logout — the same cascade as an
// account switch, but installs no new account afterward.
func wipeV2OnLogout(f *cmdutil.Factory) error {
	return core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
		wipeAccount(f, c)
		return nil
	})
}

// intersect returns the elements of a that also appear in granted
// (case-sensitive, order preserved), and whether that trimmed anything.
func intersect(a, granted []string) (trimmed []string, changed bool) {
	set := make(map[string]struct{}, len(granted))
	for _, s := range granted {
		set[s] = struct{}{}
	}
	trimmed = make([]string, 0, len(a))
	for _, s := range a {
		if _, ok := set[s]; ok {
			trimmed = append(trimmed, s)
		}
	}
	return trimmed, len(trimmed) != len(a)
}

// equalFoldSlice reports whether a and b hold the same multiset of scopes,
// compared case-insensitively and independent of order.
func equalFoldSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	count := make(map[string]int, len(a))
	for _, s := range a {
		count[strings.ToLower(s)]++
	}
	for _, s := range b {
		count[strings.ToLower(s)]--
	}
	for _, c := range count {
		if c != 0 {
			return false
		}
	}
	return true
}
