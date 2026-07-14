package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/testenv"
)

func meOnlyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
	}))
}

func TestLoadState_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	configPath, authPath := setupTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, []byte(`{invalid`), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := internalauth.NewManager(core.CliConfig{}, configPath, client.New(srv.URL))
	mgr.AuthPath = authPath
	if _, err := mgr.LoadState(); err == nil {
		t.Error("expected error on invalid auth.json")
	}
}

func TestLogin_DefaultAuthPath_UATInjection(t *testing.T) {
	srv := meOnlyServer(t)
	defer srv.Close()
	configPath, _ := setupTempConfig(t)
	mgr := internalauth.NewManager(core.CliConfig{}, configPath, client.New(srv.URL)) // AuthPath unset → defaultAuthMetaPath
	res, err := mgr.Login(context.Background(), "", nil, "uat_dp", 5*time.Second, time.Millisecond, nil)
	if err != nil || !res.Status.LoggedIn {
		t.Fatalf("Login default path: res=%+v err=%v", res, err)
	}
}

func TestLogout_AlreadyLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	mgr := newTestManager(t, srv)
	if _, err := mgr.Logout(); err != nil {
		t.Fatalf("Logout on empty state: %v", err)
	}
}

// TestKeychainKeyNaming asserts the auth key conventions behaviorally instead
// of mirroring keychain's private sanitization regex: distinct account keys
// must land in distinct files, and each must round-trip through Get.
func TestKeychainKeyNaming(t *testing.T) {
	testenv.IsolateConfigDir(t)

	accounts := []string{"uat", "partner", "store:my-store.com", "app:cid_123"}
	for _, account := range accounts {
		if err := keychain.Set(keychain.ShoplazzaCliService, account, "tok_"+account); err != nil {
			t.Fatalf("Set(%q): %v", account, err)
		}
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(cfgDir, "shoplazza-cli", "keychain"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(accounts) {
		t.Errorf("want %d distinct keychain files, got %d", len(accounts), len(entries))
	}
	for _, account := range accounts {
		got, err := keychain.Get(keychain.ShoplazzaCliService, account)
		if err != nil || got != "tok_"+account {
			t.Errorf("Get(%q) = %q, %v; want tok_%s", account, got, err, account)
		}
	}
}

// The app token slot has no acquisition command in this change. This test
// verifies the key convention round-trips, that LoadState reads the slot, and
// that Logout removes it (driven by the auth.json apps map).
func TestAppSlot_RoundTripAndLogoutCleanup(t *testing.T) {
	configPath, authPath := setupTempConfig(t)

	// Seed auth.json with an app-slot entry + the keychain tokens a future
	// select-app command would write.
	authJSON := `{"account":"a@x.com","apps":{"cid_42":{"expires_at":"2099-01-01T00:00:00Z"}}}`
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, []byte(authJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("a@x.com"), "uat_x"); err != nil {
		t.Fatal(err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, "app:cid_42", "app_tok_secret"); err != nil {
		t.Fatal(err)
	}

	// Key round-trip.
	got, err := keychain.Get(keychain.ShoplazzaCliService, "app:cid_42")
	if err != nil || got != "app_tok_secret" {
		t.Fatalf("app slot round-trip: %q %v", got, err)
	}

	mgr := internalauth.NewManager(core.CliConfig{}, configPath, client.New("http://unused"))
	mgr.AuthPath = authPath

	// LoadState surfaces the app slot (metadata from auth.json, token from keychain).
	st, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.Apps["cid_42"].Token != "app_tok_secret" {
		t.Errorf("LoadState app token = %q, want app_tok_secret", st.Apps["cid_42"].Token)
	}

	// Logout removes the app slot keychain entry.
	if _, err := mgr.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	after, _ := keychain.Get(keychain.ShoplazzaCliService, "app:cid_42")
	if after != "" {
		t.Errorf("app slot keychain entry must be removed on logout, got %q", after)
	}
}
