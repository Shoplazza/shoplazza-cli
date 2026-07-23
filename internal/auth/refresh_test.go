package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/lockfile"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

// writeExchangeEnvelope writes the real store-AT exchange response shape
// ({"code":"Success","data":{...}}) that client.PostJSON requires to unwrap.
func writeExchangeEnvelope(w http.ResponseWriter, accessToken string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
		"access_token": accessToken, "store_id": "1",
		"store_domain": "us.myshoplazza.com", "granted_scopes": []string{"read_product"},
		"at_expires_at": "2099-01-01T00:00:00Z",
	}})
}

// countingExchangeStub stubs the exchange endpoint and increments *calls.
// Single-goroutine callers only; concurrent tests use atomicCountingExchangeStub.
func countingExchangeStub(t *testing.T, calls *int, accessToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		writeExchangeEnvelope(w, accessToken)
	}))
}

// atomicCountingExchangeStub is the concurrency-safe variant for multi-goroutine tests.
func atomicCountingExchangeStub(t *testing.T, calls *int32, accessToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		writeExchangeEnvelope(w, accessToken)
	}))
}

// seedProfileToken seeds both the keychain store-AT and its ProfileMeta so
// cachedProfileToken's fast path / double-check can read it back.
func seedProfileToken(t *testing.T, authDir, name, token string, expiry time.Time) {
	t.Helper()
	if err := keychain.Set(keychain.ShoplazzaCliService, ProfileStoreKey(name), token); err != nil {
		t.Fatalf("seedProfileToken: keychain.Set: %v", err)
	}
	if err := SaveProfileMeta(authDir, strings.ToLower(name), ProfileMeta{ExpiresAt: expiry.Format(time.RFC3339)}); err != nil {
		t.Fatalf("seedProfileToken: SaveProfileMeta: %v", err)
	}
}

func TestAccessTokenReadyForProfile_CachedFastPath(t *testing.T) {
	testenv.IsolateConfigDir(t)

	// meta 未过期 + keychain 有 token → 不发任何 HTTP
	calls := 0
	srv := countingExchangeStub(t, &calls, "at-new")
	defer srv.Close()
	m := &Manager{Client: client.New(srv.URL)}
	cp := filepath.Join(t.TempDir(), "config.json")
	seedProfileToken(t, AuthDir(cp), "us", "at-cached", time.Now().Add(time.Hour))
	p := core.ProfileConfig{Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com"}
	tok, err := m.AccessTokenReadyForProfile(context.Background(), cp, p)
	if err != nil || tok != "at-cached" || calls != 0 {
		t.Fatalf("tok=%q calls=%d err=%v", tok, calls, err)
	}
}

func TestAccessTokenReadyForProfile_ThunderingHerd_OneExchange(t *testing.T) {
	testenv.IsolateConfigDir(t)

	// 过期 token + 5 并发 → 恰好 1 次 exchange（double-check 生效）
	calls := int32(0)
	srv := atomicCountingExchangeStub(t, &calls, "at-new")
	defer srv.Close()
	m := &Manager{Client: client.New(srv.URL)}
	cp := filepath.Join(t.TempDir(), "config.json")
	seedAccountUAT(t, "alice@co.com", "uat-1")
	seedProfileToken(t, AuthDir(cp), "us", "at-old", time.Now().Add(-time.Hour))
	p := core.ProfileConfig{Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com"}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.AccessTokenReadyForProfile(context.Background(), cp, p); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("exchange calls = %d, want 1", got)
	}
}

// CONC-02：profile 锁被占超时 → 降级直接 exchange（不死等、不报错）
func TestAccessTokenReadyForProfile_LockTimeoutDegrades(t *testing.T) {
	testenv.IsolateConfigDir(t)

	orig := profileLockTimeout
	profileLockTimeout = 200 * time.Millisecond
	defer func() { profileLockTimeout = orig }()

	calls := int32(0)
	srv := atomicCountingExchangeStub(t, &calls, "at-degraded")
	defer srv.Close()
	m := &Manager{Client: client.New(srv.URL)}
	cp := filepath.Join(t.TempDir(), "config.json")
	seedAccountUAT(t, "alice@co.com", "uat-1")
	// 预先占住 profile 锁不放
	hold, err := lockfile.Acquire(profileLockPath(cp, "us"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	p := core.ProfileConfig{Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com"}
	tok, err := m.AccessTokenReadyForProfile(context.Background(), cp, p)
	if err != nil || tok != "at-degraded" {
		t.Fatalf("degrade path: tok=%q err=%v", tok, err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("must exchange directly on lock timeout")
	}
}
