package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cmdauth "shoplazza-cli-v2/cmd/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/testenv"
)

func tempAuthFactory(t *testing.T, srvURL string) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	t.Setenv("SHOPLAZZA_UAT", "")
	out := &bytes.Buffer{}
	f := &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: io.Discard},
		ConfigPath: filepath.Join(dir, "config.json"),
		Config:     core.CliConfig{},
		Client:     client.New(srvURL),
		AuthClient: client.New(srvURL),
	}
	return f, out
}

func execAuth(t *testing.T, f *cmdutil.Factory, out *bytes.Buffer, args ...string) error {
	t.Helper()
	cmd := cmdauth.NewCmdAuth(f)
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	return cmd.Execute()
}

func TestLogin_RejectsPositionalArg(t *testing.T) {
	f, out := tempAuthFactory(t, "http://unused")
	err := execAuth(t, f, out, "login", "my-store.com")
	if err == nil {
		t.Error("positional store-domain must be rejected (use --store-domain)")
	}
}

func TestLogin_NonInteractiveUAT_NoScopeRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/saiga/cli/auth/me" {
			json.NewEncoder(w).Encode(map[string]any{"account": "alice@example.com"})
			return
		}
		t.Errorf("unexpected path %s — UAT login must not create a session", r.URL.Path)
	}))
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	if err := execAuth(t, f, out, "login", "--uat", "uat_test"); err != nil {
		t.Fatalf("login --uat: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if env["uat"] != "uat_test" {
		t.Errorf("expected uat echoed, got %v", env["uat"])
	}
}

func TestStatus_FreshInstall_LoggedInFalse(t *testing.T) {
	f, out := tempAuthFactory(t, "http://unused")
	if err := execAuth(t, f, out, "status"); err != nil {
		t.Fatalf("status: %v", err)
	}
	var st map[string]any
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatalf("status output not JSON: %v\n%s", err, out.String())
	}
	if st["logged_in"] != false {
		t.Errorf("logged_in = %v, want false", st["logged_in"])
	}
	for _, removed := range []string{"refresh_available", "refresh_token_expires_at", "access_token_expires_at", "store_id"} {
		if _, ok := st[removed]; ok {
			t.Errorf("status must not emit removed key %q", removed)
		}
	}
}

// storeATServer mocks the saiga endpoints a --uat store login hits: /me for the
// account, /exchange/store-at for the store token.
func storeATServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"account": "a@x.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at_x", "store_domain": "my-store.com", "at_expires_at": "2099-01-01T00:00:00Z"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

// Interactive store login (no --uat) must request scopes.
func TestLogin_StoreDomainRequiresScope(t *testing.T) {
	f, _ := tempAuthFactory(t, "http://unused")
	typ, err := execAuthErrType(t, f, "login", "--store-domain", "my-store.com")
	if err == nil || typ != output.TypeValidation {
		t.Errorf("expected type=validation when --store-domain set without scope, got type=%q err=%v", typ, err)
	}
}

// --uat store login is exempt: the store token inherits the UAT's account scopes.
func TestLogin_StoreDomainWithUAT_NoScopeOK(t *testing.T) {
	srv := storeATServer(t)
	defer srv.Close()
	f, out := tempAuthFactory(t, srv.URL)
	if err := execAuth(t, f, out, "login", "--store-domain", "my-store.com", "--uat", "uat_x"); err != nil {
		t.Fatalf("--uat store login should be exempt from the scope requirement: %v", err)
	}
}

// SHOPLAZZA_UAT env (the non-flag form of --uat) is exempt too.
func TestLogin_StoreDomainWithEnvUAT_NoScopeOK(t *testing.T) {
	srv := storeATServer(t)
	defer srv.Close()
	f, out := tempAuthFactory(t, srv.URL)
	t.Setenv("SHOPLAZZA_UAT", "uat_env") // tempAuthFactory cleared it; override after.
	if err := execAuth(t, f, out, "login", "--store-domain", "my-store.com"); err != nil {
		t.Fatalf("env-UAT store login should be exempt from the scope requirement: %v", err)
	}
}
