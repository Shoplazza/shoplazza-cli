package api

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

func TestNewCmdAPI_HasRestSubcommand(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdAPI(f)
	if cmd.Use != "api" {
		t.Errorf("Use = %q, want api", cmd.Use)
	}
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "rest <method> <path>" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'rest' subcommand under api")
	}
}

func TestNewCmdRest_RegistersFlags(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := newCmdRest(f)
	for _, name := range []string{"params", "data", "dry-run", "jq"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected --%s flag on rest command", name)
		}
	}
}

// TestNewCmdRest_RunE_DryRun exercises buildRawRequest and the dry-run output path.
func TestNewCmdRest_RunE_DryRun(t *testing.T) {
	f := &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		Client:    client.New("http://localhost"),
	}
	cmd := newCmdRest(f)
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, []string{"GET", "/orders"}); err != nil {
		t.Errorf("unexpected error in dry-run: %v", err)
	}
}

// TestNewCmdRest_RunE_DryRun_WithParams covers buildRawRequest with params.
func TestNewCmdRest_RunE_DryRun_WithParams(t *testing.T) {
	f := &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		Client:    client.New("http://localhost"),
	}
	cmd := newCmdRest(f)
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("dry-run", "true")
	_ = cmd.Flags().Set("params", `{"limit":10}`)
	if err := cmd.RunE(cmd, []string{"GET", "/products"}); err != nil {
		t.Errorf("unexpected error in dry-run with params: %v", err)
	}
}

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
