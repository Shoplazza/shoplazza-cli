// Package sidecar defines the wire protocol shared by the credential-isolation
// sidecar's two halves: the client-side transport interceptor (runs in the
// untrusted sandbox, holds only sentinel tokens) and the server-side handler
// (runs on the trusted host, injects the real token and forwards to the API).
//
// The design is ported from the Lark CLI sidecar and adapted to Shoplazza:
// Shoplazza authenticates with the `Access-Token` header (store) and
// `Cli-Partner-Token` (partner), not `Authorization: Bearer`.
package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	// ProtocolV1 is the only wire version; the server rejects anything else.
	ProtocolV1 = "v1"

	// MaxTimestampDriftSeconds bounds the replay window: a request whose
	// timestamp differs from the server's clock by more than this is rejected.
	MaxTimestampDriftSeconds = 60

	// Proxy headers the interceptor sets and the server reads. Every one except
	// the signature itself is folded into the signed canonical request.
	HeaderProxyVersion    = "X-Shoplazza-Proxy-Version"
	HeaderProxyTarget     = "X-Shoplazza-Proxy-Target"      // scheme://host of the real API
	HeaderProxyIdentity   = "X-Shoplazza-Proxy-Identity"    // IdentityStore | IdentityPartner
	HeaderProxyAuthHeader = "X-Shoplazza-Proxy-Auth-Header" // which header the token goes into
	HeaderProxySignature  = "X-Shoplazza-Proxy-Signature"   // hex HMAC-SHA256 (not itself signed)
	HeaderProxyTimestamp  = "X-Shoplazza-Proxy-Timestamp"   // unix seconds
	HeaderBodySHA256      = "X-Shoplazza-Body-SHA256"       // hex sha256 of the body
)

// Auth headers that may carry the real token. The server injects a real token
// ONLY into one of these; any other requested header is rejected, so a captured
// request cannot smuggle the token into Cookie / User-Agent / etc. Shoplazza
// uses Access-Token (store) and Cli-Partner-Token (partner); Authorization is
// accepted for endpoints that use it.
const (
	AuthHeaderAccessToken   = "Access-Token"
	AuthHeaderPartnerToken  = "Cli-Partner-Token"
	AuthHeaderAuthorization = "Authorization"
)

// Identity types. Shoplazza's real split is store-level vs partner/account.
const (
	IdentityStore   = "store"
	IdentityPartner = "partner"
)

// Sentinel tokens the sandbox uses in place of real credentials. The credential
// provider returns these; the interceptor detects them to know a request is
// sidecar-managed and which identity/header applies. They are not secrets.
const (
	SentinelStore   = "sidecar-managed-store"
	SentinelPartner = "sidecar-managed-partner"
)

// allowedAuthHeaders is the injection allowlist enforced by the server.
var allowedAuthHeaders = map[string]bool{
	AuthHeaderAccessToken:   true,
	AuthHeaderPartnerToken:  true,
	AuthHeaderAuthorization: true,
}

// AuthHeaderAllowed reports whether the server may inject a real token into h.
func AuthHeaderAllowed(h string) bool { return allowedAuthHeaders[h] }

// allowedIdentities is the identity allowlist enforced by the server.
var allowedIdentities = map[string]bool{
	IdentityStore:   true,
	IdentityPartner: true,
}

// IdentityAllowed reports whether identity is a recognised type.
func IdentityAllowed(identity string) bool { return allowedIdentities[identity] }

// CanonicalRequest is the exact, ordered set of fields covered by the HMAC
// signature. Signing identity and auth-header (not just method/path/body) means
// a captured request cannot be replayed with its identity flipped or its token
// redirected into a different header. Field ORDER is the protocol contract —
// never reorder without bumping the version.
type CanonicalRequest struct {
	Version      string
	Method       string
	Host         string
	PathAndQuery string
	BodySHA256   string
	Timestamp    string
	Identity     string
	AuthHeader   string
}

// canonicalString renders the request as the newline-delimited string that is
// fed to HMAC. The order matches the struct fields above and is load-bearing.
func (r CanonicalRequest) canonicalString() string {
	return strings.Join([]string{
		r.Version,
		r.Method,
		r.Host,
		r.PathAndQuery,
		r.BodySHA256,
		r.Timestamp,
		r.Identity,
		r.AuthHeader,
	}, "\n")
}

// BodySHA256 returns the hex-encoded SHA-256 of a request body, used to bind the
// body to the signature so it cannot be swapped in transit.
func BodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
