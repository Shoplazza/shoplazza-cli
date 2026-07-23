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

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

// setupTempConfig redirects auth/config/keychain paths to a temp dir.
func setupTempConfig(t *testing.T) (configPath, authPath string) {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
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
	pendingPolls  int
	okBody        map[string]any
	storeAT       map[string]any
	storeATStatus int // when non-zero and != 200, store-at responds with this status
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
			if opts.storeATStatus != 0 && opts.storeATStatus != http.StatusOK {
				w.WriteHeader(opts.storeATStatus)
				w.Write([]byte(`{"code":"store_not_found","errors":["store not found"]}`))
				return
			}
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
	if res.StoreToken == nil || res.StoreToken.AccessToken != "at_store" {
		t.Errorf("prewarm token must ride out on LoginResult, got %+v", res.StoreToken)
	}
	st, _ := mgr.LoadState()
	if len(st.Stores) != 0 {
		t.Errorf("login must not write the legacy store slot, got %v", st.Stores)
	}
	if len(st.GrantedScopes) != 1 || st.GrantedScopes[0] != "read_product" {
		t.Errorf("granted scopes mirror = %v", st.GrantedScopes)
	}
}

// When the session omits a prewarmed store_token, login validates the store via
// an explicit store-at exchange; a valid store gets set + its token cached.
func TestLogin_WithStoreDomain_PrewarmMissing_ValidatesViaExchange(t *testing.T) {
	srv := brokerServer(t, brokerOpts{
		okBody:  map[string]any{"status": "ok", "uat": "uat_s", "account": "a@x.com"}, // no store_token
		storeAT: map[string]any{"access_token": "at_validated", "store_id": "7", "store_domain": "my-store.com", "granted_scopes": []string{"read_product"}, "at_expires_at": "2099-01-01T00:00:00Z"},
	})
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "my-store.com", nil, "", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Status.CurrentStore != "my-store.com" || res.StoreWarning != "" {
		t.Errorf("valid store should be set with no warning; current=%q warn=%q", res.Status.CurrentStore, res.StoreWarning)
	}
	if res.StoreToken == nil || res.StoreToken.AccessToken != "at_validated" {
		t.Errorf("validation token must ride out on LoginResult, got %+v", res.StoreToken)
	}
	st, _ := mgr.LoadState()
	if len(st.Stores) != 0 {
		t.Errorf("login must not write the legacy store slot, got %v", st.Stores)
	}
}

// A bad/inaccessible --store-domain (store-at 404) does not fail login: it warns
// and leaves the store unset, and the bad domain is never persisted.
func TestLogin_WithStoreDomain_ValidationFails_WarnsAndUnsets(t *testing.T) {
	srv := brokerServer(t, brokerOpts{
		okBody:        map[string]any{"status": "ok", "uat": "uat_s", "account": "a@x.com"}, // no store_token
		storeATStatus: http.StatusNotFound,
	})
	defer srv.Close()
	mgr := newTestManager(t, srv)

	res, err := mgr.Login(context.Background(), "bad-store.com", nil, "", 5*time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("login should still succeed on a bad store: %v", err)
	}
	if res.Status.CurrentStore != "" {
		t.Errorf("bad store must not be set as current, got %q", res.Status.CurrentStore)
	}
	if !strings.Contains(res.StoreWarning, "bad-store.com") {
		t.Errorf("expected a store warning naming the domain, got %q", res.StoreWarning)
	}
	st, _ := mgr.LoadState()
	if st.CurrentStore != "" || len(st.Stores) != 0 {
		t.Errorf("bad store must not be persisted: current=%q stores=%v", st.CurrentStore, st.Stores)
	}
	if st.UAT != "uat_s" {
		t.Errorf("login (UAT) should still persist, got %q", st.UAT)
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

func TestRefreshAccessToken_MintsAndPersists(t *testing.T) {
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
	tok, err := mgr.RefreshAccessToken(context.Background(), "c.com")
	if err != nil || tok != "at_fresh" {
		t.Fatalf("RefreshAccessToken = %q, %v", tok, err)
	}
	if exchanges != 1 {
		t.Errorf("expected exactly one exchange, got %d", exchanges)
	}
	// The mint persists into the legacy store slot (read back via LoadState).
	st, err := mgr.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Stores["c.com"].Token != "at_fresh" {
		t.Errorf("persisted store token = %q, want at_fresh", st.Stores["c.com"].Token)
	}
	if exchanges != 1 {
		t.Errorf("LoadState should not re-exchange, got %d", exchanges)
	}
}

func TestRefreshAccessToken_NoStoreDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	mgr := newTestManager(t, srv)
	if _, err := mgr.RefreshAccessToken(context.Background(), ""); err == nil {
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

// granted_scopes must always serialize as [] when empty, never omitted.
func TestCurrentStatus_GrantedScopesAlwaysPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	mgr := newTestManager(t, srv)
	status, err := mgr.CurrentStatus()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"granted_scopes":[]`) {
		t.Errorf("granted_scopes must serialize as [] when empty (not null/omitted); got %s", b)
	}
	if !strings.Contains(string(b), `"current_store":""`) {
		t.Errorf("current_store must serialize as \"\" when empty (not omitted); got %s", b)
	}
}

// Regression: when the prewarmed store_token echoes a domain different from
// the one the caller requested, the current store stays the REQUESTED domain
// and the prewarm block still rides out on LoginResult for the profile layer.
func TestLogin_StoreDomainMismatch_PrewarmRidesOnResult(t *testing.T) {
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
	if res.Status.CurrentStore != "requested.com" {
		t.Errorf("current store must stay the requested domain, got %q", res.Status.CurrentStore)
	}
	if res.StoreToken == nil || res.StoreToken.AccessToken != "at_prewarm" {
		t.Errorf("prewarm token must ride out on LoginResult, got %+v", res.StoreToken)
	}
	if exchanges != 0 {
		t.Errorf("prewarm must not trigger a validation exchange; got %d", exchanges)
	}
}

func TestE2E_Login_Refresh_Logout(t *testing.T) {
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
	// 2. first token resolve mints once (store use now lives in the profile
	// model; the legacy slot is exercised via RefreshAccessToken directly).
	tok, err := mgr.RefreshAccessToken(context.Background(), "shop.com")
	if err != nil || tok != "at_store" {
		t.Fatalf("RefreshAccessToken = %q, %v", tok, err)
	}
	if exchanges != 1 {
		t.Fatalf("expected 1 exchange after first resolve, got %d", exchanges)
	}
	// 3. the mint persisted into the legacy slot (no new exchange to read it).
	state, err := mgr.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Stores["shop.com"].Token != "at_store" {
		t.Fatalf("persisted store token = %q, want at_store", state.Stores["shop.com"].Token)
	}
	if exchanges != 1 {
		t.Errorf("LoadState should not re-exchange, got %d exchanges", exchanges)
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
// only minted at interactive consent). For the SAME account it must PRESERVE an
// existing partner token from a prior interactive login rather than wiping it —
// otherwise a routine re-login would force a fresh interactive consent for every
// app command. (An account switch still clears it; see persistState.)
func TestLoginUAT_PreservesPartnerTokenSameAccount(t *testing.T) {
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

	// 2. Non-interactive --uat re-login of the SAME account produces no partner token.
	if _, err := mgr.Login(context.Background(), "", nil, "uat_injected", 5*time.Second, time.Millisecond, nil); err != nil {
		t.Fatalf("uat login: %v", err)
	}

	// 3. The same account's partner token is preserved (not wiped), so app
	// commands keep working without a fresh interactive login.
	st, err := mgr.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Partner != "pt_stale" {
		t.Errorf("partner token should be preserved across a same-account --uat re-login, got %q", st.Partner)
	}
	if got, _ := keychain.Get(keychain.ShoplazzaCliService, internalauth.AccountPartnerKey("a@x.com")); got != "pt_stale" {
		t.Errorf("partner keychain entry should be preserved for the same account, got %q", got)
	}
}
