package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/testenv"
)

// tempFactory builds a Factory rooted in an isolated config/keychain dir,
// with both Client and AuthClient pointed at srvURL.
func tempFactory(t *testing.T, srvURL string, cfg core.CliConfig) *Factory {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	return &Factory{
		ConfigPath: filepath.Join(dir, "config.json"),
		Config:     cfg,
		Client:     client.New(srvURL),
		AuthClient: client.New(srvURL),
	}
}

func exitType(t *testing.T, err error) string {
	t.Helper()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil {
		t.Fatalf("expected *output.ExitError with detail, got %v", err)
	}
	return ee.Detail.Type
}

// GATE-01: no profile configured at all is a loud (validation) error — it
// must not be mistaken for "not logged in".
func TestRequireAuth_NoProfileConfigured(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	f := tempFactory(t, "http://unused", core.CliConfig{ConfigVersion: 2})
	err := RequireAuth(context.Background(), f, newCmdWithProfileFlag())
	if err == nil || exitType(t, err) != output.TypeValidation {
		t.Errorf("expected type=validation for no profile configured, got %v", err)
	}
}

// GATE-02: a resolvable profile with no UAT in the keychain fails at mint
// time with an auth-class error ("not logged in", in v2 terms).
func TestRequireAuth_NotLoggedIn(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	cfg := core.CliConfig{ConfigVersion: 2, CurrentProfile: "us",
		Profiles: []core.ProfileConfig{{Name: "us", Account: "a@co.com", StoreDomain: "shop.com"}}}
	f := tempFactory(t, "http://unused", cfg) // fresh keychain -> no UAT for a@co.com
	err := RequireAuth(context.Background(), f, newCmdWithProfileFlag())
	if err == nil || exitType(t, err) != output.TypeAuth {
		t.Errorf("expected type=auth, got %v", err)
	}
}

// GATE-04: a resolvable, logged-in profile mints its store token and injects
// it (plus the store base URL) onto f.Client.
func TestRequireAuth_Success_SetsBearer(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
				"access_token": "at_bearer", "store_id": "1", "store_domain": "shop.com",
				"granted_scopes": []string{"read_product"}, "at_expires_at": "2099-01-01T00:00:00Z",
			}})
		}
	}))
	defer srv.Close()

	cfg := core.CliConfig{ConfigVersion: 2, CurrentProfile: "us",
		Profiles: []core.ProfileConfig{{Name: "us", Account: "a@co.com", StoreDomain: "shop.com"}}}
	f := tempFactory(t, srv.URL, cfg)
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("a@co.com"), "uat_seed"); err != nil {
		t.Fatal(err)
	}
	if err := RequireAuth(context.Background(), f, newCmdWithProfileFlag()); err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}
	if f.Client.Headers["Access-Token"] != "at_bearer" {
		t.Errorf("bearer not set: %v", f.Client.Headers)
	}
	if !strings.HasPrefix(f.Client.BaseURL, "https://shop.com") {
		t.Errorf("base URL = %q, want https://shop.com prefix", f.Client.BaseURL)
	}
}

// GATE-05/06/07/08: CI bypass matrix.
func TestRequireAuth_BypassMatrix(t *testing.T) {
	withProfile := core.CliConfig{ConfigVersion: 2, CurrentProfile: "us",
		Profiles: []core.ProfileConfig{{Name: "us", Account: "a@co.com", StoreDomain: "us.myshoplazza.com"}}}
	cases := []struct {
		name     string
		cfg      core.CliConfig
		urlEnv   string
		wantBase string // expected prefix of f.Client.ResolveURL("/x"); "" = expect an error
	}{
		{"GATE-05 token+profile", withProfile, "", "https://us.myshoplazza.com"},
		{"GATE-06 token+urlEnv no config", core.CliConfig{ConfigVersion: 2}, "https://ci.myshoplazza.com", "https://ci.myshoplazza.com"},
		{"GATE-08 urlEnv beats profile", withProfile, "https://ci.myshoplazza.com", "https://ci.myshoplazza.com"},
		{"GATE-07 token only", core.CliConfig{ConfigVersion: 2}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok-ci")
			t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", tc.urlEnv)
			t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
			f := &Factory{Config: tc.cfg, Client: client.New("")}
			err := RequireAuth(context.Background(), f, newCmdWithProfileFlag())
			if tc.wantBase == "" {
				if err == nil {
					t.Fatal("must error when no store target is available")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := f.Client.ResolveURL("/x"); !strings.HasPrefix(got, tc.wantBase) {
				t.Fatalf("base URL = %q, want prefix %q", got, tc.wantBase)
			}
		})
	}
}
