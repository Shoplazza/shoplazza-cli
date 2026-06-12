package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/output"
)

func tempFactory(t *testing.T, srvURL string, cfg core.CliConfig) *Factory {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("HOME", dir)
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

func TestRequireAuth_AccessTokenEnvBypass(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	f := tempFactory(t, "http://unused", core.CliConfig{})
	if err := RequireAuth(context.Background(), f); err != nil {
		t.Errorf("env bypass should return nil, got %v", err)
	}
}

func TestRequireAuth_NotLoggedIn(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	f := tempFactory(t, "http://unused", core.CliConfig{}) // fresh keychain → no UAT
	err := RequireAuth(context.Background(), f)
	if err == nil || exitType(t, err) != output.TypeAuth {
		t.Errorf("expected type=auth, got %v", err)
	}
}

func TestRequireAuth_LoggedInNoCurrentStore(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	f := tempFactory(t, "http://unused", core.CliConfig{}) // StoreDomain empty
	// Seed a UAT so LoggedIn == true.
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatal(err)
	}
	err := RequireAuth(context.Background(), f)
	if err == nil || exitType(t, err) != output.TypeValidation {
		t.Errorf("expected type=validation for missing current store, got %v", err)
	}
}

func TestRequireAuth_Success_SetsBearer(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_bearer", "store_domain": "shop.com", "at_expires_at": "2099-01-01T00:00:00Z"})
		}
	}))
	defer srv.Close()

	f := tempFactory(t, srv.URL, core.CliConfig{StoreDomain: "shop.com"})
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatal(err)
	}
	if err := RequireAuth(context.Background(), f); err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}
	if f.Client.Headers["Access-Token"] != "at_bearer" {
		t.Errorf("bearer not set: %v", f.Client.Headers)
	}
}
