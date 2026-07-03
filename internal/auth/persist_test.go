package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

func TestLogin_SaveConfigError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "store_domain": "f.com"})
		}
	}))
	defer srv.Close()
	_, authPath := setupTempConfig(t)
	// Force SaveConfig to fail in a way root CANNOT bypass: place config.json UNDER a
	// regular file, so SaveConfig's MkdirAll(filepath.Dir) returns ENOTDIR (a path
	// component is not a directory). A chmod/permission-based failure wouldn't trigger
	// for root (root ignores file modes), which is why this avoids that approach.
	notADir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := internalauth.NewManager(core.CliConfig{}, filepath.Join(notADir, "config.json"), client.New(srv.URL))
	mgr.AuthPath = authPath
	// store-domain login sets CurrentStore → persistState calls SaveConfig → MkdirAll fails (ENOTDIR).
	if _, err := mgr.Login(context.Background(), "f.com", nil, "uat_cfg", 5*time.Second, time.Millisecond, nil); err == nil {
		t.Error("expected SaveConfig error")
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

var testSafeNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// keychainFile mirrors internal/keychain layout:
// <UserConfigDir>/shoplazza-cli/keychain/<service>_<safeName>.enc
// where safeName replaces every char outside [a-zA-Z0-9._-] with "_".
func keychainFile(t *testing.T, account string) string {
	t.Helper()
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	safe := testSafeNameRe.ReplaceAllString(account, "_")
	return filepath.Join(cfgDir, "shoplazza-cli", "keychain", "shoplazza-cli_"+safe+".enc")
}

func TestKeychainKeyNaming(t *testing.T) {
	testenv.IsolateConfigDir(t)

	cases := map[string]string{
		"uat":                keychainFile(t, "uat"),
		"partner":            keychainFile(t, "partner"),
		"store:my-store.com": keychainFile(t, "store:my-store.com"),
		"app:cid_123":        keychainFile(t, "app:cid_123"),
	}
	for account := range cases {
		if err := keychain.Set(keychain.ShoplazzaCliService, account, "tok_"+account); err != nil {
			t.Fatalf("Set(%q): %v", account, err)
		}
	}
	seen := map[string]bool{}
	for account, want := range cases {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected file for %q at %s: %v", account, want, err)
		}
		if seen[want] {
			t.Errorf("file collision for %q at %s", account, want)
		}
		seen[want] = true
		// Account-level keys must NOT carry a suffix; resource-level keys must.
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
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_x"); err != nil {
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
