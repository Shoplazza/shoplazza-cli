package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/sidecar"
)

func key32() []byte {
	k := make([]byte, sidecar.KeySize)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

var now = time.Unix(1_700_000_000, 0)

type fakeResolver struct {
	token string
	err   error
}

func (f fakeResolver) Resolve(_ context.Context, _ string) (string, error) { return f.token, f.err }

type fakeUpstream struct {
	last   *http.Request
	status int
}

func (f *fakeUpstream) RoundTrip(req *http.Request) (*http.Response, error) {
	f.last = req
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader("upstream-ok")), Header: http.Header{}}, nil
}

// signedReq builds the request the server would receive from a well-behaved
// interceptor: path-only URL, proxy headers set, valid signature.
func signedReq(key []byte, method, target, body, identity, authHeader string) *http.Request {
	tu, _ := url.Parse(target)
	ts := strconv.FormatInt(now.Unix(), 10)
	cr := sidecar.CanonicalRequest{
		Version:      sidecar.ProtocolV1,
		Method:       method,
		Host:         tu.Host,
		PathAndQuery: tu.RequestURI(),
		BodySHA256:   sidecar.BodySHA256([]byte(body)),
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	}
	r := httptest.NewRequest(method, tu.RequestURI(), strings.NewReader(body))
	r.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	r.Header.Set(sidecar.HeaderProxyTarget, tu.Scheme+"://"+tu.Host)
	r.Header.Set(sidecar.HeaderProxyIdentity, identity)
	r.Header.Set(sidecar.HeaderProxyAuthHeader, authHeader)
	r.Header.Set(sidecar.HeaderBodySHA256, cr.BodySHA256)
	r.Header.Set(sidecar.HeaderProxyTimestamp, ts)
	r.Header.Set(sidecar.HeaderProxySignature, sidecar.Sign(key, cr))
	return r
}

func testHandler(up *fakeUpstream, res TokenResolver) *Handler {
	h := New(key32(), []string{"openapi.shoplazza.com"}, res, nil, up)
	h.now = func() time.Time { return now }
	return h
}

func TestForwardsWithRealTokenInjected(t *testing.T) {
	up := &fakeUpstream{status: 200}
	h := testHandler(up, fakeResolver{token: "REAL-STORE-TOKEN"})

	r := signedReq(key32(), "POST", "https://openapi.shoplazza.com/openapi/2026-01/orders?limit=10",
		`{"order":{}}`, sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != 200 || w.Body.String() != "upstream-ok" {
		t.Fatalf("want 200/upstream-ok, got %d/%q", w.Code, w.Body.String())
	}
	if up.last == nil {
		t.Fatal("upstream was not called")
	}
	// Real token injected into the allowlisted header.
	if up.last.Header.Get(sidecar.AuthHeaderAccessToken) != "REAL-STORE-TOKEN" {
		t.Errorf("real token not injected: %q", up.last.Header.Get(sidecar.AuthHeaderAccessToken))
	}
	// Pinned https + real host + preserved path.
	if up.last.URL.Scheme != "https" || up.last.URL.Host != "openapi.shoplazza.com" {
		t.Errorf("upstream URL wrong: %s", up.last.URL)
	}
	if up.last.URL.RequestURI() != "/openapi/2026-01/orders?limit=10" {
		t.Errorf("path not preserved: %s", up.last.URL.RequestURI())
	}
	// No proxy headers leak upstream.
	if up.last.Header.Get(sidecar.HeaderProxySignature) != "" || up.last.Header.Get(sidecar.HeaderProxyTarget) != "" {
		t.Error("proxy headers must be stripped before forwarding upstream")
	}
}

func TestRejectsTamperedSignature(t *testing.T) {
	up := &fakeUpstream{status: 200}
	h := testHandler(up, fakeResolver{token: "REAL"})
	r := signedReq(key32(), "GET", "https://openapi.shoplazza.com/openapi/2026-01/orders", "",
		sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	r.Header.Set(sidecar.HeaderProxySignature, "deadbeef") // tamper
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("tampered signature must be 401, got %d", w.Code)
	}
	if up.last != nil {
		t.Error("upstream must NOT be called on a bad signature")
	}
}

func TestRejectsDisallowedHost(t *testing.T) {
	up := &fakeUpstream{status: 200}
	h := testHandler(up, fakeResolver{token: "REAL"})
	r := signedReq(key32(), "GET", "https://evil.example.com/steal", "",
		sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-allowlisted host must be 403, got %d", w.Code)
	}
	if up.last != nil {
		t.Error("upstream must NOT be called for a disallowed host")
	}
}

func TestRejectsBodyTamper(t *testing.T) {
	up := &fakeUpstream{status: 200}
	h := testHandler(up, fakeResolver{token: "REAL"})
	r := signedReq(key32(), "POST", "https://openapi.shoplazza.com/openapi/2026-01/orders", `{"a":1}`,
		sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	// Swap the body without re-signing / re-hashing.
	r.Body = io.NopCloser(strings.NewReader(`{"a":999}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("a swapped body must be 400 (hash mismatch), got %d", w.Code)
	}
}

func TestRejectsBadVersion(t *testing.T) {
	up := &fakeUpstream{status: 200}
	h := testHandler(up, fakeResolver{token: "REAL"})
	r := signedReq(key32(), "GET", "https://openapi.shoplazza.com/x", "",
		sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	r.Header.Set(sidecar.HeaderProxyVersion, "v99")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown version must be 400, got %d", w.Code)
	}
}

func TestSanitizePathHidesIDsAndQuery(t *testing.T) {
	got := sanitizePath("/openapi/2026-01/orders/6f8a1b2c3d4e/refunds?token=secret")
	if strings.Contains(got, "secret") || strings.Contains(got, "6f8a1b2c3d4e") {
		t.Errorf("audit path must not leak ids or query: %q", got)
	}
	if got != "/openapi/2026-01/orders/:id/refunds" {
		t.Errorf("got %q", got)
	}
}
