package sidecar

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Interceptor is the client-side (sandbox) half of the sidecar: an
// http.RoundTripper that detects a sentinel token on an outgoing request,
// HMAC-signs a canonical description of it, and reroutes it to the trusted
// sidecar — which injects the real token. Requests without a sentinel pass
// through untouched, so non-API traffic (downloads, OSS uploads) is unaffected.
type Interceptor struct {
	key         []byte
	sidecarHost string // host:port of the local sidecar, e.g. "127.0.0.1:16384"
	base        http.RoundTripper
	now         func() time.Time
}

// NewInterceptor wraps base so sentinel-bearing requests are signed and routed
// to sidecarHost. If base is nil, http.DefaultTransport is used.
func NewInterceptor(key []byte, sidecarHost string, base http.RoundTripper) *Interceptor {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Interceptor{key: key, sidecarHost: sidecarHost, base: base, now: time.Now}
}

// detectSentinel reports the identity and auth-header a request is managed under,
// or ("","") if it carries no sentinel and should pass through untouched.
func detectSentinel(req *http.Request) (identity, authHeader string) {
	if req.Header.Get(AuthHeaderAccessToken) == SentinelStore {
		return IdentityStore, AuthHeaderAccessToken
	}
	if req.Header.Get(AuthHeaderPartnerToken) == SentinelPartner {
		return IdentityPartner, AuthHeaderPartnerToken
	}
	return "", ""
}

func (i *Interceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	identity, authHeader := detectSentinel(req)
	if identity == "" {
		return i.base.RoundTrip(req) // not sidecar-managed
	}

	// Capture the real target BEFORE rewriting the URL — the signature binds to
	// where the caller intended to go, not to the sidecar.
	targetScheme := req.URL.Scheme
	targetHost := req.URL.Host
	pathAndQuery := req.URL.RequestURI()

	// Buffer the body so we can hash it and still send it.
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = b
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	ts := strconv.FormatInt(i.now().Unix(), 10)
	cr := CanonicalRequest{
		Version:      ProtocolV1,
		Method:       req.Method,
		Host:         targetHost,
		PathAndQuery: pathAndQuery,
		BodySHA256:   BodySHA256(body),
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	}
	sig := Sign(i.key, cr)

	// Strip the sentinel — the sandbox must never present even a placeholder as
	// if it were the token — and attach the signed proxy metadata.
	req.Header.Del(authHeader)
	req.Header.Set(HeaderProxyVersion, ProtocolV1)
	req.Header.Set(HeaderProxyTarget, targetScheme+"://"+targetHost)
	req.Header.Set(HeaderProxyIdentity, identity)
	req.Header.Set(HeaderProxyAuthHeader, authHeader)
	req.Header.Set(HeaderBodySHA256, cr.BodySHA256)
	req.Header.Set(HeaderProxyTimestamp, ts)
	req.Header.Set(HeaderProxySignature, sig)

	// Reroute to the sidecar over loopback http; the sidecar re-establishes TLS
	// to the real API. Path and query are preserved.
	req.URL.Scheme = "http"
	req.URL.Host = i.sidecarHost
	req.Host = i.sidecarHost

	return i.base.RoundTrip(req)
}
