// Package cmdtest holds test fixtures shared by the cmd/* command packages.
// It is test-only support code; nothing in the built CLI imports it.
package cmdtest

import (
	"io"
	"strings"
	"testing"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

// FixtureScopes is the scope set the seeded fixtures grant.
var FixtureScopes = []string{"read_product", "write_product"}

// SeedLoggedInWithProfiles builds an isolated Factory with account email
// already logged in (uat seeded in the keychain) and one profile per
// storeName, each bound to "<name>.myshoplazza.com" with FixtureScopes
// granted. The first store becomes the current profile.
func SeedLoggedInWithProfiles(t *testing.T, email string, storeNames ...string) *cmdutil.Factory {
	t.Helper()
	configPath, _ := testenv.ConfigPaths(t)

	cfg := core.CliConfig{
		Accounts: []core.AccountConfig{{Name: strings.ToLower(email), GrantedScopes: FixtureScopes}},
	}
	for _, name := range storeNames {
		cfg.Profiles = append(cfg.Profiles, core.ProfileConfig{
			Name:        name,
			Account:     strings.ToLower(email),
			StoreDomain: name + ".myshoplazza.com",
			Scopes:      append([]string{}, FixtureScopes...),
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

	// Both clients are live but base-URL-less: the auth gate rebases them per
	// profile, and a nil client there is a panic rather than a test failure.
	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		ConfigPath: configPath,
		Config:     cfg,
		Client:     client.New(""),
		AuthClient: client.New(""),
	}
}

// SeedProfileToken puts a store token for profile name in the keychain and
// records its expiry in the profile meta under authDir.
func SeedProfileToken(t *testing.T, authDir, name, token string, expiresAt time.Time) {
	t.Helper()
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(name), token); err != nil {
		t.Fatalf("seed profile token: %v", err)
	}
	if err := internalauth.SaveProfileMeta(authDir, strings.ToLower(name), internalauth.ProfileMeta{
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed profile meta: %v", err)
	}
}
