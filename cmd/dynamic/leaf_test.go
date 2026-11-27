package dynamic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/pflag"
)

func rootFlags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("root", pflag.ContinueOnError)
	fs.String("format", "json", "Output format")
	fs.String("profile", "", "Profile to use")
	fs.Bool("no-input", false, "Never prompt")
	return fs
}

func TestBuildLeaf_NoBodyForGET(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"list"},
		HTTP: registry.HTTP{Method: "GET", Path: "/x"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule", nil)
	if leaf.Flags().Lookup("data") != nil {
		t.Fatal("GET must not register --data")
	}
	if leaf.Flags().Lookup("params") == nil {
		t.Fatal("--params is always present")
	}
	// --dry-run is a local leaf flag (not inherited from a module group),
	// so it appears in each spec leaf's own --help "Flags" block and module
	// group --help stays clean.
	if leaf.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run must be registered as a local flag on every leaf")
	}
}

func TestBuildLeaf_NoBodyForBlankBodyMarker(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"x"},
		HTTP: registry.HTTP{Method: "POST", Path: "/x"}, // no Body marker
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule", nil)
	if leaf.Flags().Lookup("data") != nil {
		t.Fatal("POST without http.body=='*' must not register --data")
	}
}

func TestBuildLeaf_BodyForPOST(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"create"},
		HTTP: registry.HTTP{Method: "POST", Path: "/x", Body: "*"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule", nil)
	if leaf.Flags().Lookup("data") == nil {
		t.Fatal("POST with body=='*' must register --data")
	}
}

func TestBuildLeaf_HiddenPropagated(t *testing.T) {
	cmd := registry.Command{
		Path:   []string{"x"},
		Hidden: true,
		HTTP:   registry.HTTP{Method: "GET", Path: "/x"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule", nil)
	if !leaf.Hidden {
		t.Fatal("hidden flag must propagate to cobra leaf")
	}
}

func TestBuildLeaf_SummaryFallsBackToID(t *testing.T) {
	cmd := registry.Command{
		ID:   "some-operation-id",
		Path: []string{"x"},
		HTTP: registry.HTTP{Method: "GET", Path: "/x"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule", nil)
	if leaf.Short != "some-operation-id" {
		t.Fatalf("short = %q, want id fallback", leaf.Short)
	}
}

func TestBuildLeaf_ParamFlags(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"list"},
		HTTP: registry.HTTP{Method: "GET", Path: "/orders/{order_id}/items"},
		Parameters: []registry.Parameter{
			{Name: "order_id", In: "path", Type: "string", Required: true, Description: "Order ID."},
			{Name: "page_size", In: "query", Type: "integer", Default: float64(10), Minimum: float64(1), Maximum: float64(250),
				Description: "Page   size with `ticks`.\n\n  - one per line\n"},
			{Name: "total_min", In: "query", Type: "number"},
			{Name: "published", In: "query", Type: "boolean", Description: "Only published"},
			{Name: "ids", In: "query", Type: "array", Required: true, Description: "IDs"},
			{Name: "since_ids", In: "query", Type: "array", Items: &registry.Field{Type: "integer"}},
			{Name: "status", In: "query", Enum: []any{"open", "closed"}},
			{Name: "format", In: "query", Type: "string", Description: "Export format"},
			{Name: "params", In: "query", Type: "string"},
		},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule", rootFlags())

	for name, want := range map[string][2]string{
		"order-id":  {"order_id", "string"},
		"page-size": {"page_size", "int"},
		"total-min": {"total_min", "number"},
		"published": {"published", "true|false"},
		"ids":       {"ids", "strings"},
		"since-ids": {"since_ids", "ints"},
		"status":    {"status", "string"},
	} {
		fl := leaf.Flags().Lookup(name)
		if fl == nil {
			t.Errorf("--%s is not registered", name)
			continue
		}
		if got := fl.Annotations[annotationParam]; len(got) != 1 || got[0] != want[0] {
			t.Errorf("--%s stands for %v, want %s", name, got, want[0])
		}
		if got := fl.Value.Type(); got != want[1] {
			t.Errorf("--%s type = %q, want %q", name, got, want[1])
		}
	}
	if leaf.Flags().Lookup("format") != nil {
		t.Error("a parameter named like the global --format must not get a flag")
	}
	if fl := leaf.Flags().Lookup("params"); fl == nil || len(fl.Annotations[annotationParam]) != 0 {
		t.Error("--params must stay the raw JSON flag")
	}
	if got := ParamFlags(leaf); got["page_size"] != "--page-size" || got["format"] != "" || got["params"] != "" {
		t.Errorf("ParamFlags = %v", got)
	}

	usage := leaf.UsageString()
	last := -1
	for _, want := range []string{
		"Usage:\n  list [flags]",
		"API Parameters:\n  Required:",
		"--ids strings", "IDs. Repeat or comma-separate",
		"--order-id string", "Order ID",
		"  Optional:",
		"--page-size int", "Page size with 'ticks'.\n", "- one per line. API default: 10. min 1, max 250",
		"--published true|false", "Only published",
		"--status string", "one of: open|closed",
		"Parameters set via --params (the flag name is taken):",
		`format: --params '{"format": "<value>"}'`, "Export format", "do not use --format (Output format)",
		`params: --params '{"params": "<value>"}'`,
		"Raw Input:\n", "--params string",
		"\nFlags:\n", "--dry-run",
	} {
		i := strings.Index(usage, want)
		if i < 0 {
			t.Fatalf("usage is missing %q:\n%s", want, usage)
		}
		if i < last {
			t.Errorf("%q is out of order in usage:\n%s", want, usage)
		}
		last = i
	}

	plain := buildLeafCommand(registry.Command{Path: []string{"count"}, HTTP: registry.HTTP{Method: "POST", Path: "/x", Body: "*"}},
		&registry.Spec{}, &cmdutil.Factory{}, "testmodule", rootFlags())
	usage = plain.UsageString()
	if strings.Contains(usage, "API Parameters:") || !strings.Contains(usage, "Raw Input:") || !strings.Contains(usage, "--data string") {
		t.Errorf("usage of a command without parameters:\n%s", usage)
	}
}

func TestBuildLeaf_IDFlagName(t *testing.T) {
	cases := []struct {
		path   string
		in     string
		others []string
		want   string
	}{
		{"/openapi/2026-01/pages/{id}", "path", nil, "page-id"},
		{"/openapi/2026-01/gift_cards/{id}", "path", nil, "gift-card-id"},
		{"/openapi/2026-01/markets/{id}/products", "path", []string{"collection_id"}, "market-id"},
		{"/openapi/2026-01/orders/{order_id}/risks/{id}", "path", []string{"order_id"}, "risk-id"},
		{"/openapi/2026-01/shipping-zone/{id}", "path", nil, "shipping-zone-id"},
		{"/openapi/2026-01/discounts/automatic/{id}", "path", nil, "discount-id"},
		{"/openapi/2026-01/discounts/non-automatic/{id}", "path", nil, "discount-id"},
		{"/openapi/2026-01/collections/async-task/{id}", "path", nil, "task-id"},
		{"/openapi/2026-01/metafields-shop/{id}", "path", nil, "metafield-id"},
		{"/openapi/2026-01/metafields/{owner_resource}/{owner_id}/{id}", "path", []string{"owner_resource", "owner_id"}, "metafield-id"},
		{"/openapi/2026-01/metafield_definition", "query", nil, "metafield-definition-id"},
		{"/openapi/2026-01/metafield_definition/{id}", "path", nil, "metafield-definition-id"},
		{"/openapi/2026-01/things/{thing_id}/{id}", "path", []string{"thing_id"}, "id"},
		{"/openapi/2026-01/search", "query", nil, "id"},
		{"/openapi/2026-01/pages/{id}", "path", []string{"page_id"}, "id"},
	}
	for _, c := range cases {
		params := []registry.Parameter{{Name: "id", In: c.in, Type: "string"}}
		for _, o := range c.others {
			in := "query"
			if strings.Contains(c.path, "{"+o+"}") {
				in = "path"
			}
			params = append(params, registry.Parameter{Name: o, In: in, Type: "string"})
		}
		leaf := buildLeafCommand(registry.Command{Path: []string{"x"}, HTTP: registry.HTTP{Method: "GET", Path: c.path}, Parameters: params},
			&registry.Spec{}, &cmdutil.Factory{}, "testmodule", rootFlags())
		if got := ParamFlags(leaf)["id"]; got != "--"+c.want {
			t.Errorf("%s (%s, with %v): id flag = %q, want --%s", c.path, c.in, c.others, got, c.want)
		}
		if fl := leaf.Flags().Lookup("id"); fl == nil || fl.Name != c.want {
			t.Errorf("%s: --id does not reach --%s", c.path, c.want)
		}
		if c.want != "id" && strings.Contains(leaf.UsageString(), "--id ") {
			t.Errorf("%s: usage lists --id beside --%s", c.path, c.want)
		}
	}
}

func TestBuildLeaf_EmbeddedSpec(t *testing.T) {
	var spec registry.Spec
	if err := json.Unmarshal(registry.Embedded, &spec); err != nil {
		t.Fatal(err)
	}
	wantID := map[string]int{
		"article-id": 3, "blog-id": 3, "collect-id": 2, "collection-id": 5, "coupon-id": 2, "discount-id": 4,
		"gift-card-id": 3, "market-id": 13, "menu-id": 2, "metafield-definition-id": 3, "metafield-id": 6,
		"page-id": 3, "redirect-id": 3, "risk-id": 3, "shipping-zone-id": 2, "supplier-id": 2, "task-id": 1, "webhook-id": 3,
	}
	gotID := map[string]int{}
	for _, mod := range spec.Modules {
		for _, c := range mod.Commands {
			name := mod.Name + " " + strings.Join(c.Path, " ")
			leaf := buildLeafCommand(c, &spec, &cmdutil.Factory{}, mod.Name, rootFlags())
			flags := ParamFlags(leaf)
			for _, p := range c.Parameters {
				flag := flags[p.Name]
				if flag == "" {
					t.Errorf("%s: parameter %s has no flag", name, p.Name)
				}
				if p.Name == "id" {
					gotID[strings.TrimPrefix(flag, "--")]++
				}
			}
			if !strings.Contains(leaf.UsageString(), "Usage:") {
				t.Errorf("%s: usage does not render", name)
			}
		}
	}
	for flag, n := range wantID {
		if gotID[flag] != n {
			t.Errorf("--%s names %d id parameters, want %d", flag, gotID[flag], n)
		}
		delete(gotID, flag)
	}
	if len(gotID) > 0 {
		t.Errorf("unexpected id flags: %v", gotID)
	}
}
