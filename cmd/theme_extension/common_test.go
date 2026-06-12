package theme_extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/output"
	te "shoplazza-cli-v2/internal/theme_extension"
)

func TestResolveStore(t *testing.T) {
	// override wins
	if s, err := resolveStore(&cmdutil.Factory{Config: core.CliConfig{}}, "ovr.myshoplaza.com"); err != nil || s != "ovr.myshoplaza.com" {
		t.Fatalf("override: %q %v", s, err)
	}
	// current store fallback
	f := &cmdutil.Factory{Config: core.CliConfig{StoreDomain: "cur.myshoplaza.com"}}
	if s, err := resolveStore(f, ""); err != nil || s != "cur.myshoplaza.com" {
		t.Fatalf("current: %q %v", s, err)
	}
	// both empty → validation
	_, err := resolveStore(&cmdutil.Factory{Config: core.CliConfig{}}, "")
	if err == nil || err.Detail == nil || err.Detail.Type != output.TypeValidation {
		t.Fatalf("expected type=validation error when both empty, got %v", err)
	}
	// scheme-prefixed domains normalize (a raw https:// prefix used to yield a
	// "https://https://x" base URL downstream)
	if s, err := resolveStore(&cmdutil.Factory{Config: core.CliConfig{}}, "https://ovr.myshoplaza.com/"); err != nil || s != "ovr.myshoplaza.com" {
		t.Fatalf("scheme override: %q %v", s, err)
	}
	f = &cmdutil.Factory{Config: core.CliConfig{StoreDomain: "HTTP://cur.myshoplaza.com"}}
	if s, err := resolveStore(f, ""); err != nil || s != "cur.myshoplaza.com" {
		t.Fatalf("scheme current: %q %v", s, err)
	}
	// a flag that normalizes to nothing must not slip through as "no override"
	if _, err := resolveStore(&cmdutil.Factory{Config: core.CliConfig{}}, "https://"); err == nil || err.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation for useless override, got %v", err)
	}
}

func TestConnectRequiresExtensionID(t *testing.T) {
	root := t.TempDir() // no shoplazza.extension.toml
	f := &cmdutil.Factory{}
	cmd := newCmdConnect(f)
	cmd.SetArgs([]string{"--client-id", "cid_1", "--path", root})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error for missing extension_id, got %v", err)
	}
}

func TestReleaseRequiresConnectFirst(t *testing.T) {
	root := t.TempDir()
	// has extension_id but no client_id (never connected)
	if err := te.WriteConfig(root, te.Config{ExtensionID: "tex_1", Name: "x"}); err != nil {
		t.Fatal(err)
	}
	f := &cmdutil.Factory{}
	cmd := newCmdRelease(f)
	cmd.SetArgs([]string{"--version", "1.0.0", "--path", root})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation (connect first), got %v", err)
	}
}

// ── selectPartner ─────────────────────────────────────────────────────────────

func mustParsePartners(t *testing.T, raw string) []app.Partner {
	t.Helper()
	var p []app.Partner
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("parse partners: %v", err)
	}
	return p
}

func TestSelectPartner_FlagMatchesPartner(t *testing.T) {
	partners := mustParsePartners(t, `[{"id":"p1"},{"id":"p2"}]`)
	got, err := selectPartner(partners, "p1")
	if err != nil || got != "p1" {
		t.Errorf("got (%q, %v) want (p1, nil)", got, err)
	}
}

func TestSelectPartner_FlagNotFound(t *testing.T) {
	partners := mustParsePartners(t, `[{"id":"p1"}]`)
	_, err := selectPartner(partners, "unknown")
	if err == nil {
		t.Error("expected error when flag partner not found")
	}
}

func TestSelectPartner_NoPartners(t *testing.T) {
	_, err := selectPartner(nil, "")
	if err == nil {
		t.Error("expected error when no partners available")
	}
}

func TestSelectPartner_SingleAutoSelected(t *testing.T) {
	partners := mustParsePartners(t, `[{"id":"only-one"}]`)
	got, err := selectPartner(partners, "")
	if err != nil || got != "only-one" {
		t.Errorf("single partner auto-select: got (%q, %v) want (only-one, nil)", got, err)
	}
}

func TestSelectPartner_MultipleWithoutFlag(t *testing.T) {
	partners := mustParsePartners(t, `[{"id":"p1"},{"id":"p2"}]`)
	_, err := selectPartner(partners, "")
	if err == nil {
		t.Error("expected error when multiple partners and no flag")
	}
}

// ── apiError ─────────────────────────────────────────────────────────────────

// dialError fabricates the url.Error-wrapped *net.OpError a refused dial
// produces — the exact shape the client surfaces for transport failures.
func dialError() error {
	return &url.Error{Op: "Post", URL: "https://s.myshoplaza.com/x",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}}
}

// TestAPIError_Classification pins the full transport-error mapping: HTTP →
// api (endpoint attached; 403 reclassified to auth inside ErrAPI), wire
// failure → network, anything else → internal.
func TestAPIError_Classification(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"422 HTTP → api", &client.HTTPError{StatusCode: 422, Body: `{"error":"bad"}`, Method: "POST", Path: "/x"}, output.ExitAPI},
		{"403 HTTP → auth", &client.HTTPError{StatusCode: 403, Body: `{"message":"forbidden"}`, Method: "GET", Path: "/x"}, output.ExitAuth},
		{"refused dial → network", dialError(), output.ExitNetwork},
		{"generic → internal", errors.New("boom"), output.ExitInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := apiError(tc.err)
			if got == nil || got.Code != tc.wantCode {
				t.Fatalf("apiError(%v) = %v, want exit %d", tc.err, got, tc.wantCode)
			}
		})
	}
}

// TestAPIError_NamesEndpoint: API-class errors must carry the failing
// method+path.
func TestAPIError_NamesEndpoint(t *testing.T) {
	he := &client.HTTPError{StatusCode: 422, Body: `{"error":"bad"}`, Method: "POST", Path: "/openapi/x"}
	got := apiError(he)
	if got.Detail == nil || got.Detail.Detail == nil ||
		got.Detail.Detail.Method != "POST" || got.Detail.Detail.Path != "/openapi/x" {
		t.Fatalf("expected endpoint POST /openapi/x in detail, got %+v", got.Detail)
	}
}

// ── storeTokenError ──────────────────────────────────────────────────────────

func TestStoreTokenError_Classification(t *testing.T) {
	// non-2xx exchange → auth-class with the server message + a re-login hint
	httpErr := storeTokenError(&client.HTTPError{StatusCode: 401, Body: `{"message":"uat expired"}`})
	if httpErr.Code != output.ExitAuth {
		t.Fatalf("HTTP mint failure: exit %d, want auth", httpErr.Code)
	}
	if httpErr.Detail == nil || !strings.Contains(httpErr.Detail.Message, "uat expired") || httpErr.Detail.Hint == "" {
		t.Fatalf("HTTP mint failure should keep server message + hint, got %+v", httpErr.Detail)
	}
	// wire failure → network (exit 3 would misdirect the user to re-login)
	if got := storeTokenError(dialError()); got.Code != output.ExitNetwork {
		t.Fatalf("dial failure: exit %d, want network", got.Code)
	}
	// anything else → plain auth
	if got := storeTokenError(errors.New("no UAT available")); got.Code != output.ExitAuth {
		t.Fatalf("generic failure: exit %d, want auth", got.Code)
	}
}

// ── SHOPLAZZA_ACCESS_TOKEN bypass ────────────────────────────────────────────

func TestRequireLogin_EnvTokenBypass(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	if err := requireLogin(context.Background(), &cmdutil.Factory{}); err != nil {
		t.Fatalf("requireLogin must pass with SHOPLAZZA_ACCESS_TOKEN set, got %v", err)
	}
}

func TestStoreClient_EnvTokenBypass(t *testing.T) {
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", "")
	c, err := storeClient(context.Background(), &cmdutil.Factory{}, "shop.myshoplaza.com")
	if err != nil {
		t.Fatalf("storeClient with env token: %v", err)
	}
	if c.BaseURL != "https://shop.myshoplaza.com" {
		t.Fatalf("base URL = %q, want https://shop.myshoplaza.com", c.BaseURL)
	}
	// explicit API base overrides the store-domain default (factory parity)
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", "http://127.0.0.1:9999")
	c, err = storeClient(context.Background(), &cmdutil.Factory{}, "shop.myshoplaza.com")
	if err != nil {
		t.Fatalf("storeClient with env base: %v", err)
	}
	if c.BaseURL != "http://127.0.0.1:9999" {
		t.Fatalf("base URL = %q, want the env override", c.BaseURL)
	}
}

// ── printServeBanner ──────────────────────────────────────────────────────────

func TestPrintServeBanner_WithThemeID(t *testing.T) {
	var buf bytes.Buffer
	printServeBanner(&buf, "shop.example.com", "tex_abc", "theme_123")
	out := buf.String()
	if !strings.Contains(out, "shop.example.com") {
		t.Errorf("expected domain in output, got: %q", out)
	}
	if !strings.Contains(out, "theme_123") {
		t.Errorf("expected theme_id in output, got: %q", out)
	}
	if !strings.Contains(out, "tex_abc") {
		t.Errorf("expected extension_id in output, got: %q", out)
	}
}
