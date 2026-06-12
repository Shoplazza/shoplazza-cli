package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// OAuthConfig configures the install-handshake handler used by `app dev`.
//
// The handler completes the OAuth install handshake purely to exercise the
// flow during local development. The token returned by the exchange is
// DISCARDED — it is never written to keychain, config, or any file.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       string // space- or comma-separated; urlencoded into the authorize URL

	InstallPath  string // e.g. "/auth"
	CallbackPath string // e.g. "/auth/callback"

	HTTPClient *http.Client             // token exchange client (nil -> http.DefaultClient)
	TokenURL   func(shop string) string // nil -> https://<shop>/admin/oauth/token (injectable for tests)
	NewState   func() (string, error)   // nil -> 16 random bytes hex (injectable for tests)
}

// stateTTL bounds how long an issued state nonce stays valid: the callback must
// arrive within it or the handshake is rejected.
const stateTTL = 10 * time.Minute

// oauthServer carries the per-`app dev` run state for the install handshake:
// the static config plus the issued-state nonces handleCallback verifies.
type oauthServer struct {
	cfg OAuthConfig

	mu     sync.Mutex
	states map[string]time.Time // state nonce -> issued-at; deleted on use/expiry
}

// NewOAuthHandler returns an http.Handler (a *http.ServeMux) with the install,
// callback, and root routes mounted. Used by `app dev`'s devserver.
func NewOAuthHandler(cfg OAuthConfig) http.Handler {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.TokenURL == nil {
		cfg.TokenURL = func(shop string) string {
			return fmt.Sprintf("https://%s/admin/oauth/token", shop)
		}
	}
	if cfg.NewState == nil {
		cfg.NewState = defaultState
	}

	s := &oauthServer{cfg: cfg, states: map[string]time.Time{}}
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.InstallPath, s.handleInstall)
	mux.HandleFunc(cfg.CallbackPath, s.handleCallback)
	mux.HandleFunc("/", handleRoot)
	return mux
}

// storeState records a freshly issued state nonce, opportunistically dropping
// expired entries so the map can't grow unbounded.
func (s *oauthServer) storeState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, issued := range s.states {
		if now.Sub(issued) > stateTTL {
			delete(s.states, k)
		}
	}
	s.states[state] = now
}

// consumeState validates-and-deletes a state nonce: true only when it was
// issued by this server and is within stateTTL. Single-use by design.
func (s *oauthServer) consumeState(state string) bool {
	if state == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	issued, ok := s.states[state]
	if !ok {
		return false
	}
	delete(s.states, state)
	return time.Since(issued) <= stateTTL
}

// defaultState returns 16 random bytes, hex-encoded (matches v1's
// crypto.randomBytes(16).toString('hex')).
func defaultState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// shopHostPattern is the hostname shape `shop` must match before it is
// interpolated into the authorize URL: letters/digits/dots/hyphens only.
// Anything with a '/', ':', '?', '#' or whitespace could redirect the browser
// to an attacker-chosen authority.
var shopHostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// handleInstall validates `shop` from the query, generates + stores a state
// nonce, and 302 redirects to the store's authorize endpoint.
func (s *oauthServer) handleInstall(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" || !shopHostPattern.MatchString(shop) {
		http.Error(w, "Invalid shop parameter", http.StatusBadRequest)
		return
	}

	state, err := s.cfg.NewState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	s.storeState(state)

	// url.Values escapes every param (redirect_uri/state/shop included);
	// Encode emits keys in sorted order, which the authorize endpoint is
	// agnostic to.
	q := url.Values{}
	q.Set("client_id", s.cfg.ClientID)
	q.Set("scope", s.cfg.Scopes)
	q.Set("redirect_uri", s.cfg.RedirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	authorize := fmt.Sprintf("https://%s/admin/oauth/authorize?%s", shop, q.Encode())
	http.Redirect(w, r, authorize, http.StatusFound)
}

// handleCallback validates the HMAC and the state nonce, then completes the
// token exchange (and discards the token) before redirecting to "/" (the root
// page).
func (s *oauthServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	query := r.URL.Query()

	if !verifyHMAC(query, cfg.ClientSecret) {
		http.Error(w, "HMAC validation failed", http.StatusBadRequest)
		return
	}

	shop := query.Get("shop")
	hmacParam := query.Get("hmac")
	code := query.Get("code")
	if shop == "" || hmacParam == "" || code == "" {
		http.Error(w, "Required parameters missing", http.StatusBadRequest)
		return
	}

	// CSRF guard: the state must be one this server issued in handleInstall
	// (single-use, expires after stateTTL).
	if !s.consumeState(query.Get("state")) {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(map[string]string{
		"client_id":     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
		"redirect_uri":  cfg.RedirectURI,
	})
	if err != nil {
		http.Error(w, "Failed to build token request", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, cfg.TokenURL(shop), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Failed to build token request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		// The handshake couldn't complete (transport error).
		http.Error(w, "Token exchange failed", http.StatusBadGateway)
		return
	}
	// Discard the token: drain and close the body without persisting anything.
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "Token exchange failed", http.StatusBadGateway)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// handleRoot serves the bare tunnel URL — everything not matched by the install
// or callback routes. `app dev` mounts it at "/", so the root URL (and the
// post-handshake redirect target) shows a simple page instead of a 404.
func handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, rootHTML)
}

const rootHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Hello</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
         display: flex; min-height: 100vh; margin: 0; align-items: center;
         justify-content: center; background: #0b1220; color: #e6edf3; }
  h1 { font-size: 40px; margin: 0; }
</style>
</head>
<body>
  <h1>Hello</h1>
</body>
</html>
`

// verifyHMAC ports the v1 hmacValidatorMiddleWare exactly: take every query
// param except `hmac`, sort keys ascending, build the message as key=value
// joined by '&' (raw values), compute HMAC-SHA256(clientSecret, message) hex,
// and constant-time compare it against the `hmac` param.
func verifyHMAC(query url.Values, clientSecret string) bool {
	keys := make([]string, 0, len(query))
	for k := range query {
		if k == "hmac" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+query.Get(k))
	}
	message := strings.Join(parts, "&")

	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write([]byte(message))
	generated := hex.EncodeToString(mac.Sum(nil))

	// Compare the hex-string bytes in constant time. hmac.Equal returns false
	// on a length mismatch, so no length pre-check is needed.
	return hmac.Equal([]byte(generated), []byte(query.Get("hmac")))
}
