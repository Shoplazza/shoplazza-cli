package theme_extension

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdtest"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
	te "github.com/Shoplazza/shoplazza-cli/v2/internal/theme_extension"
)

func TestResolveStore(t *testing.T) {
	// override wins
	if s, err := resolveStore(&cmdutil.Factory{Config: core.CliConfig{}}, "ovr.myshoplaza.com"); err != nil || s != "ovr.myshoplaza.com" {
		t.Fatalf("override: %q %v", s, err)
	}
	// current store fallback
	f := &cmdutil.Factory{Config: currentStoreConfig("cur.myshoplaza.com")}
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
	f = &cmdutil.Factory{Config: currentStoreConfig("HTTP://cur.myshoplaza.com")}
	if s, err := resolveStore(f, ""); err != nil || s != "cur.myshoplaza.com" {
		t.Fatalf("scheme current: %q %v", s, err)
	}
	// a flag that normalizes to nothing must not slip through as "no override"
	if _, err := resolveStore(&cmdutil.Factory{Config: core.CliConfig{}}, "https://"); err == nil || err.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation for useless override, got %v", err)
	}
}

// currentStoreConfig builds a v2 CliConfig whose CurrentStoreDomain() resolves to domain.
func currentStoreConfig(domain string) core.CliConfig {
	return core.CliConfig{
		CurrentProfile: "p",
		Profiles:       []core.ProfileConfig{{Name: "p", StoreDomain: domain}},
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
	c, domain, err := storeClient(context.Background(), &cmdutil.Factory{}, "shop.myshoplaza.com")
	if err != nil {
		t.Fatalf("storeClient with env token: %v", err)
	}
	if domain != "shop.myshoplaza.com" {
		t.Fatalf("domain = %q, want shop.myshoplaza.com", domain)
	}
	if c.BaseURL != "https://shop.myshoplaza.com" {
		t.Fatalf("base URL = %q, want https://shop.myshoplaza.com", c.BaseURL)
	}
	// explicit API base overrides the store-domain default (factory parity)
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", "http://127.0.0.1:9999")
	c, _, err = storeClient(context.Background(), &cmdutil.Factory{}, "shop.myshoplaza.com")
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

// runTECmd executes the te command tree with args against f, discarding
// output. The ad-hoc tests only assert post-command config/keychain state —
// the mocked exchange server stands in for the auth exchange only; there is
// no real store behind the fake domain, so the command's own exit status
// (the final store-openapi call against the fake domain) is irrelevant here
// and deliberately ignored. A short context deadline keeps that doomed
// network hop from ever stalling the test.
func runTECmd(t *testing.T, f *cmdutil.Factory, args ...string) {
	t.Helper()
	cmd := NewCmdThemeExtension(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd.SetContext(ctx)
	_ = cmd.Execute()
}

// TestTE_AdhocDomain_NoPersistence: `te list -s cn.myshoplazza.com` with only
// a "us" profile on file must mint via ExchangeEphemeral — no "cn" profile is
// created and no token is persisted under its keychain slot (tech design
// §4.2, zero residue for an ad-hoc domain).
func TestTE_AdhocDomain_NoPersistence(t *testing.T) {
	t.Setenv(envAccessToken, "") // force the profile/ephemeral path, not the env bypass
	srv := testenv.NewStoreATExchangeStub(t, "at-tmp")
	defer srv.Close()
	f := cmdtest.SeedLoggedInWithProfiles(t, "alice@co.com", "us") // only us
	f.AuthClient = client.New(srv.URL)

	runTECmd(t, f, "list", "-s", "cn.myshoplazza.com")

	cfg, err := core.LoadConfig(f.ConfigPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("ad-hoc must not create a profile, got %d profiles: %+v", len(cfg.Profiles), cfg.Profiles)
	}
	if v, gErr := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("cn")); gErr != nil || v != "" {
		t.Fatalf("ad-hoc must not persist a token, got v=%q err=%v", v, gErr)
	}
}

// TestTE_DomainMatchesProfile_UsesProfileCreds: `-s us.myshoplazza.com`
// matching the existing "us" profile must go through
// AccessTokenReadyForProfile — the exchange stub's token ends up cached under
// the profile's own keychain slot (proving the mint+persist path ran, not the
// ephemeral one).
func TestTE_DomainMatchesProfile_UsesProfileCreds(t *testing.T) {
	t.Setenv(envAccessToken, "") // force the profile/ephemeral path, not the env bypass
	srv := testenv.NewStoreATExchangeStub(t, "at-us")
	defer srv.Close()
	f := cmdtest.SeedLoggedInWithProfiles(t, "alice@co.com", "us")
	f.AuthClient = client.New(srv.URL)

	runTECmd(t, f, "list", "-s", "us.myshoplazza.com")

	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("us")); err != nil || v != "at-us" {
		t.Fatalf("expected profile us token minted+persisted as at-us, got v=%q err=%v", v, err)
	}
}
