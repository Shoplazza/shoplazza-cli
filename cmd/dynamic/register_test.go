package dynamic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/spf13/cobra"
)

func newFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	return &cmdutil.Factory{}
}

func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func TestRegisterCommands_NilSpec(t *testing.T) {
	root := &cobra.Command{}
	RegisterCommands(root, nil, newFactory(t))
	if len(root.Commands()) != 0 {
		t.Fatalf("nil spec must register nothing, got %d", len(root.Commands()))
	}
}

func TestRegisterCommands_EmptyModules(t *testing.T) {
	root := &cobra.Command{}
	RegisterCommands(root, &registry.Spec{}, newFactory(t))
	if len(root.Commands()) != 0 {
		t.Fatalf("empty modules: got %d cmds", len(root.Commands()))
	}
}

func TestRegisterCommands_SkipsCollidingNames(t *testing.T) {
	root := &cobra.Command{}
	root.AddCommand(&cobra.Command{Use: "auth"})
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "auth", Commands: []registry.Command{
			{Path: []string{"login"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	if cnt := len(root.Commands()); cnt != 1 {
		t.Fatalf("expected built-in auth retained without dup; got %d cmds", cnt)
	}
}

func TestRegisterCommands_SkipsDuplicateModules(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{
		{Name: "orders", Commands: []registry.Command{{Path: []string{"a"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}}}},
		{Name: "orders", Commands: []registry.Command{{Path: []string{"b"}, HTTP: registry.HTTP{Method: "GET", Path: "/b"}}}},
	}}
	RegisterCommands(root, spec, newFactory(t))
	var orders int
	for _, c := range root.Commands() {
		if c.Name() == "orders" {
			orders++
		}
	}
	if orders != 1 {
		t.Fatalf("duplicate modules: got %d 'orders' cmds, want 1", orders)
	}
}

func TestRegisterCommands_SkipsBadModuleName(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{
		{Name: "Bad_Name", Commands: []registry.Command{{Path: []string{"a"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}}}},
		{Name: "good", Commands: []registry.Command{{Path: []string{"a"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}}}},
	}}
	RegisterCommands(root, spec, newFactory(t))
	if hasSubcommand(root, "Bad_Name") {
		t.Fatal("non-kebab-case module must be skipped")
	}
	if !hasSubcommand(root, "good") {
		t.Fatal("good module should register")
	}
}

func TestRegisterCommands_DuplicatePathSkipsBoth(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "x", Commands: []registry.Command{
			{Path: []string{"dup"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}},
			{Path: []string{"dup"}, HTTP: registry.HTTP{Method: "GET", Path: "/b"}},
			{Path: []string{"keep"}, HTTP: registry.HTTP{Method: "GET", Path: "/c"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	x := root.Commands()[0]
	if hasSubcommand(x, "dup") {
		t.Fatal("duplicate path[] commands must both be skipped")
	}
	if !hasSubcommand(x, "keep") {
		t.Fatal("non-conflicting command must remain")
	}
}

func TestRegisterCommands_PrefixConflictSkipsBoth(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "x", Commands: []registry.Command{
			{Path: []string{"coupons"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}},
			{Path: []string{"coupons", "create"}, HTTP: registry.HTTP{Method: "POST", Path: "/b", Body: "*"}},
			{Path: []string{"survivor"}, HTTP: registry.HTTP{Method: "GET", Path: "/c"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	x := root.Commands()[0]
	for _, c := range x.Commands() {
		if c.Name() == "coupons" {
			t.Fatalf("prefix-conflict commands must be skipped, found %q", c.Name())
		}
	}
	if !hasSubcommand(x, "survivor") {
		t.Fatal("survivor must remain")
	}
}

func TestRegisterCommands_ImplicitGroupReuse(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "discounts", Commands: []registry.Command{
			{Path: []string{"coupons", "create"}, HTTP: registry.HTTP{Method: "POST", Path: "/c", Body: "*"}},
			{Path: []string{"coupons", "get"}, HTTP: registry.HTTP{Method: "GET", Path: "/g"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	discounts := root.Commands()[0]
	var coupons *cobra.Command
	for _, c := range discounts.Commands() {
		if c.Name() == "coupons" {
			coupons = c
		}
	}
	if coupons == nil {
		t.Fatal("coupons implicit group should exist")
	}
	if !hasSubcommand(coupons, "create") || !hasSubcommand(coupons, "get") {
		t.Fatal("coupons group must hold both create + get leaves")
	}
}

func TestRegisterCommands_ModuleWithZeroValidCommandsSkipped(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "broken", Commands: []registry.Command{
			{Path: []string{"BAD_NAME"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	if hasSubcommand(root, "broken") {
		t.Fatal("module with 0 valid commands must not appear")
	}
}

// rootWithSpec builds a fresh cobra root, attaches a few stubbed built-in
// commands, and registers all module commands from spec. The returned root
// approximates the real CLI for end-to-end behaviour testing.
func rootWithSpec(t *testing.T, spec *registry.Spec, factory *cmdutil.Factory) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "shoplazza", SilenceUsage: true, SilenceErrors: true}
	// Mirror cmd.RegisterGlobalFlags — these are inherited by every leaf via
	// PersistentFlags propagation.
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	// Stub built-ins so collision protection has names to skip against.
	root.AddCommand(&cobra.Command{Use: "auth"})
	root.AddCommand(&cobra.Command{Use: "doctor"})
	RegisterCommands(root, spec, factory)
	return root
}

func factoryAt(t *testing.T, srv *httptest.Server) (*cmdutil.Factory, *strings.Builder) {
	t.Helper()
	out := &strings.Builder{}
	c := client.New(srv.URL)
	c.SetBearerToken("dev")
	return &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: &strings.Builder{}},
		Client:    c,
	}, out
}

// Scenario 1: real dynamic resource hit — full path from spec to backend.
func TestIntegration_DynamicOrdersList(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		if !strings.Contains(r.URL.RawQuery, "page_size=10") {
			t.Errorf("query lost: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orders":[{"id":"o1"}]}`))
	}))
	defer srv.Close()
	f, out := factoryAt(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL) // gate still needs an explicit store target

	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "orders",
		Commands: []registry.Command{{
			ID: "order-list", Path: []string{"list"},
			HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/orders"},
		}},
	}}}
	root := rootWithSpec(t, spec, f)
	root.SetArgs([]string{"orders", "list", "--params", `{"page_size":10}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !hit {
		t.Fatal("backend not hit")
	}
	if !strings.Contains(out.String(), "orders") {
		t.Fatalf("stdout = %q", out.String())
	}
}

// Scenario 2: empty modules → unknown command exits with cobra error.
func TestIntegration_EmptyModulesUnknownCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("backend should not be hit")
	}))
	defer srv.Close()
	f, _ := factoryAt(t, srv)

	root := rootWithSpec(t, &registry.Spec{}, f)
	root.SetArgs([]string{"orders", "list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected unknown-command error")
	}
	// Cobra returns a non-ExitError; root.go wraps it. We assert it is unknown-command-ish.
	if !strings.Contains(err.Error(), "orders") && !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

// Scenario 3: corrupt spec yields LoadSpec → empty Spec; non-spec commands still work.
// LoadSpec is verified separately; here we exercise the equivalent path: pass nil spec.
func TestIntegration_NilSpecBuiltInsStillWork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit")
	}))
	defer srv.Close()
	f, _ := factoryAt(t, srv)

	root := rootWithSpec(t, nil, f)
	// 'auth' stub was added in rootWithSpec — registration should still see it.
	if !hasChild(root, "auth") {
		t.Fatal("built-in auth must remain registered with nil spec")
	}
	root.SetArgs([]string{"orders", "list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("orders should be unknown without spec")
	}
}

// Scenario 4: invalid command (bad HTTP method) is skipped; sibling registers.
func TestIntegration_BadCommandSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	f, _ := factoryAt(t, srv)

	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "orders",
		Commands: []registry.Command{
			{Path: []string{"bad"}, HTTP: registry.HTTP{Method: "FOO", Path: "/x"}},
			{Path: []string{"good"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}},
		},
	}}}
	root := rootWithSpec(t, spec, f)
	orders := findChild(t, root, "orders")
	if hasChild(orders, "bad") {
		t.Fatal("bad command must be skipped")
	}
	if !hasChild(orders, "good") {
		t.Fatal("sibling good command must register")
	}
}

// Scenario 5: prefix conflict — both conflicting commands skipped, others register.
func TestIntegration_PrefixConflictSkipsBoth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	f, _ := factoryAt(t, srv)

	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "discounts",
		Commands: []registry.Command{
			{Path: []string{"coupons"}, HTTP: registry.HTTP{Method: "GET", Path: "/c"}},
			{Path: []string{"coupons", "create"}, HTTP: registry.HTTP{Method: "POST", Path: "/cc", Body: "*"}},
			{Path: []string{"survivor"}, HTTP: registry.HTTP{Method: "GET", Path: "/s"}},
		},
	}}}
	root := rootWithSpec(t, spec, f)
	discounts := findChild(t, root, "discounts")
	for _, c := range discounts.Commands() {
		if c.Name() == "coupons" {
			t.Fatalf("prefix-conflict cmd 'coupons' should be skipped")
		}
	}
	if !hasChild(discounts, "survivor") {
		t.Fatal("survivor must register")
	}
}

// Scenario 6: three-level command tree. coupons is implicit group; create + get under it.
func TestIntegration_ThreeLevelCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	f, _ := factoryAt(t, srv)

	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "discounts",
		Commands: []registry.Command{
			{Path: []string{"coupons", "create"}, HTTP: registry.HTTP{Method: "POST", Path: "/c", Body: "*"}},
			{Path: []string{"coupons", "get"}, HTTP: registry.HTTP{Method: "GET", Path: "/g"}},
		},
	}}}
	root := rootWithSpec(t, spec, f)
	discounts := findChild(t, root, "discounts")
	coupons := findChild(t, discounts, "coupons")
	if !hasChild(coupons, "create") || !hasChild(coupons, "get") {
		t.Fatal("coupons group must hold create + get leaves")
	}
}

// Scenario 7: products / discounts run via spec (mirrors real cli_meta routes).
func TestIntegration_ProductsDiscountsViaSpec(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL) // gate still needs an explicit store target

	spec := &registry.Spec{Modules: []registry.Module{
		{Name: "products", Commands: []registry.Command{
			{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/products"}},
		}},
		{Name: "discounts", Commands: []registry.Command{
			{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/discounts"}},
		}},
	}}

	// products list — fresh root per invocation to avoid cobra arg state issues.
	f1, _ := factoryAt(t, srv)
	root1 := rootWithSpec(t, spec, f1)
	root1.SetArgs([]string{"products", "list"})
	if err := root1.Execute(); err != nil {
		t.Fatalf("products list: %v", err)
	}

	// discounts list — fresh root.
	f2, _ := factoryAt(t, srv)
	root2 := rootWithSpec(t, spec, f2)
	root2.SetArgs([]string{"discounts", "list"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("discounts list: %v", err)
	}

	if got := strings.Join(paths, "; "); !strings.Contains(got, "GET /openapi/2026-01/products") || !strings.Contains(got, "GET /openapi/2026-01/discounts") {
		t.Fatalf("expected both paths hit, got %q", got)
	}
}

// Scenario 8: --dry-run never sends HTTP.
func TestIntegration_DryRunNoBackendHit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("dry-run must not send HTTP")
	}))
	defer srv.Close()
	f, out := factoryAt(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL) // gate still needs an explicit store target

	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "orders",
		Commands: []registry.Command{{
			Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/orders"},
		}},
	}}}
	root := rootWithSpec(t, spec, f)
	root.SetArgs([]string{"orders", "list", "--params", `{"page_size":1}`, "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("expected dry_run in stdout, got %q", out.String())
	}
}

// ---- helpers ----

func hasChild(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func findChild(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("child %q not found under %q", name, parent.Name())
	return nil
}

// notLoggedInFactory builds a Factory with a resolvable profile but a fresh
// (empty) keychain and no SHOPLAZZA_ACCESS_TOKEN, so cmdutil.RequireAuth
// resolves the profile but fails to mint its token, reporting "not logged
// in". Mirrors tempFactory in internal/cmdutil/require_auth_test.go.
func notLoggedInFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	out := &strings.Builder{}
	cfg := core.CliConfig{ConfigVersion: 2, CurrentProfile: "us",
		Profiles: []core.ProfileConfig{{Name: "us", Account: "a@co.com", StoreDomain: "us.myshoplazza.com"}}}
	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: &strings.Builder{}},
		ConfigPath: filepath.Join(dir, "config.json"),
		Config:     cfg,
		Client:     client.New("http://127.0.0.1:1"),
		AuthClient: client.New("http://127.0.0.1:1"),
	}
}

// themesSpec is a minimal spec with one valid spec-generated leaf so
// buildModuleCommand produces a real module node (the auth gate host).
func themesSpec() *registry.Spec {
	return &registry.Spec{Modules: []registry.Module{{
		Name: "themes",
		Commands: []registry.Command{{
			ID: "theme-list", Path: []string{"list"},
			HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/themes"},
		}},
	}}}
}

// mountUnderThemes registers the spec module on a fresh root and mounts s
// under the resulting "themes" module command — the same wiring production
// uses (RegisterCommands then RegisterShortcuts finds the existing group).
func mountUnderThemes(t *testing.T, f *cmdutil.Factory, s common.Shortcut) *cobra.Command {
	t.Helper()
	root := rootWithSpec(t, themesSpec(), f)
	var themesCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "themes" {
			themesCmd = c
		}
	}
	if themesCmd == nil {
		t.Fatal("themes module command not registered")
	}
	common.Mount(s, themesCmd, f)
	return root
}

// TestAuthGate_AuthFreeShortcutRunsWithoutLogin: a Shortcut declaring
// AuthFree (themes init / package) must execute with no credentials at all —
// previously even --dry-run exited 3.
func TestAuthGate_AuthFreeShortcutRunsWithoutLogin(t *testing.T) {
	f := notLoggedInFactory(t)
	ran := false
	s := common.Shortcut{
		Service: "themes", Command: "local-probe", Use: "local-probe", Short: "p",
		AuthFree: true, Local: true,
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			ran = true
			return common.ExecResult{Body: map[string]any{"status": "ok"}}, nil
		},
	}
	root := mountUnderThemes(t, f, s)
	root.SetArgs([]string{"themes", "local-probe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("AuthFree shortcut must run without login; got: %v", err)
	}
	if !ran {
		t.Fatal("Execute handler never ran")
	}
}

// TestAuthGate_NormalShortcutStillGated: a shortcut WITHOUT AuthFree under the
// same module must keep hitting the auth gate (type=auth, "not logged in").
func TestAuthGate_NormalShortcutStillGated(t *testing.T) {
	f := notLoggedInFactory(t)
	s := common.Shortcut{
		Service: "themes", Command: "gated-probe", Use: "gated-probe", Short: "p",
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			t.Error("gated shortcut must not execute without login")
			return common.ExecResult{}, nil
		},
	}
	root := mountUnderThemes(t, f, s)
	root.SetArgs([]string{"themes", "gated-probe"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected auth-gate error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeAuth {
		t.Fatalf("expected type=auth ExitError, got %T: %v", err, err)
	}
}

// TestAuthGate_SpecLeafStillGated: spec-generated leaves carry no AuthFree
// annotation and must remain gated.
func TestAuthGate_SpecLeafStillGated(t *testing.T) {
	f := notLoggedInFactory(t)
	root := rootWithSpec(t, themesSpec(), f)
	root.SetArgs([]string{"themes", "list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected auth-gate error for the spec leaf")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeAuth {
		t.Fatalf("expected type=auth ExitError, got %T: %v", err, err)
	}
}
