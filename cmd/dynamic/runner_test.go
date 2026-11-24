package dynamic

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
)

func newRunFactory(t *testing.T, srv *httptest.Server) (*cmdutil.Factory, *strings.Builder, *strings.Builder) {
	t.Helper()
	out := &strings.Builder{}
	errOut := &strings.Builder{}
	c := client.New(srv.URL)
	c.SetBearerToken("dev")
	return &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: errOut},
		Client:    c,
	}, out, errOut
}

func TestRunE_InvalidParamsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit on validation error")
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", "not-json"})
	err := leaf.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != "validation" {
		t.Fatalf("expected validation ExitError, got %T %v", err, err)
	}
}

func TestRunE_MissingPathParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit on missing path param")
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"get"}, HTTP: registry.HTTP{Method: "GET", Path: "/x/{id}"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", "{}"})
	err := leaf.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != "validation" {
		t.Fatalf("expected validation, got %v", err)
	}
}

func TestRunE_DryRunDoesNotSendRequest(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	f, out, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/orders"},
		Parameters: []registry.Parameter{{Name: "page_size", In: "query"}}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	// --dry-run is inherited from the root's PersistentFlags in production;
	// wrap the leaf in a stub parent that mirrors that setup.
	parent := &cobra.Command{Use: "shoplazza", SilenceUsage: true, SilenceErrors: true}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.AddCommand(leaf)
	parent.SetArgs([]string{"list", "--params", `{"page_size":10}`, "--dry-run"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit {
		t.Fatal("dry-run must not call backend")
	}
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("expected dry_run marker in output, got: %s", out.String())
	}
}

func TestRunE_HappyPathListGET(t *testing.T) {
	var gotURL string
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"discounts":[{"id":"d1"}]}`))
	}))
	defer srv.Close()
	f, out, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/discounts"},
		Parameters: []registry.Parameter{
			{Name: "page_size", In: "query"},
			{Name: "ids", In: "query", Type: "array"},
			{Name: "vendors", In: "query", Type: "array"},
			{Name: "title", In: "query", Type: "string"},
		}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", `{"page_size":10,"ids":"d1, d2,,","vendors":["Nike, Inc."],"title":"a,b"}`})
	if err := leaf.Execute(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(gotURL, "page_size=10") {
		t.Fatalf("expected page_size in URL, got %q", gotURL)
	}
	for key, want := range map[string][]string{
		"ids":     {"d1", "d2"},   // comma-joined string for an array param: split
		"vendors": {"Nike, Inc."}, // JSON array: sent as given
		"title":   {"a,b"},        // not an array param: untouched
	} {
		if got := gotQuery[key]; strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("query %s = %q, want %q (URL %q)", key, got, want, gotURL)
		}
	}
	if !strings.Contains(out.String(), "discounts") {
		t.Fatalf("expected response in stdout, got %q", out.String())
	}
}

func TestRunE_PathParamSubstituted(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"get"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/discounts/{id}"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", `{"id":"d001"}`})
	if err := leaf.Execute(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotPath != "/openapi/2026-01/discounts/d001" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestRunE_HTTPErrorBecomesAPIExitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"bad"}`))
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	err := leaf.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != "api" {
		t.Fatalf("expected api ExitError, got %v", err)
	}
}

func TestRunE_BodyPOSTWithData(t *testing.T) {
	var gotBody json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"new"}`))
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{
		Path: []string{"create"},
		HTTP: registry.HTTP{Method: "POST", Path: "/openapi/2026-01/coupons", Body: "*"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--data", `{"coupon":{"code":"X"}}`})
	if err := leaf.Execute(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(string(gotBody), `"code":"X"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestRunE_ParamsAndBodyChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server must not be hit")
	}))
	defer srv.Close()
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	sessionLeaf := registry.Command{
		Path: []string{"add"},
		HTTP: registry.HTTP{Method: "POST", Path: "/themes/edit-sessions/{oseid}/sections/{section_id}/blocks", Body: "*"},
		Parameters: []registry.Parameter{
			{Name: "oseid", In: "path", Required: true}, {Name: "section_id", In: "path", Required: true},
		},
		Body: &registry.Body{Required: true, Fields: []registry.Field{
			{Name: "doc_id", Required: true}, {Name: "theme_id", Required: true},
			{Name: "block", Required: true, Schema: "Block"}, {Name: "index"},
		}},
	}
	noParams := registry.Command{Path: []string{"count"}, HTTP: registry.HTTP{Method: "GET", Path: "/customers/count"}}
	twoPath := registry.Command{Path: []string{"get"}, HTTP: registry.HTTP{Method: "GET", Path: "/customers/{customer_id}/addresses/{address_id}"},
		Parameters: []registry.Parameter{{Name: "customer_id", In: "path", Required: true}, {Name: "address_id", In: "path", Required: true}}}
	getOne := registry.Command{Path: []string{"get"}, HTTP: registry.HTTP{Method: "GET", Path: "/products/{product_id}"},
		Parameters: []registry.Parameter{{Name: "product_id", In: "path", Required: true}}}
	pathAndQuery := registry.Command{Path: []string{"items"}, HTTP: registry.HTTP{Method: "GET", Path: "/locations/{location_id}/items"},
		Parameters: []registry.Parameter{{Name: "location_id", In: "path", Required: true}, {Name: "variant_ids", In: "query", Type: "array", Required: true}, {Name: "page_size", In: "query"}}}
	productUpdate := registry.Command{Path: []string{"update"}, HTTP: registry.HTTP{Method: "PUT", Path: "/products/{product_id}", Body: "*"},
		Parameters: []registry.Parameter{{Name: "product_id", In: "path", Required: true}},
		Body:       &registry.Body{Fields: []registry.Field{{Name: "product", Type: "object", Schema: "Product"}}}}
	pageUpdate := registry.Command{Path: []string{"update"}, HTTP: registry.HTTP{Method: "PUT", Path: "/pages/{id}", Body: "*"},
		Parameters: []registry.Parameter{{Name: "id", In: "path", Required: true}},
		Body:       &registry.Body{Fields: []registry.Field{{Name: "page", Type: "object", Schema: "Page", Required: true}}}}
	updateBySku := registry.Command{Path: []string{"update-by-sku"}, HTTP: registry.HTTP{Method: "PUT", Path: "/variants/sku/{sku}", Body: "*"},
		Parameters: []registry.Parameter{{Name: "sku", In: "path", Required: true}},
		Body: &registry.Body{Fields: []registry.Field{
			{Name: "refuse_multi_result", Type: "boolean", Required: true}, {Name: "variant", Type: "object", Schema: "Variant", Required: true},
		}}}
	spec := &registry.Spec{Schemas: map[string]registry.ObjectSchema{
		"Product": {Fields: []registry.Field{{Name: "title", Type: "string"}, {Name: "description", Type: "string"}}},
		"Page":    {Fields: []registry.Field{{Name: "title", Type: "string"}}},
		"Variant": {Fields: []registry.Field{{Name: "price", Type: "number"}}},
	}}
	long := strings.Repeat("x", 300)
	path := `{"oseid":"os1","section_id":"s1"}`
	full := `{"doc_id":"d1","theme_id":"T1","block":{}}`
	cases := []struct {
		name        string
		cmd         registry.Command
		args        []string
		wantSubtype string
		wantParam   string
		wantMsg     string
		wantHint    string
	}{
		{"body field in --params", sessionLeaf,
			[]string{"--params", `{"oseid":"os1","section_id":"s1","theme_id":"T1"}`, "--data", `{"doc_id":"d1","block":{}}`},
			output.SubtypeMisplacedParam, "theme_id", "theme_id is a request-body field", "move it from --params into --data"},
		{"two body fields in --params", sessionLeaf,
			[]string{"--params", `{"oseid":"os1","section_id":"s1","theme_id":"T1","doc_id":"d1"}`, "--data", `{"block":{}}`},
			output.SubtypeMisplacedParam, "doc_id", "doc_id, theme_id are request-body fields", "move them"},
		{"undeclared --params key", sessionLeaf,
			[]string{"--params", `{"oseid":"os1","section_id":"s1","themeid":"T1"}`, "--data", full},
			output.SubtypeUnknownParam, "themeid", "unknown --params key: themeid", "accepted --params keys: oseid, section_id"},
		{"--params on a command that declares none", noParams,
			[]string{"--params", `{"created_at_min":"2026-01-01"}`},
			output.SubtypeUnknownParam, "created_at_min", "unknown --params key", "takes no --params"},
		{"required body field missing", sessionLeaf,
			[]string{"--params", path, "--data", `{"doc_id":"d1","block":{}}`},
			output.SubtypeMissingRequiredField, "theme_id", "--data is missing required field: theme_id", "schema"},
		{"null counts as missing", sessionLeaf,
			[]string{"--params", path, "--data", `{"doc_id":"d1","theme_id":null,"block":{}}`},
			output.SubtypeMissingRequiredField, "theme_id", "missing required field: theme_id", ""},
		{"no --data at all", sessionLeaf,
			[]string{"--params", path},
			output.SubtypeMissingRequiredField, "doc_id", "missing required fields: doc_id, theme_id, block", ""},
		{"nested fields are not checked", sessionLeaf,
			[]string{"--params", path, "--data", full}, "", "", "", ""},
		{"fields outside their wrapper", productUpdate,
			[]string{"--params", `{"product_id":"p1"}`, "--data", `{"description":"x"}`},
			output.SubtypeMissingRequiredField, "product", `--data is missing the "product" wrapper: {"product":{"description":"x"}}`, "schema"},
		{"fields outside a required wrapper", pageUpdate,
			[]string{"--params", `{"id":"1"}`, "--data", `{"title":"x"}`},
			output.SubtypeMissingRequiredField, "page", `--data is missing the "page" wrapper: {"page":{"title":"x"}}`, ""},
		{"wrapper fields beside another top-level field", updateBySku,
			[]string{"--params", `{"sku":"s1"}`, "--data", `{"refuse_multi_result":true,"price":1}`},
			output.SubtypeMissingRequiredField, "variant", `--data is missing the "variant" wrapper: {"refuse_multi_result":true,"variant":{"price":1}}`, ""},
		{"a stray field joins a wrapper already given", productUpdate,
			[]string{"--params", `{"product_id":"p1"}`, "--data", `{"product":{"title":"t"},"description":"x"}`},
			output.SubtypeMissingRequiredField, "product", `{"product":{"description":"x","title":"t"}}`, ""},
		{"a long body is shown by its keys", productUpdate,
			[]string{"--params", `{"product_id":"p1"}`, "--data", `{"description":"` + long + `","title":"t"}`},
			output.SubtypeMissingRequiredField, "product", `{"product":{"description":…,"title":…}}`, ""},
		{"wrapped fields pass", productUpdate,
			[]string{"--params", `{"product_id":"p1"}`, "--data", `{"product":{"description":"x"}}`}, "", "", "", ""},
		{"a key no wrapper has is left alone", productUpdate,
			[]string{"--params", `{"product_id":"p1"}`, "--data", `{"bogus":1}`}, "", "", "", ""},
		{"every missing path param at once", twoPath,
			[]string{"--params", `{}`},
			output.SubtypeMissingRequiredParam, "customer_id", "--params is missing required keys: customer_id, address_id", "to see required parameters"},
		{"one of two path params", twoPath,
			[]string{"--params", `{"customer_id":"c1"}`},
			output.SubtypeMissingRequiredParam, "address_id", "--params is missing required key: address_id", ""},
		{"path then required query", pathAndQuery,
			[]string{"--params", `{"page_size":5}`},
			output.SubtypeMissingRequiredParam, "location_id", "missing required keys: location_id, variant_ids", ""},
		{"empty string counts as missing", getOne,
			[]string{"--params", `{"product_id":""}`},
			output.SubtypeMissingRequiredParam, "product_id", "missing required key: product_id", ""},
		{"empty array counts as missing", pathAndQuery,
			[]string{"--params", `{"location_id":"l1","variant_ids":[]}`},
			output.SubtypeMissingRequiredParam, "variant_ids", "missing required key: variant_ids", ""},
		{"only commas count as missing", pathAndQuery,
			[]string{"--params", `{"location_id":"l1","variant_ids":" , "}`},
			output.SubtypeMissingRequiredParam, "variant_ids", "missing required key: variant_ids", ""},
		{"brace placeholder", getOne,
			[]string{"--params", `{"product_id":"{product_id}"}`},
			output.SubtypeMissingRequiredParam, "product_id", `product_id (placeholder "{product_id}")`, ""},
		{"angle placeholder", getOne,
			[]string{"--params", `{"product_id":"<id>"}`},
			output.SubtypeMissingRequiredParam, "product_id", `product_id (placeholder "<id>")`, ""},
		{"mustache placeholder", getOne,
			[]string{"--params", `{"product_id":"{{product_id}}"}`},
			output.SubtypeMissingRequiredParam, "product_id", `product_id (placeholder "{{product_id}}")`, ""},
		{"placeholder with a space", getOne,
			[]string{"--params", `{"product_id":"<product id>"}`},
			output.SubtypeMissingRequiredParam, "product_id", `product_id (placeholder "<product id>")`, ""},
		{"braces around a number are not a name", getOne,
			[]string{"--params", `{"product_id":"{1}"}`}, "", "", "", ""},
		{"braces inside a real value", getOne,
			[]string{"--params", `{"product_id":"p-{1}"}`}, "", "", "", ""},
		{"required params all given", pathAndQuery,
			[]string{"--params", `{"location_id":"l1","variant_ids":["v1"]}`}, "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, _, _ := newRunFactory(t, srv)
			leaf := buildLeafCommand(c.cmd, spec, f, "testmodule")
			leaf.SetArgs(append(c.args, "--dry-run"))
			err := leaf.Execute()
			if c.wantSubtype == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
				t.Fatalf("want a validation error, got %v", err)
			}
			if ee.Detail.Subtype != c.wantSubtype || ee.Detail.Param != c.wantParam {
				t.Errorf("subtype/param = %q/%q, want %q/%q", ee.Detail.Subtype, ee.Detail.Param, c.wantSubtype, c.wantParam)
			}
			if !strings.Contains(ee.Detail.Message, c.wantMsg) || !strings.Contains(ee.Detail.Hint, c.wantHint) {
				t.Errorf("message/hint = %q / %q, want %q / %q", ee.Detail.Message, ee.Detail.Hint, c.wantMsg, c.wantHint)
			}
		})
	}
}
