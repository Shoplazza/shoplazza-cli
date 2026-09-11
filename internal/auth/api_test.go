package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

func TestExchangeAppAT_SendsFourFields(t *testing.T) {
	var got exchangeAppATRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/saiga/cli/auth/exchange/app-at" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		// partner_id is a uint64 carried as a string; at_expires_at is a
		// Timestamp serialized by protojson as RFC3339.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "app_at_xyz",
			"partner_id":    "42",
			"client_id":     "cid_1",
			"at_expires_at": "2030-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	m := &Manager{Client: client.New(srv.URL)}
	// partner_id is sent as a string ("42") — protojson's canonical uint64 form.
	block, err := m.exchangeAppAT(context.Background(), "uat_1", "cid_1", "secret_1", "42")
	if err != nil {
		t.Fatalf("exchangeAppAT: %v", err)
	}
	if got.UAT != "uat_1" || got.ClientID != "cid_1" || got.ClientSecret != "secret_1" || got.PartnerID != "42" {
		t.Fatalf("request body = %+v", got)
	}
	if block.AccessToken != "app_at_xyz" || block.ClientID != "cid_1" ||
		block.PartnerID != "42" || block.ATExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("block = %+v", block)
	}
}

func mgrTo(srvURL string) *Manager {
	return &Manager{Client: client.New(srvURL)}
}

func TestMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/saiga/cli/auth/me" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body meRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.UAT != "uat_x" {
			t.Errorf("uat = %q", body.UAT)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "account": "alice@example.com"})
	}))
	defer srv.Close()

	res, err := mgrTo(srv.URL).me(context.Background(), "uat_x")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if res.Account != "alice@example.com" || res.UserID != "u1" {
		t.Errorf("me = %+v", res)
	}
}

func TestExchangeStoreAT_DecodesQuotedUint64StoreID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/saiga/cli/auth/exchange/store-at" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body exchangeStoreATRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.StoreDomain != "shop.com" || body.UAT != "uat_x" {
			t.Errorf("req = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		// protojson serializes uint64 store_id as a quoted string.
		w.Write([]byte(`{"access_token":"at_s","store_id":"9988776655443322110","store_domain":"shop.com","granted_scopes":["read_product"],"at_expires_at":"2026-06-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	block, err := mgrTo(srv.URL).exchangeStoreAT(context.Background(), "uat_x", "shop.com")
	if err != nil {
		t.Fatalf("exchangeStoreAT: %v", err)
	}
	if block.AccessToken != "at_s" {
		t.Errorf("access_token = %q", block.AccessToken)
	}
	if block.StoreID != "9988776655443322110" {
		t.Errorf("store_id = %q (quoted-uint64 must decode into a string field intact)", block.StoreID)
	}
	if len(block.GrantedScopes) != 1 || block.GrantedScopes[0] != "read_product" {
		t.Errorf("granted_scopes = %v", block.GrantedScopes)
	}
}

func TestParseSaigaAuthError(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{403, `{"code":"user_denied","errors":["denied"]}`, "denied"},
		{400, `{"code":"session_expired"}`, "expired"},
		{500, `{"code":"boom"}`, "authentication failed"},
		{500, `not json`, "authentication failed"},
	}
	for _, c := range cases {
		err := parseSaigaAuthError(&client.HTTPError{StatusCode: c.status, Body: c.body})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("status %d body %q → %v; want substring %q", c.status, c.body, err, c.want)
		}
	}
}

// TestStoreAppKcKey guards the keychain key contract from the package-internal
// side: resource-scoped store/app builders keep their "<kind>:<id>" prefix, and
// the v2 account builders keep the namespaced format the profile Gate reads —
// drift there silently re-opens the login/Gate split (BUG-01).
func TestStoreAppKcKey(t *testing.T) {
	if got := storeKcKey("my-store.com"); got != "store:my-store.com" {
		t.Errorf("storeKcKey = %q", got)
	}
	if got := appKcKey("cid_123"); got != "app:cid_123" {
		t.Errorf("appKcKey = %q", got)
	}
	if got := AccountUATKey("Alice@Co.com"); got != "account:alice@co.com:uat" {
		t.Errorf("AccountUATKey = %q", got)
	}
	if got := AccountPartnerKey("Alice@Co.com"); got != "account:alice@co.com:partner" {
		t.Errorf("AccountPartnerKey = %q", got)
	}
}
