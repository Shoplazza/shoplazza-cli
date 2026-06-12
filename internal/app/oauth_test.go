package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// testHMAC computes the HMAC the same way the middleware does: all params
// except hmac, sorted ascending, joined as key=value with &, then
// HMAC-SHA256(secret, message) hex-encoded.
func testHMAC(t *testing.T, q url.Values, secret string) string {
	t.Helper()
	keys := make([]string, 0, len(q))
	for k := range q {
		if k == "hmac" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+q.Get(k))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyHMAC_ValidAndTampered(t *testing.T) {
	const secret = "shh-secret"
	q := url.Values{}
	q.Set("shop", "demo.myshoplazza.com")
	q.Set("code", "the-code")
	q.Set("state", "STATE123")
	q.Set("timestamp", "1700000000")

	good := testHMAC(t, q, secret)
	q.Set("hmac", good)
	if !verifyHMAC(q, secret) {
		t.Fatalf("verifyHMAC should accept a correctly computed hmac")
	}

	// Flip a char in the hmac -> must fail.
	tampered := []byte(good)
	if tampered[0] == 'a' {
		tampered[0] = 'b'
	} else {
		tampered[0] = 'a'
	}
	q.Set("hmac", string(tampered))
	if verifyHMAC(q, secret) {
		t.Fatalf("verifyHMAC should reject a tampered hmac")
	}

	// Correct hmac but wrong secret -> must fail.
	q.Set("hmac", good)
	if verifyHMAC(q, "wrong-secret") {
		t.Fatalf("verifyHMAC should reject when the secret differs")
	}
}

func newTestHandler(cfg OAuthConfig) http.Handler {
	if cfg.ClientID == "" {
		cfg.ClientID = "client-abc"
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = "shh-secret"
	}
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = "https://app.example.com/auth/callback"
	}
	if cfg.Scopes == "" {
		cfg.Scopes = "read_products write_products"
	}
	if cfg.InstallPath == "" {
		cfg.InstallPath = "/auth"
	}
	if cfg.CallbackPath == "" {
		cfg.CallbackPath = "/auth/callback"
	}
	return NewOAuthHandler(cfg)
}

func TestInstall_RedirectsToAuthorize(t *testing.T) {
	cfg := OAuthConfig{
		ClientID:     "client-abc",
		ClientSecret: "shh-secret",
		RedirectURI:  "https://app.example.com/auth/callback",
		Scopes:       "read_products write_products",
		InstallPath:  "/auth",
		CallbackPath: "/auth/callback",
		NewState:     func() (string, error) { return "STATE123", nil },
	}
	h := newTestHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth?shop=demo.myshoplazza.com", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusFound)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location did not parse: %v", err)
	}
	if loc.Scheme != "https" || loc.Host != "demo.myshoplazza.com" || loc.Path != "/admin/oauth/authorize" {
		t.Fatalf("authorize endpoint = %q, want https://demo.myshoplazza.com/admin/oauth/authorize", loc.String())
	}
	q := loc.Query()
	for key, want := range map[string]string{
		"client_id":     "client-abc",
		"scope":         "read_products write_products",
		"redirect_uri":  "https://app.example.com/auth/callback",
		"response_type": "code",
		"state":         "STATE123",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("authorize param %s = %q, want %q", key, got, want)
		}
	}
	// The redirect_uri must actually be percent-escaped in the raw URL (the old
	// raw interpolation shipped it verbatim).
	if !strings.Contains(loc.RawQuery, "redirect_uri=https%3A%2F%2Fapp.example.com%2Fauth%2Fcallback") {
		t.Errorf("redirect_uri not escaped in %q", loc.RawQuery)
	}
}

// TestInstall_InvalidShop_400 locks the shop validation: empty or non-hostname
// values (separators, schemes, whitespace) must 400 instead of being
// interpolated into the redirect target.
func TestInstall_InvalidShop_400(t *testing.T) {
	h := newTestHandler(OAuthConfig{})
	for _, shop := range []string{
		"",                                    // missing
		"evil.com/phish",                      // path separator
		"evil.com:8443",                       // port/colon
		"evil.com?x=1",                        // query
		"evil.com#frag",                       // fragment
		"evil .com",                           // whitespace
		"attacker.com\\@demo.myshoplazza.com", // backslash trickery
	} {
		req := httptest.NewRequest(http.MethodGet, "/auth?shop="+url.QueryEscape(shop), nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("shop=%q: status = %d, want %d", shop, rr.Code, http.StatusBadRequest)
		}
	}
}

// install drives the install route so the handler issues (and stores) a state.
func install(t *testing.T, h http.Handler, shop string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth?shop="+url.QueryEscape(shop), nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("install: status = %d, want %d (body=%s)", rr.Code, http.StatusFound, rr.Body.String())
	}
}

func TestCallback_ValidHMAC_ExchangesAndRedirects(t *testing.T) {
	const secret = "shh-secret"
	var hits int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("token endpoint method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("token body not JSON: %v (%s)", err, body)
		}
		if got["client_id"] != "client-abc" {
			t.Errorf("client_id = %v, want client-abc", got["client_id"])
		}
		if got["client_secret"] != secret {
			t.Errorf("client_secret = %v, want %s", got["client_secret"], secret)
		}
		if got["code"] != "the-code" {
			t.Errorf("code = %v, want the-code", got["code"])
		}
		if got["grant_type"] != "authorization_code" {
			t.Errorf("grant_type = %v, want authorization_code", got["grant_type"])
		}
		if got["redirect_uri"] != "https://app.example.com/auth/callback" {
			t.Errorf("redirect_uri = %v", got["redirect_uri"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"should-be-discarded","scope":"read_products"}`))
	}))
	defer tokenSrv.Close()

	cfg := OAuthConfig{
		ClientID:     "client-abc",
		ClientSecret: secret,
		RedirectURI:  "https://app.example.com/auth/callback",
		Scopes:       "read_products write_products",
		InstallPath:  "/auth",
		CallbackPath: "/auth/callback",
		TokenURL:     func(shop string) string { return tokenSrv.URL },
		NewState:     func() (string, error) { return "STATE123", nil },
	}
	h := newTestHandler(cfg)
	// The callback only accepts states the install route issued.
	install(t, h, "demo.myshoplazza.com")

	q := url.Values{}
	q.Set("shop", "demo.myshoplazza.com")
	q.Set("code", "the-code")
	q.Set("state", "STATE123")
	q.Set("timestamp", "1700000000")
	q.Set("hmac", testHMAC(t, q, secret))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%s)", rr.Code, http.StatusFound, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "/" {
		t.Fatalf("Location = %q, want /", got)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("token endpoint hit %d times, want 1", n)
	}

	// States are single-use: replaying the exact same (valid-HMAC) callback
	// must be rejected without another token exchange.
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/auth/callback?"+q.Encode(), nil))
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("replayed callback status = %d, want %d", rr2.Code, http.StatusBadRequest)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("token endpoint hit %d times after replay, want still 1", n)
	}
}

// TestCallback_UnknownState_400 locks the CSRF guard: a callback whose state
// was never issued by this server's install route must 400 before the token
// exchange, even with a valid HMAC.
func TestCallback_UnknownState_400(t *testing.T) {
	const secret = "shh-secret"
	var hits int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer tokenSrv.Close()

	h := newTestHandler(OAuthConfig{
		TokenURL: func(shop string) string { return tokenSrv.URL },
	})

	q := url.Values{}
	q.Set("shop", "demo.myshoplazza.com")
	q.Set("code", "the-code")
	q.Set("state", "FORGED")
	q.Set("hmac", testHMAC(t, q, secret))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/callback?"+q.Encode(), nil))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "Invalid state parameter") {
		t.Fatalf("body = %q, want it to contain %q", rr.Body.String(), "Invalid state parameter")
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("token endpoint hit %d times, want 0", n)
	}
}

func TestRoot_ServesHello(t *testing.T) {
	h := newTestHandler(OAuthConfig{InstallPath: "/auth", CallbackPath: "/auth/callback"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Hello") {
		t.Fatalf("root body should contain Hello; got %q", rr.Body.String())
	}
}

func TestCallback_BadHMAC_400(t *testing.T) {
	const secret = "shh-secret"
	var hits int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer tokenSrv.Close()

	cfg := OAuthConfig{
		ClientID:     "client-abc",
		ClientSecret: secret,
		RedirectURI:  "https://app.example.com/auth/callback",
		Scopes:       "read_products",
		InstallPath:  "/auth",
		CallbackPath: "/auth/callback",
		TokenURL:     func(shop string) string { return tokenSrv.URL },
	}
	h := newTestHandler(cfg)

	q := url.Values{}
	q.Set("shop", "demo.myshoplazza.com")
	q.Set("code", "the-code")
	q.Set("state", "STATE123")
	q.Set("hmac", "deadbeefdeadbeef") // tampered / invalid

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "HMAC validation failed") {
		t.Fatalf("body = %q, want it to contain %q", rr.Body.String(), "HMAC validation failed")
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("token endpoint hit %d times, want 0", n)
	}
}

func TestCallback_MissingParams_400(t *testing.T) {
	const secret = "shh-secret"
	var hits int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer tokenSrv.Close()

	cfg := OAuthConfig{
		ClientID:     "client-abc",
		ClientSecret: secret,
		RedirectURI:  "https://app.example.com/auth/callback",
		Scopes:       "read_products",
		InstallPath:  "/auth",
		CallbackPath: "/auth/callback",
		TokenURL:     func(shop string) string { return tokenSrv.URL },
	}
	h := newTestHandler(cfg)

	// Valid hmac over {shop} only — no code -> passes HMAC, fails required-params.
	q := url.Values{}
	q.Set("shop", "demo.myshoplazza.com")
	q.Set("hmac", testHMAC(t, q, secret))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "Required parameters missing") {
		t.Fatalf("body = %q, want it to contain %q", rr.Body.String(), "Required parameters missing")
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("token endpoint hit %d times, want 0", n)
	}
}
