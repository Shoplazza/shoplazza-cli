package auth_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// The wizard's one hard invariant: with no terminal (pipes, CI, agents) `auth
// login` asks nothing, and its output, exit code and request set are what they
// were before. The command seam gives that for free — the factory's streams are
// buffers, not character devices, so the gate is closed.
//
// runLoginOffTTY also proves the second half of the invariant, that the command
// never blocks: a prompt reached here would sit waiting for a key forever, so
// exceeding the deadline is a failure rather than a slow test.
func runLoginOffTTY(t *testing.T, f *cmdutil.Factory, out *bytes.Buffer, args ...string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- execAuth(t, f, out, args...) }()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatalf("auth %v blocked without a terminal — the gate must never prompt", args)
		return nil
	}
}

// exitDetail returns the exit code, error type and hint carried by err.
func exitDetail(t *testing.T, err error) (int, string, string) {
	t.Helper()
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("error is not an *output.ExitError: %v", err)
	}
	if ee.Detail == nil {
		return ee.Code, "", ""
	}
	return ee.Code, ee.Detail.Type, ee.Detail.Hint
}

// webFlowServer mocks the endpoints a login hits — the browser flow, the UAT
// fast path's /me, and the store exchange — counting requests so a fail-fast
// case can assert it made none.
func webFlowServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1", "authorize_url": "https://example.com/authorize"})
		case "/api/saiga/cli/auth/sessions/sess1/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "uat": "uat_web", "account": "alice@example.com"})
		case "/api/saiga/cli/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"account": "alice@example.com", "user_id": "u-1"})
		case "/api/saiga/cli/auth/exchange/store-at":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
				"access_token": "at-1", "store_id": "100001", "store_domain": "my-store.myshoplaza.com",
				"granted_scopes": []string{"read_product"}, "at_expires_at": "2099-01-01T00:00:00Z",
			}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

// Every flag combination the two screens bind to goes straight through, exactly
// as before the wizard existed: no prompt, the same envelope. The screens are
// suppressed by the closed gate here, so this covers the combinations that
// would otherwise ask (bare run, -s alone answers only screen one) as well as
// the ones a terminal would also run unprompted.
func TestLogin_OffTTY_RunsWithoutPrompting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		flow string
	}{
		{"bare run", []string{"login"}, "web"},
		{"domain only", []string{"login", "--domain", "products"}, "web"},
		{"domain all", []string{"login", "--domain", "all"}, "web"},
		{"scope only", []string{"login", "--scope", "read_product"}, "web"},
		{"store and domain", []string{"login", "-s", "my-store.myshoplaza.com", "--domain", "products"}, "web"},
		{"store and scope", []string{"login", "-s", "my-store.myshoplaza.com", "--scope", "read_product"}, "web"},
		{"uat", []string{"login", "--uat", "uat_ci"}, "uat"},
		{"uat and store", []string{"login", "--uat", "uat_ci", "-s", "my-store.myshoplaza.com"}, "uat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := webFlowServer(t, &hits)
			defer srv.Close()

			f, out := tempAuthFactory(t, srv.URL)
			if err := runLoginOffTTY(t, f, out, tc.args...); err != nil {
				t.Fatalf("auth %v: %v", tc.args, err)
			}
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not the result envelope: %v\n%s", err, out.String())
			}
			if env["ok"] != true || env["action"] != "login" || env["flow"] != tc.flow {
				t.Errorf("envelope = %v, want the pre-wizard login envelope with flow %q", env, tc.flow)
			}
		})
	}
}

// Invalid values fail before anything else happens — no prompt, and no request
// either, so a typo can never come back as "authorized but missing scopes".
func TestLogin_OffTTY_InvalidValuesFailFast(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		wantHint string
	}{
		{"unknown domain", []string{"login", "--domain", "nosuchdomain"}, "--domain"},
		{"misspelled scope", []string{"login", "--scope", "read_prodcut"}, "auth scopes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := webFlowServer(t, &hits)
			defer srv.Close()

			f, out := tempAuthFactory(t, srv.URL)
			err := runLoginOffTTY(t, f, out, tc.args...)
			if err == nil {
				t.Fatalf("auth %v: want a validation error", tc.args)
			}
			code, typ, hint := exitDetail(t, err)
			if code != output.ExitValidation || typ != output.TypeValidation {
				t.Errorf("exit code = %d type = %q, want %d/validation", code, typ, output.ExitValidation)
			}
			if !strings.Contains(hint, tc.wantHint) {
				t.Errorf("hint = %q, want it to point at %q", hint, tc.wantHint)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("%d request(s) sent before validation failed, want 0", n)
			}
		})
	}
}

// A store login without permissions still errors here, pointing at the flags —
// the prompt that would answer it needs a terminal.
func TestLogin_OffTTY_StoreWithoutScopeStillErrors(t *testing.T) {
	var hits atomic.Int32
	srv := webFlowServer(t, &hits)
	defer srv.Close()

	f, out := tempAuthFactory(t, srv.URL)
	err := runLoginOffTTY(t, f, out, "login", "--store-domain", "my-store.com")
	if err == nil {
		t.Fatal("want a validation error for a store login with no scope")
	}
	code, typ, hint := exitDetail(t, err)
	if code != output.ExitValidation || typ != output.TypeValidation {
		t.Errorf("exit code = %d type = %q, want %d/validation", code, typ, output.ExitValidation)
	}
	if !strings.Contains(hint, "--scope") || !strings.Contains(hint, "--domain") {
		t.Errorf("hint = %q, want it to name --scope and --domain", hint)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("%d request(s) sent before validation failed, want 0", n)
	}
}
