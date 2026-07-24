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

	cmdauth "github.com/Shoplazza/shoplazza-cli/v2/cmd/auth"
	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
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
	f.Config.Accounts = []core.AccountConfig{{Name: "alice@example.com"}}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@example.com"), "uat_seed"); err != nil {
		t.Fatal(err)
	}
	if err := execAuth(t, f, out, "store", "use", "--store-domain", "my-store.com"); err != nil {
		t.Fatalf("store use: %v", err)
	}
	var env map[string]any
	json.Unmarshal(out.Bytes(), &env)
	if env["profile"] != "my-store.com" || env["store_domain"] != "my-store.com" {
		t.Errorf("profile/store_domain = %v/%v", env["profile"], env["store_domain"])
	}
	if env["token_status"] != "valid" {
		t.Errorf("token_status = %v, want valid (eager mint)", env["token_status"])
	}

	// Verify the current store was persisted to config.json (not just returned).
	cfgData, err := os.ReadFile(f.ConfigPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if !strings.Contains(string(cfgData), `"storeDomain": "my-store.com"`) {
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
	f.Config.Accounts = []core.AccountConfig{{Name: "alice@example.com"}}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@example.com"), "uat_seed"); err != nil {
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
	f.Config.Accounts = []core.AccountConfig{{Name: "alice@example.com"}}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@example.com"), "uat_seed"); err != nil {
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

// The scope check runs AFTER the store-token exchange (UseStore), against the
// fresh per-store grant — the exchange succeeds but its granted set doesn't
// cover the requested scope, so the command rejects. No v2 profile is created
// or activated (asserted below). The legacy cfg.StoreDomain write has been
// removed, so a rejected request no longer changes the current-store context.
func TestStoreUse_ScopeNotGranted_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/saiga/cli/auth/exchange/store-at" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at_use", "store_domain": "my-store.com",
				"at_expires_at":  "2099-01-01T00:00:00Z",
				"granted_scopes": []string{"read_product"},
			})
		}
	}))
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	f.Config.Accounts = []core.AccountConfig{{Name: "alice@example.com"}}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@example.com"), "uat_seed"); err != nil {
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

	// The post-check must still block the v2 profile sync before it upserts anything.
	cfg, cErr := core.LoadConfig(f.ConfigPath)
	if cErr == nil && (len(cfg.Profiles) != 0 || cfg.CurrentProfile != "") {
		t.Errorf("no profile should be created/activated for an out-of-grant scope request, got profiles=%+v current=%q",
			cfg.Profiles, cfg.CurrentProfile)
	}

	// The freshly-minted token/meta must be cleaned up too — no orphans.
	if tok, kerr := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("my-store.com")); kerr == nil && tok != "" {
		t.Error("orphan keychain token left behind after rejected store use")
	}
	meta, _ := internalauth.LoadProfileMeta(internalauth.AuthDir(f.ConfigPath), "my-store.com")
	if meta.ExpiresAt != "" {
		t.Error("orphan profile meta left behind after rejected store use")
	}
}

// Regression for the bug fix-pass-2 introduced: after an account-only login
// (no --store-domain), f.Config.Account().GrantedScopes is empty, so a
// pre-exchange check against it would reject every --scope. The post-exchange
// check against newStatus.GrantedScopes must succeed instead.
func TestStoreUse_AfterAccountOnlyLogin_ScopeSubset_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "alice@example.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at_use", "store_domain": "my-store.com",
				"at_expires_at":  "2099-01-01T00:00:00Z",
				"granted_scopes": []string{"read_product", "write_product"},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	// Account-only login: no --store-domain, so GrantedScopes stays empty.
	if err := execAuth(t, f, out, "login", "--uat", "uat_test", "--scope", "read_product"); err != nil {
		t.Fatalf("account-only login: %v", err)
	}

	out.Reset()
	if err := execAuth(t, f, out, "store", "use", "--store-domain", "my-store.com", "--scope", "read_product"); err != nil {
		t.Fatalf("store use --scope after account-only login should succeed: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if env["store_domain"] != "my-store.com" {
		t.Errorf("store_domain = %v", env["store_domain"])
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
