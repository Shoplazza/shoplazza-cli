package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	internalauth "github.com/Shoplazza/shoplazza-cli/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/core"
	"github.com/Shoplazza/shoplazza-cli/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/internal/testenv"
)

func TestEnsureAppToken_FetchesSecretThenMints(t *testing.T) {
	// Isolate config + keychain to temp (HOME/XDG drive os.UserConfigDir).
	dir := testenv.IsolateConfigDir(t)
	// Seed a UAT so AppTokenReady's logged-in gate passes.
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@co.com"), "uat_1"); err != nil {
		t.Fatal(err)
	}

	// Fake Dashboard: app nested under "app"; secret field is "secret".
	dash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli/v2/partners/p1/apps/cid_1" {
			t.Fatalf("dashboard path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_1", "id": 9799759, "secret": "sec_xyz"}},
		})
	}))
	defer dash.Close()

	// Fake saiga: asserts the secret was forwarded, returns an app token.
	var gotSecret string
	saiga := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/saiga/cli/auth/exchange/app-at" {
			t.Fatalf("saiga path = %s", r.URL.Path)
		}
		var body struct {
			ClientSecret string `json:"client_secret"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotSecret = body.ClientSecret
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "app_at_1", "client_id": "cid_1", "at_expires_at": "2030-01-01T00:00:00Z",
		})
	}))
	defer saiga.Close()

	cfg := core.CliConfig{Accounts: []core.AccountConfig{{Name: "alice@co.com"}}}
	mgr := internalauth.NewManager(cfg, filepath.Join(dir, "config.json"), client.New(saiga.URL))
	d := NewDashboard(client.New(dash.URL), "partner_tok")

	tok, err := EnsureAppToken(context.Background(), d, mgr, "p1", "cid_1")
	if err != nil {
		t.Fatalf("EnsureAppToken: %v", err)
	}
	if tok != "app_at_1" {
		t.Fatalf("token = %q", tok)
	}
	if gotSecret != "sec_xyz" {
		t.Fatalf("saiga did not receive the dashboard secret: %q", gotSecret)
	}
}
