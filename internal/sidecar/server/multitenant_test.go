package server

import (
	"context"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/sidecar"
)

func key32b() []byte {
	k := make([]byte, sidecar.KeySize)
	for i := range k {
		k[i] = byte(255 - i)
	}
	return k
}

func writeKey(t *testing.T, dir, name string, key []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".key"), []byte(hex.EncodeToString(key)), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

// fakeTenants is a TenantResolver stub: per-client token + allowed host.
type fakeTenants struct {
	token map[string]string
	host  map[string]string
}

func (f fakeTenants) AllowHost(client, host string) bool { return f.host[client] == host }
func (f fakeTenants) Token(_ context.Context, client, _ string) (string, error) {
	return f.token[client], nil
}

func TestMultiAuthIdentifiesClientByKey(t *testing.T) {
	dir := t.TempDir()
	writeKey(t, dir, "alice", key32())
	writeKey(t, dir, "bob", key32b())
	ck, err := LoadClientKeys(dir)
	if err != nil {
		t.Fatalf("load keys: %v", err)
	}
	auth := NewMultiAuth(ck)

	cr := sampleCR()
	if client, err := auth.Identify(cr, sidecar.Sign(key32(), cr), now); err != nil || client != "alice" {
		t.Errorf("alice-signed request should identify as alice, got %q err=%v", client, err)
	}
	if client, err := auth.Identify(cr, sidecar.Sign(key32b(), cr), now); err != nil || client != "bob" {
		t.Errorf("bob-signed request should identify as bob, got %q err=%v", client, err)
	}
	// A key that isn't provisioned must be rejected — no fallback.
	stranger := make([]byte, sidecar.KeySize)
	if _, err := auth.Identify(cr, sidecar.Sign(stranger, cr), now); err == nil {
		t.Error("an unknown client key must be rejected")
	}
}

// sampleCR is a canonical request for alice's store.
func sampleCR() sidecar.CanonicalRequest {
	return sidecar.CanonicalRequest{
		Version:      sidecar.ProtocolV1,
		Method:       "GET",
		Host:         "alice.myshoplazza.com",
		PathAndQuery: "/openapi/2026-01/orders",
		BodySHA256:   sidecar.BodySHA256(nil),
		Timestamp:    "1700000000",
		Identity:     sidecar.IdentityStore,
		AuthHeader:   sidecar.AuthHeaderAccessToken,
	}
}

func TestMultiTenantForwardsWithClientToken(t *testing.T) {
	dir := t.TempDir()
	writeKey(t, dir, "alice", key32())
	ck, _ := LoadClientKeys(dir)
	up := &fakeUpstream{status: 200}
	tenants := fakeTenants{
		token: map[string]string{"alice": "ALICE-STORE-TOKEN"},
		host:  map[string]string{"alice": "alice.myshoplazza.com"},
	}
	h := NewMultiTenant(NewMultiAuth(ck), tenants, nil, up)
	h.now = func() time.Time { return now }

	r := signedReq(key32(), "GET", "https://alice.myshoplazza.com/openapi/2026-01/orders", "",
		sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if up.last.Header.Get(sidecar.AuthHeaderAccessToken) != "ALICE-STORE-TOKEN" {
		t.Errorf("alice's own token must be injected, got %q", up.last.Header.Get(sidecar.AuthHeaderAccessToken))
	}
}

func TestMultiTenantRejectsCrossHost(t *testing.T) {
	dir := t.TempDir()
	writeKey(t, dir, "alice", key32())
	ck, _ := LoadClientKeys(dir)
	up := &fakeUpstream{status: 200}
	// alice is only allowed to reach her own store.
	tenants := fakeTenants{
		token: map[string]string{"alice": "ALICE"},
		host:  map[string]string{"alice": "alice.myshoplazza.com"},
	}
	h := NewMultiTenant(NewMultiAuth(ck), tenants, nil, up)
	h.now = func() time.Time { return now }

	// alice signs a request aimed at BOB's store — must be forbidden.
	r := signedReq(key32(), "GET", "https://bob.myshoplazza.com/openapi/2026-01/orders", "",
		sidecar.IdentityStore, sidecar.AuthHeaderAccessToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Errorf("a client reaching another tenant's host must be 403, got %d", w.Code)
	}
	if up.last != nil {
		t.Error("upstream must not be called on a cross-host attempt")
	}
}
