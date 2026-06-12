package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
)

// setupTempConfig redirects auth/config/keychain paths to a temp dir.
func setupTempConfig(t *testing.T) (configPath, authPath string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("HOME", dir)
	configPath = filepath.Join(dir, "config.json")
	authPath = filepath.Join(dir, "auth.json")
	return configPath, authPath
}

func newTestManager(t *testing.T, srv *httptest.Server) *internalauth.Manager {
	t.Helper()
	configPath, authPath := setupTempConfig(t)
	mgr := internalauth.NewManager(core.CliConfig{}, configPath, client.New(srv.URL))
	mgr.AuthPath = authPath
	return mgr
}

type brokerOpts struct {
	pendingPolls int
	okBody       map[string]any
	storeAT      map[string]any
}

// brokerServer mocks the auth backend for the interactive web flow: pending → ok,
// with the token bundle returned inline on the OK poll.
func brokerServer(t *testing.T, opts brokerOpts) *httptest.Server {
	t.Helper()
	polls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/saiga/cli/auth/sessions":
			json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1", "authorize_url": "https://example.com/oauth?s=sess1"})
		case strings.HasSuffix(r.URL.Path, "/token"):
			polls++
			if polls < opts.pendingPolls+1 {
				json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
				return
			}
			json.NewEncoder(w).Encode(opts.okBody)
		case r.URL.Path == "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "account": "alice@example.com"})
		case r.URL.Path == "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(opts.storeAT)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
}

func TestLogin_PureAccount_WritesUATandPartner(t *testing.T) {
	srv := brokerServer(t, brokerOpts{
		okBody: map[string]any{
			"status": "ok", "uat": "uat_acct", "account": "alice@example.com",
			"uat_expires_at": "2026-12-01T00:00:00Z",
			"partner_token":  map[string]any{"access_token": "pt_1", "partner_id": "777", "at_expires_at": "2027-01-01T00:00:00Z"},
		},
	})
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "", nil, "", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Flow != "web" || !res.Status.LoggedIn {
		t.Errorf("res = %+v", res)
	}
	if res.Status.CurrentStore != "" {
		t.Errorf("pure-account login must not set current store, got %q", res.Status.CurrentStore)
	}
	st, _ := mgr.LoadState()
	if st.UAT != "uat_acct" || st.Partner != "pt_1" {
		t.Errorf("state UAT=%q Partner=%q", st.UAT, st.Partner)
	}
	if len(st.Stores) != 0 {
		t.Errorf("pure-account login must not write a store token, got %v", st.Stores)
	}
}

func TestLogin_WithStoreDomain_PrewarmsStoreToken(t *testing.T) {
	srv := brokerServer(t, brokerOpts{
		okBody: map[string]any{
			"status": "ok", "uat": "uat_s", "account": "a@x.com",
			"store_token": map[string]any{"access_token": "at_store", "store_id": "42", "store_domain": "my-store.com", "granted_scopes": []string{"read_product"}, "at_expires_at": "2026-12-01T00:00:00Z"},
		},
	})
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "my-store.com", nil, "", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Status.CurrentStore != "my-store.com" {
		t.Errorf("current store = %q", res.Status.CurrentStore)
	}
	st, _ := mgr.LoadState()
	if st.Stores["my-store.com"].Token != "at_store" {
		t.Errorf("store token = %q", st.Stores["my-store.com"].Token)
	}
	if len(st.GrantedScopes) != 1 || st.GrantedScopes[0] != "read_product" {
		t.Errorf("granted scopes mirror = %v", st.GrantedScopes)
	}
}

func TestLogin_WithStoreDomain_PrewarmMissing_StillSetsCurrentStore(t *testing.T) {
	srv := brokerServer(t, brokerOpts{
		okBody: map[string]any{"status": "ok", "uat": "uat_s", "account": "a@x.com"}, // no store_token
	})
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "my-store.com", nil, "", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Status.CurrentStore != "my-store.com" {
		t.Errorf("current store should be set even when prewarm omits store_token; got %q", res.Status.CurrentStore)
	}
	st, _ := mgr.LoadState()
	if _, ok := st.Stores["my-store.com"]; ok {
		t.Errorf("no store token expected when prewarm omitted it")
	}
}

func TestLogin_UATInjection_CallsMe_NoPartner(t *testing.T) {
	meCalled, exchangeCalled := false, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			meCalled = true
			json.NewEncoder(w).Encode(map[string]any{"user_id": "u9", "account": "carol@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			exchangeCalled = true
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_s", "store_domain": "s.com"})
		case "/api/saiga/cli/auth/sessions", "/api/saiga/cli/auth/sessions/sess1/token":
			t.Errorf("UAT injection must not create or poll a session: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "", nil, "uat_injected", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Flow != "uat" || !meCalled || exchangeCalled {
		t.Errorf("flow=%q meCalled=%v exchangeCalled=%v (no store-domain → no exchange)", res.Flow, meCalled, exchangeCalled)
	}
	st, _ := mgr.LoadState()
	if st.UAT != "uat_injected" || st.Partner != "" {
		t.Errorf("UAT injection must store UAT and no partner; got UAT=%q Partner=%q", st.UAT, st.Partner)
	}
}

func TestLogin_PollDenied_HTTP403_MapsToAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/saiga/cli/auth/sessions":
			json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1", "authorize_url": "https://example.com/x"})
		case strings.HasSuffix(r.URL.Path, "/token"):
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"code":"user_denied","errors":["denied"]}`))
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	_, err := mgr.Login(context.Background(), "", nil, "", 5*time.Second, time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected denied auth error, got %v", err)
	}
}

func TestLogin_PollTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/saiga/cli/auth/sessions" {
			json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1", "authorize_url": "x"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	_, err := mgr.Login(context.Background(), "", nil, "", 40*time.Millisecond, 10*time.Millisecond, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestUseStore_ExchangesAndSetsCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_use", "store_domain": "b.com", "granted_scopes": []string{"read_order"}, "at_expires_at": "2026-12-01T00:00:00Z"})
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	if _, err := mgr.Login(context.Background(), "", nil, "uat_z", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("Login: %v", err)
	}
	st, err := mgr.UseStore(context.Background(), "b.com")
	if err != nil {
		t.Fatalf("UseStore: %v", err)
	}
	if st.CurrentStore != "b.com" {
		t.Errorf("current store = %q", st.CurrentStore)
	}
	loaded, _ := mgr.LoadState()
	if loaded.Stores["b.com"].Token != "at_use" || loaded.CurrentStore != "b.com" {
		t.Errorf("persisted store/current wrong: %+v", loaded)
	}
}

func TestUseStore_NoUAT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	mgr := newTestManager(t, srv)
	if _, err := mgr.UseStore(context.Background(), "b.com"); err == nil {
		t.Error("expected error when no UAT present")
	}
}

func TestAccessTokenReady_RefreshesWhenMissing(t *testing.T) {
	exchanges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			exchanges++
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_fresh", "store_domain": "c.com", "at_expires_at": "2099-01-01T00:00:00Z"})
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	if _, err := mgr.Login(context.Background(), "", nil, "uat_q", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("Login: %v", err)
	}
	tok, err := mgr.AccessTokenReady(context.Background(), "c.com")
	if err != nil || tok != "at_fresh" {
		t.Fatalf("AccessTokenReady = %q, %v", tok, err)
	}
	if exchanges != 1 {
		t.Errorf("expected exactly one exchange, got %d", exchanges)
	}
	if _, err := mgr.AccessTokenReady(context.Background(), "c.com"); err != nil {
		t.Fatal(err)
	}
	if exchanges != 1 {
		t.Errorf("cached token should not re-exchange, got %d", exchanges)
	}
}

func TestAccessTokenReady_NoStoreDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	mgr := newTestManager(t, srv)
	if _, err := mgr.AccessTokenReady(context.Background(), ""); err == nil {
		t.Error("expected error with empty store domain")
	}
}

func TestLogout_ClearsAllTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_lo", "store_domain": "d.com"})
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	if _, err := mgr.Login(context.Background(), "d.com", nil, "uat_lo", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := mgr.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	st, _ := mgr.LoadState()
	if st.UAT != "" || st.Partner != "" || len(st.Stores) != 0 || st.CurrentStore != "" {
		t.Errorf("logout did not clear everything: %+v", st)
	}
	status, _ := mgr.CurrentStatus()
	if status.LoggedIn {
		t.Error("should not be logged in after logout")
	}
}

func TestPersistState_NoTokensInAuthJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "super_secret_store_at", "store_domain": "e.com"})
		}
	}))
	defer srv.Close()
	configPath, authPath := setupTempConfig(t)
	mgr := internalauth.NewManager(core.CliConfig{}, configPath, client.New(srv.URL))
	mgr.AuthPath = authPath

	if _, err := mgr.Login(context.Background(), "e.com", nil, "uat_secret_value", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("Login: %v", err)
	}
	data, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	body := string(data)
	for _, secret := range []string{"super_secret_store_at", "uat_secret_value"} {
		if strings.Contains(body, secret) {
			t.Errorf("auth.json must not contain token %q", secret)
		}
	}
}

func TestCurrentStatus_NotLoggedIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	mgr := newTestManager(t, srv)
	status, err := mgr.CurrentStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.LoggedIn {
		t.Error("fresh install must not be logged in")
	}
}

// Regression: when the prewarmed store_token echoes a domain different from
// the one the caller requested, the store token must still be keyed by the
// current store so AccessTokenReady hits the cache instead of re-exchanging on
// every command.
func TestLogin_StoreDomainMismatch_CacheHitsByCurrentStore(t *testing.T) {
	exchanges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/saiga/cli/auth/sessions":
			json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1", "authorize_url": "x"})
		case strings.HasSuffix(r.URL.Path, "/token"):
			json.NewEncoder(w).Encode(map[string]any{
				"status": "ok", "uat": "uat_m", "account": "a@x.com",
				"store_token": map[string]any{
					"access_token": "at_prewarm", "store_domain": "normalized.myshoplazza.com",
					"at_expires_at": "2099-01-01T00:00:00Z",
				},
			})
		case r.URL.Path == "/api/saiga/cli/auth/exchange/store-at":
			exchanges++
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_reexchanged", "store_domain": "normalized.myshoplazza.com", "at_expires_at": "2099-01-01T00:00:00Z"})
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "requested.com", nil, "", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// The store must be keyed by the current store, whatever it is.
	tok, err := mgr.AccessTokenReady(context.Background(), res.Status.CurrentStore)
	if err != nil {
		t.Fatalf("AccessTokenReady: %v", err)
	}
	if tok != "at_prewarm" {
		t.Errorf("expected cached prewarm token, got %q", tok)
	}
	if exchanges != 0 {
		t.Errorf("prewarmed token under current store should be reused; got %d re-exchanges", exchanges)
	}
}

func TestE2E_Login_StoreUse_Refresh_Logout(t *testing.T) {
	exchanges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "alice@example.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			exchanges++
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at_store", "store_domain": "shop.com",
				"at_expires_at": "2099-01-01T00:00:00Z", "granted_scopes": []string{"read_product"},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	configPath, authPath := setupTempConfig(t)
	mgr := internalauth.NewManager(core.CliConfig{}, configPath, client.New(srv.URL))
	mgr.AuthPath = authPath

	// 1. Pure-account login via injected UAT (calls /me only).
	if _, err := mgr.Login(context.Background(), "", nil, "uat_e2e", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("login: %v", err)
	}
	// 2. store use → current store + store token (one exchange).
	if _, err := mgr.UseStore(context.Background(), "shop.com"); err != nil {
		t.Fatalf("use: %v", err)
	}
	if exchanges != 1 {
		t.Fatalf("expected 1 exchange after UseStore, got %d", exchanges)
	}
	// 3. business call resolves token transparently from cache (no new exchange).
	tok, err := mgr.AccessTokenReady(context.Background(), "shop.com")
	if err != nil || tok != "at_store" {
		t.Fatalf("AccessTokenReady = %q, %v", tok, err)
	}
	if exchanges != 1 {
		t.Errorf("cached token should not re-exchange, got %d exchanges", exchanges)
	}
	// 4. logout clears everything.
	if _, err := mgr.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	st, _ := mgr.CurrentStatus()
	if st.LoggedIn {
		t.Error("should be logged out")
	}
}

// A non-interactive `auth login --uat` cannot obtain a partner token (those are
// only minted at interactive consent). It must not leave a stale partner
// token from a prior interactive login behind — LoadState reads the partner
// keychain entry unconditionally, so a lingering entry would be resurrected.
func TestLoginUAT_ClearsStalePartnerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/saiga/cli/auth/sessions":
			json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1", "authorize_url": "x"})
		case strings.HasSuffix(r.URL.Path, "/token"):
			json.NewEncoder(w).Encode(map[string]any{
				"status": "ok", "uat": "uat_interactive", "account": "a@x.com",
				"partner_token": map[string]any{"access_token": "pt_stale", "partner_id": "1", "at_expires_at": "2099-01-01T00:00:00Z"},
			})
		case r.URL.Path == "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	mgr := newTestManager(t, srv)

	// 1. Interactive login mints a partner token.
	if _, err := mgr.Login(context.Background(), "", nil, "", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("interactive login: %v", err)
	}
	if st, _ := mgr.LoadState(); st.Partner != "pt_stale" {
		t.Fatalf("setup: expected partner token after interactive login, got %q", st.Partner)
	}

	// 2. Non-interactive --uat re-login produces no partner token.
	if _, err := mgr.Login(context.Background(), "", nil, "uat_injected", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("uat login: %v", err)
	}

	// 3. The stale partner token must not be resurrected by LoadState...
	st, err := mgr.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Partner != "" {
		t.Errorf("stale partner token resurrected after --uat re-login: %q", st.Partner)
	}
	// ...and must be gone from the keychain entirely.
	if got, _ := keychain.Get(keychain.ShoplazzaCliService, "partner"); got != "" {
		t.Errorf("partner keychain entry should be removed when login yields no partner, got %q", got)
	}
}
