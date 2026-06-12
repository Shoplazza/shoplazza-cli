package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
)

// setupTempConfigInternal redirects auth/config/keychain paths to a temp dir
// for internal-package tests.
func setupTempConfigInternal(t *testing.T) (configPath, authPath string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("HOME", dir)
	configPath = filepath.Join(dir, "config.json")
	authPath = filepath.Join(dir, "auth.json")
	return configPath, authPath
}

func TestAppTokenReady_MintsAndCaches(t *testing.T) {
	cfgPath, authPath := setupTempConfigInternal(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "app_at_1", "client_id": "cid_1", "at_expires_at": "2030-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	m := NewManager(core.CliConfig{}, cfgPath, client.New(srv.URL))
	m.AuthPath = authPath
	if err := m.persistState(AuthState{UAT: "uat_1", Account: "a@x.com"}); err != nil {
		t.Fatal(err)
	}

	tok, err := m.AppTokenReady(context.Background(), "cid_1", "secret_1", "partner_1")
	if err != nil || tok != "app_at_1" {
		t.Fatalf("AppTokenReady = %q, %v", tok, err)
	}
	if _, err := m.AppTokenReady(context.Background(), "cid_1", "secret_1", "partner_1"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 saiga call (cached on 2nd), got %d", calls)
	}
}

func TestAppTokenReady_NoUAT(t *testing.T) {
	cfgPath, authPath := setupTempConfigInternal(t)
	m := NewManager(core.CliConfig{}, cfgPath, client.New("http://unused"))
	m.AuthPath = authPath
	if _, err := m.AppTokenReady(context.Background(), "cid_1", "s", "p"); err == nil {
		t.Fatal("expected error when no UAT")
	}
}
