package profile

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
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

// newTestFactory builds a temp, isolated Factory with a single logged-in
// account ("alice@co.com", granted read_product/write_product) and its UAT
// seeded in keychain. AuthClient targets srvURL (the exchange stub); pass ""
// when a test never reaches the exchange call.
func newTestFactory(t *testing.T, srvURL string) *cmdutil.Factory {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	configPath := filepath.Join(dir, "config.json")

	cfg := core.CliConfig{
		Accounts: []core.AccountConfig{
			{Name: "alice@co.com", GrantedScopes: []string{"read_product", "write_product"}},
		},
	}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@co.com"), "uat-1"); err != nil {
		t.Fatalf("seed account uat: %v", err)
	}

	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		ConfigPath: configPath,
		Config:     cfg,
		Client:     client.New(""),
		AuthClient: client.New(srvURL),
	}
}

// execProfile runs the profile command tree with args, capturing stdout.
func execProfile(f *cmdutil.Factory, args ...string) (string, error) {
	out := &bytes.Buffer{}
	cmd := NewCmdProfile(f)
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return out.String(), err
}

// runCmd executes args and fails the test on any error, returning stdout.
func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	out, err := execProfile(f, args...)
	if err != nil {
		t.Fatalf("cmd %v: unexpected error: %v (out=%s)", args, err, out)
	}
	return out
}

// runCmdErr executes args and fails the test if it DIDN'T error.
func runCmdErr(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	_, err := execProfile(f, args...)
	if err == nil {
		t.Fatalf("cmd %v: expected an error, got none", args)
	}
	return err
}

// seedTwoProfiles seeds a factory with two profiles (named a, b) bound to
// the fixture account, current=a, previous unset. It persists to disk since
// lifecycle commands read config fresh from f.ConfigPath under the lock.
func seedTwoProfiles(t *testing.T, a, b string) *cmdutil.Factory {
	t.Helper()
	f := newTestFactory(t, "")
	f.Config.Profiles = []core.ProfileConfig{
		{Name: a, Account: "alice@co.com", StoreDomain: a + ".myshoplazza.com", Scopes: []string{"read_product", "write_product"}},
		{Name: b, Account: "alice@co.com", StoreDomain: b + ".myshoplazza.com", Scopes: []string{"read_product", "write_product"}},
	}
	f.Config.CurrentProfile = a
	if err := core.SaveConfig(f.ConfigPath, f.Config); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}
	return f
}

// seedProfileToken persists a profile's cached store access token: the
// keychain entry plus its ProfileMeta (expiry), matching what a real
// exchange would have written.
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

// setPreviousProfile overrides the persisted previousProfile pointer for
// tests that need a specific one pre-set (seedTwoProfiles leaves it empty).
func setPreviousProfile(t *testing.T, f *cmdutil.Factory, name string) {
	t.Helper()
	f.Config.PreviousProfile = name
	if err := core.SaveConfig(f.ConfigPath, f.Config); err != nil {
		t.Fatalf("set previous profile: %v", err)
	}
}
