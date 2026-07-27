package profile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
)

// newExchangeStub returns an httptest server that stubs the store-AT exchange
// endpoint, always returning accessToken with a fixed store id/scope/expiry,
// wrapped in the real {"code":"Success","data":{...}} envelope the client
// unwraps.
func newExchangeStub(t *testing.T, accessToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"access_token": accessToken, "store_id": "100001",
			"store_domain": "us.myshoplazza.com", "granted_scopes": []string{"read_product"},
			"at_expires_at": "2099-01-01T00:00:00Z",
		}})
	}))
}

// newFailingExchangeStub returns an httptest server whose exchange endpoint
// always fails with the given status/raw body (CMD-02: store not found).
func newFailingExchangeStub(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestProfileAdd_HappyPath(t *testing.T) {
	srv := newExchangeStub(t, "at-1")
	defer srv.Close()
	f := newTestFactory(t, srv.URL)

	out := runCmd(t, f, "add", "--name", "us", "--store-domain", "us.myshoplazza.com")

	cfg, err := core.LoadConfig(f.ConfigPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	p := cfg.FindProfile("us")
	if p == nil || p.Account != "alice@co.com" || cfg.CurrentProfile != "us" {
		t.Fatalf("cfg: %+v out: %s", cfg, out) // first profile auto-becomes current
	}
	if p.StoreID != "100001" {
		t.Fatalf("StoreID not backfilled from meta: %+v", p)
	}
}

func TestProfileAdd_ExchangeFails_ZeroResidue(t *testing.T) {
	srv := newFailingExchangeStub(t, 404, "store not found") // CMD-02
	defer srv.Close()
	f := newTestFactory(t, srv.URL)

	err := runCmdErr(t, f, "add", "--name", "ghost", "--store-domain", "nope.myshoplazza.com")
	if !strings.Contains(err.Error(), "store not found") {
		t.Fatalf("error = %v, want to mention 'store not found'", err)
	}

	cfg, _ := core.LoadConfig(f.ConfigPath)
	if len(cfg.Profiles) != 0 {
		t.Fatalf("config must have zero residue, got %+v", cfg.Profiles)
	}
}

func TestProfileAdd_Validation(t *testing.T) {
	f := newTestFactory(t, "")
	// Pre-seed "us" in-memory (no exchange involved) so the second case can
	// collide on "US" — add's account/duplicate checks read f.Config directly.
	f.Config.Profiles = append(f.Config.Profiles, core.ProfileConfig{
		Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com",
	})

	for _, tc := range []struct {
		args    []string
		wantMsg string
	}{
		{[]string{"add", "--name", "con", "--store-domain", "x.myshoplazza.com"}, "reserved"},
		{[]string{"add", "--name", "US", "--store-domain", "y.myshoplazza.com"}, "already exists"},                     // 先建 us 再撞 US
		{[]string{"add", "--name", "z", "--store-domain", "z.myshoplazza.com", "--scope", "write_all"}, "not granted"}, // ⊄ 全集
	} {
		err := runCmdErr(t, f, tc.args...)
		if !strings.Contains(err.Error(), tc.wantMsg) {
			t.Errorf("args=%v: error = %q, want to contain %q", tc.args, err.Error(), tc.wantMsg)
		}
	}
}
