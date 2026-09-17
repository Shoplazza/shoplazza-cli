package sidecar

import (
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func testKey() []byte {
	k := make([]byte, KeySize)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

// fixedNow is a deterministic clock so timestamp-drift tests don't flake.
var fixedNow = time.Unix(1_700_000_000, 0)

func sampleReq() CanonicalRequest {
	return CanonicalRequest{
		Version:      ProtocolV1,
		Method:       "POST",
		Host:         "openapi.shoplazza.com",
		PathAndQuery: "/openapi/2026-01/orders?limit=10",
		BodySHA256:   BodySHA256([]byte(`{"order":{}}`)),
		Timestamp:    strconv.FormatInt(fixedNow.Unix(), 10),
		Identity:     IdentityStore,
		AuthHeader:   AuthHeaderAccessToken,
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	key := testKey()
	req := sampleReq()
	sig := Sign(key, req)
	if err := Verify(key, req, sig, fixedNow); err != nil {
		t.Fatalf("valid signature should verify: %v", err)
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	req := sampleReq()
	sig := Sign(testKey(), req)
	other := make([]byte, KeySize) // all-zero, different key
	if err := Verify(other, req, sig, fixedNow); err == nil {
		t.Error("signature under a different key must not verify")
	}
}

// TestVerifyRejectsTampering is the core security property: changing ANY signed
// field (including identity and auth-header) invalidates the signature.
func TestVerifyRejectsTampering(t *testing.T) {
	key := testKey()
	base := sampleReq()
	sig := Sign(key, base)

	mutations := map[string]func(*CanonicalRequest){
		"method":     func(r *CanonicalRequest) { r.Method = "GET" },
		"host":       func(r *CanonicalRequest) { r.Host = "evil.example.com" },
		"path":       func(r *CanonicalRequest) { r.PathAndQuery = "/openapi/2026-01/orders?limit=250" },
		"body":       func(r *CanonicalRequest) { r.BodySHA256 = BodySHA256([]byte("different")) },
		"identity":   func(r *CanonicalRequest) { r.Identity = IdentityPartner },
		"authHeader": func(r *CanonicalRequest) { r.AuthHeader = AuthHeaderPartnerToken },
	}
	for name, mutate := range mutations {
		tampered := base
		mutate(&tampered)
		if err := Verify(key, tampered, sig, fixedNow); err == nil {
			t.Errorf("tampering with %s must invalidate the signature", name)
		}
	}
}

func TestVerifyRejectsStaleTimestamp(t *testing.T) {
	key := testKey()
	req := sampleReq()
	sig := Sign(key, req)

	// 61s in the future exceeds the ±60s window.
	late := fixedNow.Add(61 * time.Second)
	if err := Verify(key, req, sig, late); err == nil {
		t.Error("a timestamp beyond the drift window must be rejected")
	}
	// 60s is on the boundary and still allowed.
	edge := fixedNow.Add(60 * time.Second)
	if err := Verify(key, req, sig, edge); err != nil {
		t.Errorf("a timestamp within the drift window should pass: %v", err)
	}
}

func TestVerifyRejectsNonNumericTimestamp(t *testing.T) {
	key := testKey()
	req := sampleReq()
	req.Timestamp = "not-a-number"
	if err := Verify(key, req, Sign(key, req), fixedNow); err == nil {
		t.Error("a non-numeric timestamp must be rejected")
	}
}

func TestBodySHA256Deterministic(t *testing.T) {
	a := BodySHA256([]byte("hello"))
	b := BodySHA256([]byte("hello"))
	if a != b {
		t.Fatal("BodySHA256 must be deterministic")
	}
	if a == BodySHA256([]byte("world")) {
		t.Fatal("different bodies must hash differently")
	}
	if len(a) != 64 {
		t.Errorf("sha256 hex should be 64 chars, got %d", len(a))
	}
}

func TestDecodeKey(t *testing.T) {
	good := hex.EncodeToString(testKey())
	if _, err := DecodeKey(good); err != nil {
		t.Errorf("a valid 32-byte hex key should decode: %v", err)
	}
	if _, err := DecodeKey("zz"); err == nil {
		t.Error("non-hex must fail")
	}
	if _, err := DecodeKey(hex.EncodeToString([]byte("short"))); err == nil {
		t.Error("a wrong-length key must fail")
	}
}
