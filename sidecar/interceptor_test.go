package sidecar

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// capturingRT records the request it last saw and returns a canned 200.
type capturingRT struct{ last *http.Request }

func (c *capturingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	c.last = req
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: http.Header{}}, nil
}

func newTestInterceptor(rt http.RoundTripper) *Interceptor {
	i := NewInterceptor(testKey(), "127.0.0.1:16384", rt)
	i.now = func() time.Time { return fixedNow }
	return i
}

func TestInterceptorReroutesAndSignsSentinelRequest(t *testing.T) {
	rt := &capturingRT{}
	i := newTestInterceptor(rt)

	body := `{"order":{"id":"1"}}`
	req, _ := http.NewRequest("POST", "https://openapi.shoplazza.com/openapi/2026-01/orders?limit=10", strings.NewReader(body))
	req.Header.Set(AuthHeaderAccessToken, SentinelStore)

	if _, err := i.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	got := rt.last

	// Rerouted to the sidecar over http; path+query preserved.
	if got.URL.Scheme != "http" || got.URL.Host != "127.0.0.1:16384" {
		t.Errorf("want reroute to http://127.0.0.1:16384, got %s://%s", got.URL.Scheme, got.URL.Host)
	}
	if got.URL.RequestURI() != "/openapi/2026-01/orders?limit=10" {
		t.Errorf("path/query must be preserved, got %s", got.URL.RequestURI())
	}
	// The sentinel must be gone — the sandbox presents no token at all.
	if got.Header.Get(AuthHeaderAccessToken) != "" {
		t.Error("sentinel Access-Token must be stripped before leaving the sandbox")
	}
	// Proxy metadata present and consistent.
	if got.Header.Get(HeaderProxyTarget) != "https://openapi.shoplazza.com" {
		t.Errorf("proxy target: got %q", got.Header.Get(HeaderProxyTarget))
	}
	if got.Header.Get(HeaderProxyIdentity) != IdentityStore {
		t.Errorf("identity: got %q", got.Header.Get(HeaderProxyIdentity))
	}

	// The signature the server would reconstruct must verify.
	target, _ := url.Parse(got.Header.Get(HeaderProxyTarget))
	cr := CanonicalRequest{
		Version:      got.Header.Get(HeaderProxyVersion),
		Method:       got.Method,
		Host:         target.Host,
		PathAndQuery: got.URL.RequestURI(),
		BodySHA256:   got.Header.Get(HeaderBodySHA256),
		Timestamp:    got.Header.Get(HeaderProxyTimestamp),
		Identity:     got.Header.Get(HeaderProxyIdentity),
		AuthHeader:   got.Header.Get(HeaderProxyAuthHeader),
	}
	if err := Verify(testKey(), cr, got.Header.Get(HeaderProxySignature), fixedNow); err != nil {
		t.Errorf("server-side verification of the interceptor's signature failed: %v", err)
	}
	// Body must survive the round trip for the sidecar to forward it.
	if got.Body != nil {
		b, _ := io.ReadAll(got.Body)
		if !bytes.Equal(b, []byte(body)) {
			t.Errorf("body must be preserved, got %q", b)
		}
	}
	// The body hash in the header must match the actual body.
	if got.Header.Get(HeaderBodySHA256) != BodySHA256([]byte(body)) {
		t.Error("body hash header must match the body")
	}
}

func TestInterceptorPassesThroughNonSentinel(t *testing.T) {
	rt := &capturingRT{}
	i := newTestInterceptor(rt)

	req, _ := http.NewRequest("GET", "https://cdn.example.com/theme.zip", nil)
	req.Header.Set(AuthHeaderAccessToken, "a-real-looking-but-not-sentinel-token")

	if _, err := i.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	// Untouched: still pointed at the original host, no proxy headers.
	if rt.last.URL.Host != "cdn.example.com" {
		t.Errorf("non-sentinel request must pass through untouched, host=%s", rt.last.URL.Host)
	}
	if rt.last.Header.Get(HeaderProxySignature) != "" {
		t.Error("non-sentinel request must not be signed/rerouted")
	}
}

func TestInterceptorPartnerIdentity(t *testing.T) {
	rt := &capturingRT{}
	i := newTestInterceptor(rt)

	req, _ := http.NewRequest("GET", "https://openapi.shoplazza.com/openapi/partner/apps", nil)
	req.Header.Set(AuthHeaderPartnerToken, SentinelPartner)

	if _, err := i.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if rt.last.Header.Get(HeaderProxyIdentity) != IdentityPartner {
		t.Errorf("partner sentinel should map to partner identity, got %q", rt.last.Header.Get(HeaderProxyIdentity))
	}
	if rt.last.Header.Get(HeaderProxyAuthHeader) != AuthHeaderPartnerToken {
		t.Errorf("auth-header should be Cli-Partner-Token, got %q", rt.last.Header.Get(HeaderProxyAuthHeader))
	}
	if rt.last.Header.Get(AuthHeaderPartnerToken) != "" {
		t.Error("partner sentinel must be stripped")
	}
}
