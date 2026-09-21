package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
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

// writeAuthMeta seeds the account auth metadata inside the isolated config
// dir. Call it after tempFactory, which does the isolating.
func writeAuthMeta(t *testing.T, body string) {
	t.Helper()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "shoplazza-cli", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// loggedInFactory is GATE-04's wiring: a resolvable profile whose store-token
// exchange succeeds, with a seeded UAT and auth metadata.
func loggedInFactory(t *testing.T, meta string) *Factory {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
				"access_token": "at_bearer", "store_id": "1", "store_domain": "shop.com",
				"granted_scopes": []string{"read_product"}, "at_expires_at": "2099-01-01T00:00:00Z",
			}})
		}
	}))
	t.Cleanup(srv.Close)

	cfg := core.CliConfig{ConfigVersion: 2, CurrentProfile: "us",
		Profiles: []core.ProfileConfig{{Name: "us", Account: "a@co.com", StoreDomain: "shop.com"}}}
	f := tempFactory(t, srv.URL, cfg)
	writeAuthMeta(t, meta)
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("a@co.com"), "uat_seed"); err != nil {
		t.Fatal(err)
	}
	return f
}

// GATE-09: the audit header carries the login user id captured at login.
func TestRequireAuth_InjectsCliUserID(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	t.Setenv(EnvCliUserID, "")
	f := loggedInFactory(t, `{"account":"a@co.com","user_id":"u_42"}`)
	if err := RequireAuth(context.Background(), f, newCmdWithProfileFlag()); err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}
	if got := f.Client.Headers["cli-user-id"]; got != "u_42" {
		t.Errorf("cli-user-id = %q, want u_42", got)
	}
}

// GATE-10: the env override beats the persisted id.
func TestRequireAuth_CliUserIDEnvWins(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	t.Setenv(EnvCliUserID, "u_env")
	f := loggedInFactory(t, `{"account":"a@co.com","user_id":"u_42"}`)
	if err := RequireAuth(context.Background(), f, newCmdWithProfileFlag()); err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}
	if got := f.Client.Headers["cli-user-id"]; got != "u_env" {
		t.Errorf("cli-user-id = %q, want u_env", got)
	}
}

// GATE-11: an id-less session omits the header rather than failing the command.
func TestRequireAuth_CliUserIDAbsentIsNotFatal(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	t.Setenv(EnvCliUserID, "")
	f := loggedInFactory(t, `{"account":"a@co.com"}`)
	if err := RequireAuth(context.Background(), f, newCmdWithProfileFlag()); err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}
	if got, ok := f.Client.Headers["cli-user-id"]; ok {
		t.Errorf("cli-user-id must be absent, got %q", got)
	}
}

// GATE-12: under the injected-token bypass the audit id comes from the env
// only — a local login must not attribute a CI call to that user.
func TestRequireAuth_BypassCliUserIDFromEnvOnly(t *testing.T) {
	for _, tc := range []struct{ name, env, want string }{
		{"env unset -> omitted", "", ""},
		{"env set -> injected", "u_ci", "u_ci"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := loggedInFactory(t, `{"account":"a@co.com","user_id":"u_42"}`)
			t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok-ci")
			t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", "https://ci.myshoplazza.com")
			t.Setenv(EnvCliUserID, tc.env)
			if err := RequireAuth(context.Background(), f, newCmdWithProfileFlag()); err != nil {
				t.Fatalf("RequireAuth: %v", err)
			}
			if got := f.Client.Headers["cli-user-id"]; got != tc.want {
				t.Errorf("cli-user-id = %q, want %q", got, tc.want)
			}
		})
	}
}
