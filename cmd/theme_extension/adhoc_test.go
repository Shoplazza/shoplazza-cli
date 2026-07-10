package theme_extension

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/testenv"
)

// newExchangeStub returns an httptest server stubbing the store-AT exchange
// endpoint, always returning accessToken. Local copy of
// internal/auth/profile_token_test.go's helper of the same name (unexported,
// cross-package) — same envelope shape.
func newExchangeStub(t *testing.T, accessToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"access_token": accessToken, "store_id": "1",
			"store_domain": "cn.myshoplazza.com", "granted_scopes": []string{"read_product"},
			"at_expires_at": "2099-01-01T00:00:00Z",
		}})
	}))
}

// allScopes mirrors cmd/auth/profile_sync_test.go's fixture scope set.
var teAllScopes = []string{"read_product", "write_product"}

// seedLoggedInWithProfiles builds an isolated Factory with account email
// already logged in (uat seeded in keychain) and one profile per storeName,
// each bound to "<name>.myshoplazza.com". Local copy of cmd/auth's helper of
// the same name (unexported, cross-package) — same pattern, no AuthClient set
// (callers point it at their own exchange stub). Also seeds the legacy
// keychain "uat" entry: te's requireLogin gate still checks v1 CurrentStatus
// (state.UAT), which a real `auth login` always populates alongside the v2
// account/profile state (Manager.Login → persistState, then SyncAfterLogin).
func seedLoggedInWithProfiles(t *testing.T, email string, storeNames ...string) *cmdutil.Factory {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	configPath := filepath.Join(dir, "config.json")

	cfg := core.CliConfig{
		Accounts: []core.AccountConfig{{Name: strings.ToLower(email), GrantedScopes: teAllScopes}},
	}
	for _, name := range storeNames {
		cfg.Profiles = append(cfg.Profiles, core.ProfileConfig{
			Name:        name,
			Account:     strings.ToLower(email),
			StoreDomain: name + ".myshoplazza.com",
			Scopes:      append([]string{}, teAllScopes...),
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
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat-seed"); err != nil {
		t.Fatalf("seed legacy uat: %v", err)
	}

	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		ConfigPath: configPath,
		Config:     cfg,
	}
}

// runTECmd executes the te command tree with args against f, discarding
// output. The ad-hoc tests only assert post-command config/keychain state —
// the mocked exchange server stands in for the auth exchange only; there is
// no real store behind the fake domain, so the command's own exit status
// (the final store-openapi call against the fake domain) is irrelevant here
// and deliberately ignored. A short context deadline keeps that doomed
// network hop from ever stalling the test.
func runTECmd(t *testing.T, f *cmdutil.Factory, args ...string) {
	t.Helper()
	cmd := NewCmdThemeExtension(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd.SetContext(ctx)
	_ = cmd.Execute()
}

// TestTE_AdhocDomain_NoPersistence: `te list -s cn.myshoplazza.com` with only
// a "us" profile on file must mint via ExchangeEphemeral — no "cn" profile is
// created and no token is persisted under its keychain slot (tech design
// §4.2, zero residue for an ad-hoc domain).
func TestTE_AdhocDomain_NoPersistence(t *testing.T) {
	t.Setenv(envAccessToken, "") // force the profile/ephemeral path, not the env bypass
	srv := newExchangeStub(t, "at-tmp")
	defer srv.Close()
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us") // only us
	f.AuthClient = client.New(srv.URL)

	runTECmd(t, f, "list", "-s", "cn.myshoplazza.com")

	cfg, err := core.LoadConfig(f.ConfigPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("ad-hoc must not create a profile, got %d profiles: %+v", len(cfg.Profiles), cfg.Profiles)
	}
	if v, gErr := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("cn")); gErr != nil || v != "" {
		t.Fatalf("ad-hoc must not persist a token, got v=%q err=%v", v, gErr)
	}
}

// TestTE_DomainMatchesProfile_UsesProfileCreds: `-s us.myshoplazza.com`
// matching the existing "us" profile must go through
// AccessTokenReadyForProfile — the exchange stub's token ends up cached under
// the profile's own keychain slot (proving the mint+persist path ran, not the
// ephemeral one).
func TestTE_DomainMatchesProfile_UsesProfileCreds(t *testing.T) {
	t.Setenv(envAccessToken, "") // force the profile/ephemeral path, not the env bypass
	srv := newExchangeStub(t, "at-us")
	defer srv.Close()
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us")
	f.AuthClient = client.New(srv.URL)

	runTECmd(t, f, "list", "-s", "us.myshoplazza.com")

	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("us")); err != nil || v != "at-us" {
		t.Fatalf("expected profile us token minted+persisted as at-us, got v=%q err=%v", v, err)
	}
}
