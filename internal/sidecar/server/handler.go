// Package server is the trusted-host half of the credential-isolation sidecar.
// It verifies HMAC-signed proxy requests from a sandboxed CLI, injects the real
// token (resolved out of band, never seen by the sandbox), and forwards to the
// Shoplazza API. It is meant to be compiled into a small standalone binary run
// on a trusted host — not linked into the user-facing CLI.
//
// The same pipeline serves both tenancy models: an Authenticator turns a signed
// request into a client identity ("" for single-tenant, a client name for
// multi-tenant), and a TenantResolver maps that client to its allowed target
// host and the real token to inject.
package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/sidecar"
)

// Authenticator verifies a request's signature and returns the client it belongs
// to. Single-tenant returns "" (one shared key); multi-tenant returns the client
// whose per-client key validated the signature.
type Authenticator interface {
	Identify(cr sidecar.CanonicalRequest, signature string, now time.Time) (client string, err error)
}

// TenantResolver maps a verified client to what it may do: which API host it may
// reach, and the real token to inject for a given identity. It is the only thing
// that ever touches real credentials.
type TenantResolver interface {
	AllowHost(client, host string) bool
	Token(ctx context.Context, client, identity string) (string, error)
}

// TokenResolver is the single-tenant token source (one configured profile). It
// is adapted to a TenantResolver by New via singleTenant.
type TokenResolver interface {
	Resolve(ctx context.Context, identity string) (token string, err error)
}

// Handler is the sidecar's HTTP handler. Build it with New (single-tenant) or
// NewMultiTenant.
type Handler struct {
	auth     Authenticator
	tenants  TenantResolver
	upstream http.RoundTripper
	logger   *log.Logger
	now      func() time.Time
}

// New builds a single-tenant Handler: one shared HMAC key, one token source, a
// fixed set of allowed API hosts.
func New(key []byte, allowedHosts []string, resolver TokenResolver, logger *log.Logger, upstream http.RoundTripper) *Handler {
	hosts := make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		hosts[h] = true
	}
	return newHandler(singleAuth{key: key}, singleTenant{hosts: hosts, resolver: resolver}, logger, upstream)
}

// NewMultiTenant builds a multi-tenant Handler: per-client keys identify the
// client, and tenants resolves that client's host + token in isolation.
func NewMultiTenant(auth Authenticator, tenants TenantResolver, logger *log.Logger, upstream http.RoundTripper) *Handler {
	return newHandler(auth, tenants, logger, upstream)
}

func newHandler(auth Authenticator, tenants TenantResolver, logger *log.Logger, upstream http.RoundTripper) *Handler {
	if upstream == nil {
		upstream = http.DefaultTransport
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Handler{auth: auth, tenants: tenants, upstream: upstream, logger: logger, now: time.Now}
}

// singleAuth verifies against one shared key and reports no client name.
type singleAuth struct{ key []byte }

func (s singleAuth) Identify(cr sidecar.CanonicalRequest, signature string, now time.Time) (string, error) {
	return "", sidecar.Verify(s.key, cr, signature, now)
}

// singleTenant serves one profile: a fixed host allowlist and one token source.
type singleTenant struct {
	hosts    map[string]bool
	resolver TokenResolver
}

func (s singleTenant) AllowHost(_, host string) bool { return s.hosts[host] }
func (s singleTenant) Token(ctx context.Context, _, identity string) (string, error) {
	return s.resolver.Resolve(ctx, identity)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.now()
	pathAndQuery := r.URL.RequestURI()

	reject := func(status int, reason string) {
		http.Error(w, reason, status)
		h.logger.Printf("REJECT method=%s path=%s status=%d reason=%q",
			r.Method, sanitizePath(pathAndQuery), status, reason)
	}

	// 0 — protocol version.
	if r.Header.Get(sidecar.HeaderProxyVersion) != sidecar.ProtocolV1 {
		reject(http.StatusBadRequest, "unsupported proxy version")
		return
	}

	// 1 — read + hash body, confirm it matches the signed digest.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		reject(http.StatusBadRequest, "cannot read body")
		return
	}
	if sidecar.BodySHA256(body) != r.Header.Get(sidecar.HeaderBodySHA256) {
		reject(http.StatusBadRequest, "body hash mismatch")
		return
	}

	// Reconstruct the canonical request from the (untrusted) headers. The target
	// host comes from the signed proxy-target, not the Host header.
	target, err := url.Parse(r.Header.Get(sidecar.HeaderProxyTarget))
	if err != nil || target.Host == "" {
		reject(http.StatusBadRequest, "invalid proxy target")
		return
	}
	identity := r.Header.Get(sidecar.HeaderProxyIdentity)
	authHeader := r.Header.Get(sidecar.HeaderProxyAuthHeader)
	cr := sidecar.CanonicalRequest{
		Version:      sidecar.ProtocolV1,
		Method:       r.Method,
		Host:         target.Host,
		PathAndQuery: pathAndQuery,
		BodySHA256:   r.Header.Get(sidecar.HeaderBodySHA256),
		Timestamp:    r.Header.Get(sidecar.HeaderProxyTimestamp),
		Identity:     identity,
		AuthHeader:   authHeader,
	}

	// 2 — signature (covers method/host/path/body/timestamp/identity/header) with
	// replay-window enforcement; also identifies the client (multi-tenant).
	client, err := h.auth.Identify(cr, r.Header.Get(sidecar.HeaderProxySignature), start)
	if err != nil {
		reject(http.StatusUnauthorized, "signature verification failed")
		return
	}

	// 3 — identity + auth-header allowlists (token-smuggling guard).
	if !sidecar.IdentityAllowed(identity) {
		reject(http.StatusForbidden, "identity not allowed: "+identity)
		return
	}
	if !sidecar.AuthHeaderAllowed(authHeader) {
		reject(http.StatusForbidden, "auth-header not allowed: "+authHeader)
		return
	}

	// 4 — SSRF / downgrade guard: the client must be allowed to reach this host.
	if !h.tenants.AllowHost(client, target.Host) {
		reject(http.StatusForbidden, "target host not allowed: "+target.Host)
		return
	}

	// 5 — resolve the real token for THIS client (no cross-tenant fallback).
	token, err := h.tenants.Token(r.Context(), client, identity)
	if err != nil {
		http.Error(w, "token resolution failed", http.StatusBadGateway)
		h.logger.Printf("TOKEN_ERROR method=%s path=%s client=%s identity=%s error=%q",
			r.Method, sanitizePath(pathAndQuery), client, identity, sanitizeError(err))
		return
	}

	// 6 — build the upstream request: pinned https, real host, real token in the
	// one allowlisted header; every proxy/auth header stripped.
	upstreamURL := "https://" + target.Host + pathAndQuery
	up, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, strings.NewReader(string(body)))
	if err != nil {
		reject(http.StatusInternalServerError, "cannot build upstream request")
		return
	}
	copySafeHeaders(up.Header, r.Header)
	up.Header.Set(authHeader, token)

	resp, err := h.upstream.RoundTrip(up)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		h.logger.Printf("UPSTREAM_ERROR method=%s path=%s client=%s identity=%s error=%q",
			r.Method, sanitizePath(pathAndQuery), client, identity, sanitizeError(err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// 7 — copy the upstream response back verbatim.
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)

	h.logger.Printf("FORWARD method=%s path=%s client=%s identity=%s status=%d duration=%s",
		r.Method, sanitizePath(pathAndQuery), client, identity, resp.StatusCode,
		h.now().Sub(start).Round(time.Millisecond))
}

// proxyHeaderPrefix marks headers the sandbox added for the sidecar; none are
// forwarded upstream.
const proxyHeaderPrefix = "X-Shoplazza-Proxy-"

// copySafeHeaders copies client headers to the upstream request, dropping the
// sidecar's own proxy headers and any auth headers (the server sets the real one).
func copySafeHeaders(dst, src http.Header) {
	for k, vs := range src {
		if strings.HasPrefix(k, proxyHeaderPrefix) || k == sidecar.HeaderBodySHA256 {
			continue
		}
		switch k {
		case sidecar.AuthHeaderAccessToken, sidecar.AuthHeaderPartnerToken, sidecar.AuthHeaderAuthorization, "Host":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// sanitizeError caps an upstream/resolver error so a long message can't bloat or
// leak into audit logs.
func sanitizeError(err error) string {
	s := err.Error()
	const max = 200
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// sanitizePath drops the query string and replaces id-like path segments with
// ":id" so audit logs never record resource ids or query params.
func sanitizePath(pathAndQuery string) string {
	p := pathAndQuery
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if looksLikeID(seg) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func looksLikeID(seg string) bool {
	if len(seg) < 8 {
		return false
	}
	for _, c := range seg {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}
