package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/testenv"
)

// seedAccountUAT stores a UAT under the v2 account-namespaced keychain key.
func seedAccountUAT(t *testing.T, email, uat string) {
	t.Helper()
	if err := keychain.Set(keychain.ShoplazzaCliService, AccountUATKey(email), uat); err != nil {
		t.Fatalf("seedAccountUAT: %v", err)
	}
}

// seedSingleAccountConfig sets cfg.Accounts to a single entry so Config.Account() resolves.
func seedSingleAccountConfig(cfg *core.CliConfig, email string) {
	cfg.Accounts = []core.AccountConfig{{Name: email}}
}

// newExchangeStub returns an httptest server that stubs the store-AT exchange
// endpoint, always returning accessToken with a fixed store/scope/expiry.
func newExchangeStub(t *testing.T, accessToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"access_token": accessToken, "store_id": "1",
			"store_domain": "cn.myshoplazza.com", "granted_scopes": []string{"read_product"},
			"at_expires_at": "2099-01-01T00:00:00Z",
		}})
	}))
}

func TestExchangeForProfile_SendsScopesAndPersists(t *testing.T) {
	testenv.IsolateConfigDir(t)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"access_token": "at-1", "store_id": "100001",
			"store_domain": "us.myshoplazza.com", "granted_scopes": []string{"read_product"},
			"at_expires_at": "2099-01-01T00:00:00Z",
		}})
	}))
	defer srv.Close()

	authDir := t.TempDir()
	m := &Manager{Client: client.New(srv.URL)}
	seedAccountUAT(t, "alice@co.com", "uat-1")

	p := core.ProfileConfig{Name: "us", Account: "alice@co.com",
		StoreDomain: "us.myshoplazza.com", Scopes: []string{"read_product"}}
	tok, err := m.ExchangeForProfile(context.Background(), authDir, p)
	if err != nil || tok != "at-1" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if sc, _ := gotBody["scopes"].([]any); len(sc) != 1 || sc[0] != "read_product" {
		t.Fatalf("scopes not sent: %v", gotBody)
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, ProfileStoreKey("us")); err != nil || v != "at-1" {
		t.Fatalf("token not persisted: v=%q err=%v", v, err)
	}
	if meta, err := LoadProfileMeta(authDir, "us"); err != nil || meta.StoreID != "100001" {
		t.Fatalf("meta not persisted: %+v err=%v", meta, err)
	}
}

func TestExchangeEphemeral_NoPersistence(t *testing.T) {
	testenv.IsolateConfigDir(t)

	srv := newExchangeStub(t, "at-tmp")
	defer srv.Close()

	m := &Manager{Client: client.New(srv.URL)}
	seedAccountUAT(t, "alice@co.com", "uat-1")
	seedSingleAccountConfig(&m.Config, "alice@co.com")

	tok, err := m.ExchangeEphemeral(context.Background(), "cn.myshoplazza.com")
	if err != nil || tok != "at-tmp" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, ProfileStoreKey("cn")); err != nil || v != "" {
		t.Fatalf("ephemeral must not persist: v=%q err=%v", v, err)
	}
}

func TestExchangeStoreATScoped_OmitsScopesKeyWhenNil(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-plain", "store_id": "42", "store_domain": "shop.com",
			"granted_scopes": []string{"read_product"}, "at_expires_at": "2030-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	m := &Manager{Client: client.New(srv.URL)}
	block, err := m.exchangeStoreAT(context.Background(), "uat_x", "shop.com")
	if err != nil {
		t.Fatalf("exchangeStoreAT: %v", err)
	}
	if block.AccessToken != "at-plain" {
		t.Fatalf("access_token = %q", block.AccessToken)
	}
	if _, ok := gotBody["scopes"]; ok {
		t.Fatalf("scopes key must be omitted when nil: %v", gotBody)
	}
}
