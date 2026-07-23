package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	internalauth "github.com/Shoplazza/shoplazza-cli/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/internal/core"
)

// TestLoginThenGate_StoreTokenReady is the "real login output → Gate input"
// seam that unit fixtures (which seed v2 keys directly) skipped. It reproduces
// BUG-01: `auth login` persisted the UAT under a v1 keychain key while the Gate
// reads the v2 account-namespaced key, so the first store command after login
// failed with "no UAT available". A real login must leave the Gate able to mint
// a store token for the profile it created.
func TestLoginThenGate_StoreTokenReady(t *testing.T) {
	store := "gate-store.myshoplaza.com"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"account": "alice@example.com", "user_id": "u-1"})
		case "/api/saiga/cli/auth/exchange/store-at":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
				"access_token": "at-gate", "store_id": "100001", "store_domain": store,
				"granted_scopes": []string{"read_product"}, "at_expires_at": "2099-01-01T00:00:00Z",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	if err := execAuth(t, f, out, "login", "--uat", "uat-abc", "--store-domain", store); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	// Reload the config login wrote and resolve the profile it created.
	cfg, err := core.LoadConfig(f.ConfigPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := cfg.Current()
	if p == nil {
		t.Fatalf("login did not create/select a profile: %+v", cfg)
	}

	// The Gate path a store command runs: mint/get the profile's store token.
	// This calls AccountUAT(profile.Account) under the hood — the exact read the
	// login write must line up with.
	mgr := internalauth.NewManager(cfg, f.ConfigPath, f.AuthClient)
	tok, err := mgr.AccessTokenReadyForProfile(context.Background(), f.ConfigPath, *p)
	if err != nil {
		t.Fatalf("gate could not ready a store token after login (BUG-01 namespace split): %v", err)
	}
	if tok == "" {
		t.Fatal("gate returned an empty store token")
	}
}
