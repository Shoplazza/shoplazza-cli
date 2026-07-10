package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/app/project"
	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/testenv"
)

func TestRequireLogin_NotLoggedIn(t *testing.T) {
	// Isolate config + keychain to a temp dir so the result does NOT depend on
	// whether the developer running the test is logged in. The keychain reads
	// os.UserConfigDir(), driven by HOME / XDG_CONFIG_HOME.
	testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")

	f := &cmdutil.Factory{Config: core.CliConfig{}, AuthClient: client.New("http://unused")}
	err := requireLogin(context.Background(), f)

	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeAuth {
		t.Fatalf("expected auth ExitError (no UAT in isolated keychain), got %v", err)
	}
}

// TestDashboardClient_DoesNotMutateAuthClient verifies that dashboardClient
// creates a fresh HTTP client for the Dashboard rather than mutating the shared
// f.AuthClient. Prior to the fix, NewDashboard(f.AuthClient, tok) called
// SetBearerToken on the shared client — credential bleed to any subsequent use
// of f.AuthClient (e.g. auth calls).
func TestDashboardClient_DoesNotMutateAuthClient(t *testing.T) {
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")

	// Seed UAT + partner token in the isolated keychain so PartnerToken() succeeds.
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_1"); err != nil {
		t.Fatalf("keychain Set uat: %v", err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, "partner", "ptok_1"); err != nil {
		t.Fatalf("keychain Set partner: %v", err)
	}

	f := &cmdutil.Factory{
		Config:     core.CliConfig{},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New("https://partners.example.com"),
	}

	_, err := dashboardClient(context.Background(), f)
	if err != nil {
		t.Fatalf("dashboardClient returned error: %v", err)
	}

	// The shared f.AuthClient must NOT have been mutated with the partner token.
	if got := f.AuthClient.Headers["Access-Token"]; got != "" {
		t.Fatalf("f.AuthClient was mutated: Headers[Access-Token]=%q, want empty", got)
	}
}

// seedLoginKeychain isolates HOME/keychain to a temp dir and seeds UAT +
// partner token so requireLogin/dashboardClient-style helpers get past the
// login gate. Also seeds a v2 account UAT (storeTokenForDomain's
// ExchangeEphemeral path reads Config.Accounts, not the legacy "uat" key).
// Returns the isolated dir (for ConfigPath).
func seedLoginKeychain(t *testing.T) string {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_1"); err != nil {
		t.Fatalf("keychain Set uat: %v", err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, "partner", "ptok_1"); err != nil {
		t.Fatalf("keychain Set partner: %v", err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@co.com"), "uat_1"); err != nil {
		t.Fatalf("keychain Set account uat: %v", err)
	}
	return dir
}

// deadServerURL returns the URL of a server that refuses connections (started
// then immediately closed), to provoke transport-level net.Errors.
func deadServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

// TestApiError_NetError_RoutesToErrNetwork: cmd/app's apiError classifies
// transport failures as network (exit 4), not internal (exit 5).
func TestApiError_NetError_RoutesToErrNetwork(t *testing.T) {
	d := app.NewDashboard(client.New(deadServerURL(t)), "ptok")
	_, err := d.GetPartners(context.Background())
	if err == nil {
		t.Fatal("expected a transport error from the dead server")
	}
	ee := apiError(err)
	if ee.Code != output.ExitNetwork {
		t.Fatalf("exit code = %d, want ExitNetwork (%d); detail=%+v", ee.Code, output.ExitNetwork, ee.Detail)
	}
}

// TestApiError_HTTPError_CarriesEndpoint: ErrAPI envelopes from apiError name
// the failing endpoint (method+path in error.detail).
func TestApiError_HTTPError_CarriesEndpoint(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	_, err := d.GetPartners(context.Background())
	ee := apiError(err)
	if ee.Detail == nil || ee.Detail.Detail == nil || ee.Detail.Detail.Method == "" || ee.Detail.Detail.Path == "" {
		t.Fatalf("expected endpoint in error detail, got %+v", ee.Detail)
	}
}

// TestStoreClient_NetError_RoutesToErrNetwork: a store-token mint that dies on
// the wire is network-class, not auth-class — exit 3 would misdirect the user
// to re-login.
func TestStoreClient_NetError_RoutesToErrNetwork(t *testing.T) {
	dir := seedLoginKeychain(t)
	f := &cmdutil.Factory{
		Config:     core.CliConfig{Accounts: []core.AccountConfig{{Name: "alice@co.com"}}},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New(deadServerURL(t)),
	}
	_, err := storeClient(context.Background(), f, "demo.myshoplazza.com")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitNetwork {
		t.Fatalf("exit code = %d, want ExitNetwork (%d); msg=%q", ee.Code, output.ExitNetwork, ee.Error())
	}
}

// TestStoreClient_AuthRejection_StaysAuth keeps the genuine-auth-failure branch
// on exit 3: a 403 on the token exchange is an auth problem.
func TestStoreClient_AuthRejection_StaysAuth(t *testing.T) {
	dir := seedLoginKeychain(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer srv.Close()
	f := &cmdutil.Factory{
		Config:     core.CliConfig{Accounts: []core.AccountConfig{{Name: "alice@co.com"}}},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New(srv.URL),
	}
	_, err := storeClient(context.Background(), f, "demo.myshoplazza.com")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitAuth {
		t.Fatalf("exit code = %d, want ExitAuth (%d); msg=%q", ee.Code, output.ExitAuth, ee.Error())
	}
}

// TestPartnerOpenapiClient_NetError_RoutesToErrNetwork mirrors the storeClient
// classification for the app-token mint.
func TestPartnerOpenapiClient_NetError_RoutesToErrNetwork(t *testing.T) {
	dir := seedLoginKeychain(t)
	dead := deadServerURL(t)
	f := &cmdutil.Factory{
		Config:     core.CliConfig{},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New(dead),
	}
	_, err := partnerOpenapiClient(context.Background(), f, "cid_1", "sec", "p1", dead)
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitNetwork {
		t.Fatalf("exit code = %d, want ExitNetwork (%d); msg=%q", ee.Code, output.ExitNetwork, ee.Error())
	}
}

// TestDashboardClient_WarnsOnUserIDFailure: a failed UserIDReady /
// AccessTokenReady is not silent — a one-line stderr warning names the root
// cause before the downstream 403 confuses the user. Still best-effort: the
// client is returned without error.
func TestDashboardClient_WarnsOnUserIDFailure(t *testing.T) {
	dir := seedLoginKeychain(t)
	var errBuf bytes.Buffer
	f := &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{ErrOut: &errBuf},
		Config: core.CliConfig{
			CurrentProfile: "demo",
			Profiles:       []core.ProfileConfig{{Name: "demo", Account: "alice@co.com", StoreDomain: "demo.myshoplazza.com"}},
		},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New(deadServerURL(t)), // Me + store-token exchange both die on the wire
	}
	d, err := dashboardClient(context.Background(), f)
	if err != nil || d == nil {
		t.Fatalf("dashboardClient must stay best-effort, got err=%v", err)
	}
	warnings := errBuf.String()
	if !strings.Contains(warnings, "warning: could not resolve login user id") {
		t.Errorf("missing user-id warning, stderr=%q", warnings)
	}
	if !strings.Contains(warnings, "warning: could not mint a store token for demo.myshoplazza.com") {
		t.Errorf("missing store-token warning, stderr=%q", warnings)
	}
}

// TestResolveStoreID_EmptyWithNilError: StoreIDFor's ("", nil) outcome (here:
// no UAT in the session) is treated as a resolution failure with the same
// actionable hint, instead of flowing an empty store_id to the backend.
func TestResolveStoreID_EmptyWithNilError(t *testing.T) {
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	f := &cmdutil.Factory{
		Config:     core.CliConfig{},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New("http://unused"),
	}
	_, err := resolveStoreID(context.Background(), f, "demo.myshoplazza.com")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitAuth {
		t.Fatalf("exit code = %d, want ExitAuth (%d)", ee.Code, output.ExitAuth)
	}
	if !strings.Contains(ee.Error(), "could not resolve store id for demo.myshoplazza.com") {
		t.Fatalf("message = %q, want the store-id resolution wording", ee.Error())
	}
}

// TestResolveStoreID_FromProfileConfig_NoV1Exchange is the merge-blocker
// regression: with a profile bound to the target store whose StoreID is
// already populated in config.json, resolveStoreID must return it directly
// rather than shadow-minting a full-scope v1 store token (StoreIDFor's
// exchange+persistState side effect) behind a limited-scope profile's back.
func TestResolveStoreID_FromProfileConfig_NoV1Exchange(t *testing.T) {
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	// A logged-in account (legacy UAT) so the old code path COULD reach the
	// v1 exchange if it were still consulted.
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatalf("keychain Set uat: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected network call %s — resolveStoreID must resolve from the profile, not the v1 exchange", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &cmdutil.Factory{
		Config: core.CliConfig{
			CurrentProfile: "demo",
			Profiles: []core.ProfileConfig{
				{Name: "demo", Account: "alice@co.com", StoreDomain: "demo.myshoplazza.com", StoreID: "12345", Scopes: []string{"read_product"}},
			},
		},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New(srv.URL),
	}

	got, err := resolveStoreID(context.Background(), f, "demo.myshoplazza.com")
	if err != nil {
		t.Fatalf("resolveStoreID: %v", err)
	}
	if got != "12345" {
		t.Fatalf("storeID = %q, want %q", got, "12345")
	}
	if tok, _ := keychain.Get(keychain.ShoplazzaCliService, "store:demo.myshoplazza.com"); tok != "" {
		t.Fatalf("a v1 store:<domain> keychain entry should not have been created, got %q", tok)
	}
}

// TestResolveStoreID_FromProfileMeta_NoV1Exchange covers the second lookup
// tier: the profile's config StoreID is empty (e.g. added before the id was
// backfilled), but ProfileMeta (auth/<name>.json, populated by every
// profile-scoped exchange) already has it. Still must not touch the v1 path.
func TestResolveStoreID_FromProfileMeta_NoV1Exchange(t *testing.T) {
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	if err := keychain.Set(keychain.ShoplazzaCliService, "uat", "uat_seed"); err != nil {
		t.Fatalf("keychain Set uat: %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	if err := internalauth.SaveProfileMeta(internalauth.AuthDir(configPath), "demo", internalauth.ProfileMeta{StoreID: "67890"}); err != nil {
		t.Fatalf("SaveProfileMeta: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected network call %s — resolveStoreID must resolve from profile meta, not the v1 exchange", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &cmdutil.Factory{
		Config: core.CliConfig{
			CurrentProfile: "demo",
			Profiles: []core.ProfileConfig{
				{Name: "demo", Account: "alice@co.com", StoreDomain: "demo.myshoplazza.com", Scopes: []string{"read_product"}},
			},
		},
		ConfigPath: configPath,
		AuthClient: client.New(srv.URL),
	}

	got, err := resolveStoreID(context.Background(), f, "demo.myshoplazza.com")
	if err != nil {
		t.Fatalf("resolveStoreID: %v", err)
	}
	if got != "67890" {
		t.Fatalf("storeID = %q, want %q", got, "67890")
	}
	if tok, _ := keychain.Get(keychain.ShoplazzaCliService, "store:demo.myshoplazza.com"); tok != "" {
		t.Fatalf("a v1 store:<domain> keychain entry should not have been created, got %q", tok)
	}
}

// TestLeafCommands_RejectPositionalArgs: leaf commands carry cobra.NoArgs, so a
// stray positional arg fails fast instead of being silently ignored.
func TestLeafCommands_RejectPositionalArgs(t *testing.T) {
	root := NewCmdApp(&cmdutil.Factory{})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"list", "bogus"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an unknown-argument error for `app list bogus`")
	}
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewCmdApp_HasAllSubcommands(t *testing.T) {
	cmd := NewCmdApp(&cmdutil.Factory{})
	want := map[string]bool{"init": false, "list": false, "info": false, "config": false,
		"extension": false, "versions": false, "deploy": false, "dev": false}
	for _, c := range cmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestRunList_PrintsApps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/cli/v2/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1", "name": "Acme"}}}})
		case strings.Contains(r.URL.Path, "/partners/p1/apps"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"apps": []map[string]any{
					{"client_id": "cid_a", "name": "App A"},
					{"client_id": "cid_b", "name": "App B"}}, "total": 2}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	var buf bytes.Buffer
	if err := runList(context.Background(), d, "", &buf, "json", ""); err != nil {
		t.Fatalf("runList: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cid_a") || !strings.Contains(out, "cid_b") {
		t.Fatalf("output missing client ids: %s", out)
	}
}

func TestResolveTargetStore(t *testing.T) {
	if got, err := resolveTargetStore("current.com"); err != nil || got != "current.com" {
		t.Fatalf("current store should resolve: got %q, %v", got, err)
	}
	_, err := resolveTargetStore("")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error when no current store, got %v", err)
	}
}

func TestSelectPartner_SingleAuto(t *testing.T) {
	id, err := selectPartner([]app.Partner{{ID: "p1", BusinessName: "Acme"}}, "")
	if err != nil || id != "p1" {
		t.Fatalf("got %q, %v", id, err)
	}
}

func TestSelectPartner_MultiNoFlag_Validation(t *testing.T) {
	_, err := selectPartner([]app.Partner{{ID: "p1"}, {ID: "p2"}}, "")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSelectPartner_FlagWins(t *testing.T) {
	id, err := selectPartner([]app.Partner{{ID: "p1"}, {ID: "p2"}}, "p2")
	if err != nil || id != "p2" {
		t.Fatalf("got %q, %v", id, err)
	}
}

func TestSelectPartner_FlagNotFound_Validation(t *testing.T) {
	_, err := selectPartner([]app.Partner{{ID: "p1"}}, "pX")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSelectPartner_None_Validation(t *testing.T) {
	_, err := selectPartner(nil, "")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestRunList_NoPartners_Errors(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"partners": []map[string]any{}}})
	})
	var buf bytes.Buffer
	err := runList(context.Background(), d, "", &buf, "json", "")
	if err == nil {
		t.Fatal("expected error when no partners available")
	}
}

func TestRunList_PartnerNotFound_Errors(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/partners") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1", "name": "Acme"}}}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"apps": []map[string]any{}}})
		}
	})
	var buf bytes.Buffer
	err := runList(context.Background(), d, "pX", &buf, "json", "")
	if err == nil {
		t.Fatal("expected error when --partner not found")
	}
}

func TestRunList_FilterByPartner_Success(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/partners") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{
					{"id": "p1", "name": "Acme"},
					{"id": "p2", "name": "Other"},
				}}})
		} else if strings.Contains(r.URL.Path, "/partners/p1/apps") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"apps": []map[string]any{{"client_id": "cid_x"}}}})
		} else {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	var buf bytes.Buffer
	if err := runList(context.Background(), d, "p1", &buf, "json", ""); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(buf.String(), "cid_x") {
		t.Errorf("output missing cid_x: %s", buf.String())
	}
}

// TestEnsurePartnerID covers the v1-project fallback: a toml partner_id is
// returned without any network call; an empty one is resolved live from /info
// (keyed on client_id); and an unresolvable one is a validation error.
func TestEnsurePartnerID(t *testing.T) {
	// 1. partner_id present in config → no /info call.
	noNet := app.NewDashboard(client.New("http://127.0.0.1:0"), "ptok")
	pid, ex := ensurePartnerID(context.Background(), noNet, project.Config{ClientID: "cid_1", PartnerID: "p_local"})
	if ex != nil || pid != "p_local" {
		t.Fatalf("present partner_id: pid=%q ex=%v (want p_local, nil; no network)", pid, ex)
	}

	// 2. empty partner_id → resolved from /info.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID := r.URL.Query().Get("app_client_id")
		w.Header().Set("Content-Type", "application/json")
		if gotID == "cid_empty" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partner": map[string]any{"id": "3634", "business_name": "212"}}})
			return
		}
		// no partner in payload
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{}})
	}))
	defer srv.Close()
	d := app.NewDashboard(client.New(srv.URL), "ptok")

	pid, ex = ensurePartnerID(context.Background(), d, project.Config{ClientID: "cid_empty"})
	if ex != nil || pid != "3634" {
		t.Fatalf("fallback resolve: pid=%q ex=%v (want 3634, nil)", pid, ex)
	}

	// 3. empty partner_id and /info has none → validation error.
	_, ex = ensurePartnerID(context.Background(), d, project.Config{ClientID: "cid_none"})
	if ex == nil || ex.Code != output.ExitValidation {
		t.Fatalf("unresolvable partner: ex=%v (want validation error)", ex)
	}
}

func TestReconcileExtensionApps(t *testing.T) {
	// Mismatch: warns and drops the cross-app id so it deploys as new.
	var buf bytes.Buffer
	got := reconcileExtensionApps(&buf, []app.LocalExt{
		{Name: "preorder", Type: "theme", ExtensionID: "657", AppID: "app_ff"},
	}, "app_xuxu")
	if got[0].ExtensionID != "" {
		t.Errorf("cross-app id should be dropped, got %q", got[0].ExtensionID)
	}
	if !strings.Contains(buf.String(), "preorder") || !strings.Contains(buf.String(), "app_ff") {
		t.Errorf("expected a cross-app warning naming the extension and its app, got %q", buf.String())
	}

	// Same app: keep the id, no warning.
	buf.Reset()
	got = reconcileExtensionApps(&buf, []app.LocalExt{
		{Name: "co", ExtensionID: "777", AppID: "app_xuxu"},
	}, "app_xuxu")
	if got[0].ExtensionID != "777" || buf.Len() != 0 {
		t.Errorf("same-app should keep id and not warn; id=%q warn=%q", got[0].ExtensionID, buf.String())
	}

	// v2 toml (no AppID): unchanged, no warning.
	buf.Reset()
	got = reconcileExtensionApps(&buf, []app.LocalExt{
		{Name: "v2", ExtensionID: "999"},
	}, "app_xuxu")
	if got[0].ExtensionID != "999" || buf.Len() != 0 {
		t.Errorf("v2 ext should be untouched; id=%q warn=%q", got[0].ExtensionID, buf.String())
	}
}
