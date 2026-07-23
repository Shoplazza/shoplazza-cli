package auth

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

var allScopes = []string{"read_product", "write_product"}

// authDir is the v2 auth metadata directory for a test factory.
func authDir(f *cmdutil.Factory) string { return internalauth.AuthDir(f.ConfigPath) }

// future returns a timestamp well past any near-expiry margin.
func future() time.Time { return time.Now().Add(time.Hour) }

// seedLoggedInWithProfiles builds an isolated Factory with account email
// already logged in (uat seeded in keychain) and one profile per storeName,
// each bound to "<name>.myshoplazza.com" with the full allScopes set.
func seedLoggedInWithProfiles(t *testing.T, email string, storeNames ...string) *cmdutil.Factory {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	configPath := filepath.Join(dir, "config.json")

	cfg := core.CliConfig{
		Accounts: []core.AccountConfig{{Name: strings.ToLower(email), GrantedScopes: allScopes}},
	}
	for _, name := range storeNames {
		cfg.Profiles = append(cfg.Profiles, core.ProfileConfig{
			Name:        name,
			Account:     strings.ToLower(email),
			StoreDomain: name + ".myshoplazza.com",
			Scopes:      append([]string{}, allScopes...),
		})
	}
	if len(cfg.Profiles) > 0 {
		cfg.CurrentProfile = cfg.Profiles[0].Name
	}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey(email), "uat-seed"); err != nil {
		t.Fatalf("seed account uat: %v", err)
	}

	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		ConfigPath: configPath,
		Config:     cfg,
	}
}

// loginResultFor builds a minimal LoginResult carrying just what
// SyncAfterLogin reads: Status.Account and Status.GrantedScopes.
func loginResultFor(email string, scopes []string) internalauth.LoginResult {
	return internalauth.LoginResult{Status: internalauth.Status{Account: email, GrantedScopes: scopes, LoggedIn: true}}
}

// seedProfileToken persists a profile's cached store access token: the
// keychain entry plus its ProfileMeta (expiry), matching a real exchange.
func seedProfileToken(t *testing.T, dir, name, token string, expiresAt time.Time) {
	t.Helper()
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(name), token); err != nil {
		t.Fatalf("seed profile token: %v", err)
	}
	if err := internalauth.SaveProfileMeta(dir, strings.ToLower(name), internalauth.ProfileMeta{
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed profile meta: %v", err)
	}
}

// seedExtraProfile appends a profile bound to storeDomain under the same
// account, used to manufacture a derived-name conflict.
func seedExtraProfile(t *testing.T, f *cmdutil.Factory, name, storeDomain string) {
	t.Helper()
	cfg, err := core.LoadConfig(f.ConfigPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Profiles = append(cfg.Profiles, core.ProfileConfig{
		Name:        name,
		Account:     cfg.Account().Name,
		StoreDomain: storeDomain,
		Scopes:      allScopes,
	})
	if err := core.SaveConfig(f.ConfigPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

// RLG-01/02/03: same-account re-login keeps profiles but clears their cached
// store tokens (the Gate re-mints lazily on next use).
func TestSync_ReLogin_KeepsProfilesClearsTokens(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us", "cn")
	seedProfileToken(t, authDir(f), "us", "at-us", future())
	_, err := SyncAfterLogin(f, loginResultFor("alice@co.com", allScopes), "", nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if len(cfg.Profiles) != 2 {
		t.Fatal("profiles must survive re-login")
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("us")); err != nil || v != "" {
		t.Fatalf("cached ATs must be cleared on re-login, got v=%q err=%v", v, err)
	}
}

// Re-login with a narrower granted set trims each profile's scopes to the
// intersection, with exactly one stderr warning per trimmed profile.
func TestSync_ScopeNarrowing_TrimsWithOneWarning(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us") // us.Scopes = allScopes
	var buf bytes.Buffer
	_, _ = SyncAfterLogin(f, loginResultFor("alice@co.com", []string{"read_product"}), "", nil, &buf)
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if got := cfg.FindProfile("us").Scopes; len(got) != 1 || got[0] != "read_product" {
		t.Fatalf("trim to intersection: %v", got)
	}
	if n := strings.Count(buf.String(), "trimmed"); n != 1 {
		t.Fatalf("exactly one warning, got %d: %s", n, buf.String())
	}
}

// Logging in as a different account cascade-wipes the old account: its
// profiles and credentials are gone, the new account is installed alone.
func TestSync_AccountSwitch_CascadeWipes(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us")
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountPartnerKey("alice@co.com"), "partner-seed"); err != nil {
		t.Fatalf("seed alice partner token: %v", err)
	}
	if err := internalauth.SaveAccountMeta(authDir(f), "alice@co.com", internalauth.AccountMeta{UserID: "u1"}); err != nil {
		t.Fatalf("seed alice account meta: %v", err)
	}
	_, _ = SyncAfterLogin(f, loginResultFor("bob@co.com", allScopes), "", nil, io.Discard)
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if len(cfg.Profiles) != 0 || cfg.Account().Name != "bob@co.com" {
		t.Fatalf("old account must be wiped: %+v", cfg)
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@co.com")); err != nil || v != "" {
		t.Fatalf("alice credentials must be removed, got v=%q err=%v", v, err)
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.AccountPartnerKey("alice@co.com")); err != nil || v != "" {
		t.Fatalf("alice partner token must be removed, got v=%q err=%v", v, err)
	}
	metaPath := filepath.Join(authDir(f), "_accounts", "alice@co.com.json")
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatalf("alice account meta file must be removed, err=%v", err)
	}
}

// CRT-01/03: login with --store-domain derives the profile name from the
// domain, and a name collision falls back to a "-2" suffix.
func TestSync_NewStore_DerivedNameAndConflictSuffix(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com") // no profile yet
	_, _ = SyncAfterLogin(f, loginResultFor("alice@co.com", allScopes), "us.myshoplazza.com", nil, io.Discard)
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.CurrentProfile != "us" {
		t.Fatalf("derived name: %+v", cfg)
	}
	// Manufacture a derived-name collision: a profile already named "cn" bound
	// to a different store.
	seedExtraProfile(t, f, "cn", "other.myshoplazza.com")
	_, _ = SyncAfterLogin(f, loginResultFor("alice@co.com", allScopes), "cn.myshoplazza.com", nil, io.Discard)
	cfg, _ = core.LoadConfig(f.ConfigPath)
	if cfg.FindProfile("cn-2") == nil || cfg.CurrentProfile != "cn-2" {
		t.Fatalf("conflict suffix: %+v", cfg)
	}
}

// SRV-04 (adapted): SyncAfterLogin never mints or persists a profile store
// token itself — minting is the Gate's job (AccessTokenReadyForProfile),
// lazily, on demand. Requesting a scope subset for a brand-new store profile
// must not leave anything in the profile's keychain slot.
func TestSync_ScopeSubset_IgnoresPrewarmToken(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com")
	res := loginResultFor("alice@co.com", allScopes)
	_, _ = SyncAfterLogin(f, res, "us.myshoplazza.com", []string{"read_product"}, io.Discard)
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("us")); err != nil || v != "" {
		t.Fatalf("SyncAfterLogin must never write a profile store token, got v=%q err=%v", v, err)
	}
}

// equalFoldSlice is a set comparison: order and case must not matter.
func TestEqualFoldSlice_OrderAndCaseInsensitive(t *testing.T) {
	if !equalFoldSlice([]string{"Read_Product", "write_product"}, []string{"write_Product", "read_product"}) {
		t.Fatal("same scopes in different order/case must compare equal")
	}
	if equalFoldSlice([]string{"read_product"}, []string{"read_product", "write_product"}) {
		t.Fatal("different-length sets must not compare equal")
	}
}

// CRT-05: re-selecting an existing store profile with a different scope
// subset updates it silently (no stderr output) and clears its cached AT.
func TestSync_DuplicateCreate_SilentScopeUpdate(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us") // us exists, full scopes
	var buf bytes.Buffer
	_, _ = SyncAfterLogin(f, loginResultFor("alice@co.com", allScopes),
		"us.myshoplazza.com", []string{"read_product"}, &buf)
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if got := cfg.FindProfile("us").Scopes; len(got) != 1 {
		t.Fatalf("scope updated: %v", got)
	}
	if buf.Len() != 0 {
		t.Fatalf("silent update — no output, got %q", buf.String())
	}
}
