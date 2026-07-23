package dynamic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/registry"

	"github.com/spf13/cobra"
)

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
