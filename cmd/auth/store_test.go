package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	cmdauth "shoplazza-cli-v2/cmd/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/output"
)

func execAuthErrType(t *testing.T, f *cmdutil.Factory, args ...string) (string, error) {
	t.Helper()
	cmd := cmdauth.NewCmdAuth(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	if err == nil {
		return "", nil
	}
	var ee *output.ExitError
	if errors.As(err, &ee) && ee.Detail != nil {
		return ee.Detail.Type, err
	}
	return "", err
}

func TestStoreUse_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_use", "store_domain": "my-store.com", "at_expires_at": "2099-01-01T00:00:00Z"})
		}
	}))
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatal(err)
	}
	if err := execAuth(t, f, out, "store", "use", "--store-domain", "my-store.com"); err != nil {
		t.Fatalf("store use: %v", err)
	}
	var env map[string]any
	json.Unmarshal(out.Bytes(), &env)
	status, _ := env["status"].(map[string]any)
	if status["current_store"] != "my-store.com" {
		t.Errorf("status.current_store = %v", status["current_store"])
	}

	// Verify the current store was persisted to config.json (not just returned).
	cfgData, err := os.ReadFile(f.ConfigPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if !strings.Contains(string(cfgData), `"store_domain": "my-store.com"`) {
		t.Errorf("config.json should persist current store; got: %s", cfgData)
	}
}

func TestStoreUse_ScopesRequired422(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			io.WriteString(w, `{"errors":["Scopes is required"]}`)
		}
	}))
	defer srv.Close()

	f, _ := tempAuthFactory(t, srv.URL)
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatal(err)
	}

	cmd := cmdauth.NewCmdAuth(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"store", "use", "--store-domain", "my-store.com"})
	cmd.SetContext(context.Background())
	err := cmd.Execute()

	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil {
		t.Fatalf("expected an ExitError, got %v", err)
	}
	if ee.Detail.Message != "Scopes is required" {
		t.Errorf("message = %q, want clean 'Scopes is required'", ee.Detail.Message)
	}
	// A scope/permission failure on the store-token exchange is auth-class.
	if ee.Detail.Type != output.TypeAuth {
		t.Errorf("type = %q, want auth", ee.Detail.Type)
	}
	// It carries a re-auth hint pointing at 'login' with scopes (the only way to
	// mint a scoped store token), interpolating the requested store domain.
	if !strings.Contains(ee.Detail.Hint, "shoplazza auth login -s my-store.com --scope") {
		t.Errorf("expected re-auth hint with store domain, got %q", ee.Detail.Hint)
	}
}

func TestStoreUse_StoreNotFound404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"session_not_found","errors":["store not found: my-store.com"]}`)
		}
	}))
	defer srv.Close()

	f, _ := tempAuthFactory(t, srv.URL)
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatal(err)
	}

	cmd := cmdauth.NewCmdAuth(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"store", "use", "--store-domain", "my-store.com"})
	cmd.SetContext(context.Background())
	err := cmd.Execute()

	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil {
		t.Fatalf("expected an ExitError, got %v", err)
	}
	// A not-found store is still auth-class, with the clean server message...
	if ee.Detail.Type != output.TypeAuth {
		t.Errorf("type = %q, want auth", ee.Detail.Type)
	}
	if ee.Detail.Message != "store not found: my-store.com" {
		t.Errorf("message = %q, want clean not-found message", ee.Detail.Message)
	}
	// ...but no scope hint: re-authorizing can't fix a wrong store domain.
	if ee.Detail.Hint != "" {
		t.Errorf("expected no hint for store-not-found, got %q", ee.Detail.Hint)
	}
}

// The scope check must run BEFORE the store-token exchange (UseStore), so a
// rejected --scope never mints/persists a v1 store token or touches the v1
// cfg.StoreDomain — this test also seeds a real server handler for the
// exchange endpoint and asserts it is never hit.
func TestStoreUse_ScopeNotGranted_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			t.Errorf("store-at exchange must not be called when --scope is rejected pre-exchange")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_use", "store_domain": "my-store.com",
			"at_expires_at":  "2099-01-01T00:00:00Z",
			"granted_scopes": []string{"read_product"},
		})
	}))
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatal(err)
	}
	// Seed a logged-in account with a known granted-scope set so the pre-check
	// (validated against f.Config.Account().GrantedScopes) has something to reject.
	f.Config.Accounts = []core.AccountConfig{{Name: "alice@co.com", GrantedScopes: []string{"read_product"}}}
	if err := core.SaveConfig(f.ConfigPath, f.Config); err != nil {
		t.Fatal(err)
	}

	err := execAuth(t, f, out, "store", "use", "--store-domain", "my-store.com", "--scope", "write_product")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil {
		t.Fatalf("expected an ExitError for an out-of-grant scope, got %v", err)
	}
	if ee.Detail.Type != output.TypeValidation {
		t.Errorf("type = %q, want validation", ee.Detail.Type)
	}

	// The scope-gate must block the v2 profile sync before it upserts anything.
	cfg, cErr := core.LoadConfig(f.ConfigPath)
	if cErr == nil && (len(cfg.Profiles) != 0 || cfg.CurrentProfile != "") {
		t.Errorf("no profile should be created/activated for an out-of-grant scope request, got profiles=%+v current=%q",
			cfg.Profiles, cfg.CurrentProfile)
	}
	// The v1 side effects (persistState) must not have run either: the
	// rejected store must not become the legacy current store...
	if cErr == nil && cfg.StoreDomain != "" {
		t.Errorf("v1 cfg.StoreDomain must stay empty for a rejected store, got %q", cfg.StoreDomain)
	}
	// ...and no v1 store token ("store:<domain>", see internal/auth/persist.go
	// storeKcKey) should have been cached in keychain.
	if tok, _ := keychain.Get(keychain.ShoplazzaCliService, "store:my-store.com"); tok != "" {
		t.Errorf("v1 store token must not be minted for a rejected --scope, got %q", tok)
	}
}

func TestStoreUse_NotLoggedIn(t *testing.T) {
	f, _ := tempAuthFactory(t, "http://unused")
	typ, err := execAuthErrType(t, f, "store", "use", "--store-domain", "my-store.com")
	if err == nil || typ != output.TypeAuth {
		t.Errorf("expected type=auth, got type=%q err=%v", typ, err)
	}
}

func TestStoreUse_MissingFlag(t *testing.T) {
	f, _ := tempAuthFactory(t, "http://unused")
	typ, err := execAuthErrType(t, f, "store", "use")
	if err == nil || typ != output.TypeValidation {
		t.Errorf("expected type=validation, got type=%q err=%v", typ, err)
	}
}
