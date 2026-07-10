package api

import (
	"bytes"
	"context"
	"io"
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

// seedLoggedInWithProfiles builds an isolated Factory with account email
// already logged in (uat seeded in keychain) and one profile per storeName,
// each bound to "<name>.myshoplazza.com". Mirrors cmd/auth's test helper of
// the same name.
func seedLoggedInWithProfiles(t *testing.T, email string, storeNames ...string) *cmdutil.Factory {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	configPath := filepath.Join(dir, "config.json")

	allScopes := []string{"read_product", "write_product"}
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
		Client:     client.New(""),
		AuthClient: client.New(""),
	}
}

// seedProfileToken persists a profile's cached store access token: the
// keychain entry plus its ProfileMeta (expiry), matching a real exchange.
func seedProfileToken(t *testing.T, authDir, name, token string, expiresAt time.Time) {
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

// runAPICmd runs the api command tree with args, capturing stdout, and fails
// the test on any RunE error.
func runAPICmd(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	cmd := NewCmdAPI(f)
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api %v: unexpected error: %v", args, err)
	}
	return buf.String()
}

// GATE-10: dry-run still goes through the Gate, so it can print the full
// resolved URL (profile base URL) without sending a real request.
func TestAPIRest_DryRun_PrintsProfileBaseURL(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us")
	seedProfileToken(t, internalauth.AuthDir(f.ConfigPath), "us", "at-1", time.Now().Add(time.Hour))
	out := runAPICmd(t, f, "rest", "GET", "/products.json", "--dry-run")
	if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, "us.myshoplazza.com") {
		t.Fatalf("dry-run must resolve URL through profile: %s", out)
	}
}
