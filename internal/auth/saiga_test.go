package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
)

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
		{504, `{"code":"session_expired"}`, "expired"},
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
