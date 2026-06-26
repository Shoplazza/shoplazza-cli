package products

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/shortcuts/common"
)

func newProductExecInput(t *testing.T, flags map[string]string, values map[string]string, dryRun bool) common.ExecInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for name, typ := range flags {
		switch typ {
		case "string":
			cmd.Flags().String(name, "", "")
		case "int":
			cmd.Flags().Int(name, 0, "")
		case "bool":
			cmd.Flags().Bool(name, false, "")
		case "stringslice":
			cmd.Flags().StringSlice(name, nil, "")
		}
	}
	var args []string
	for name, val := range values {
		args = append(args, "--"+name+"="+val)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	return common.ExecInput{Flags: common.NewCobraFlagSet(cmd), DryRun: dryRun}
}

// ── stockShortcut.Execute ─────────────────────────────────────────────────────

var stockExecFlags = map[string]string{
	"variant-id": "string", "location-id": "string",
	"set": "int", "adjust": "int",
}

func TestStockExecute_BothFlagsErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "set": "10", "adjust": "5",
	}, false)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Error("expected error when both --set and --adjust are provided")
	}
}

func TestStockExecute_NeitherFlagErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1",
	}, false)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Error("expected error when neither --set nor --adjust is provided")
	}
}

func TestStockExecute_SetNegativeErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "set": "-1",
	}, false)
	_, _ = stockShortcut.Execute(context.Background(), in)
}

func TestStockExecute_AdjustZeroErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "adjust": "0",
	}, false)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Error("expected error when --adjust is 0 (API rejects ≤ 0)")
	}
}

func TestStockExecute_AdjustDryRun_NoLocation(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "adjust": "5",
	}, true)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) < 2 {
		t.Errorf("expected ≥2 plans, got %d", len(result.Plans))
	}
}

func TestStockExecute_AdjustDryRun_WithLocation(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "adjust": "3", "location-id": "loc-1",
	}, true)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) < 2 {
		t.Errorf("expected ≥2 plans, got %d", len(result.Plans))
	}
}

func TestStockExecute_SetDryRun_NoLocation(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "set": "10",
	}, true)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) < 3 {
		t.Errorf("expected ≥3 plans, got %d", len(result.Plans))
	}
}

// ── setPriceShortcut.Execute ──────────────────────────────────────────────────

var setPriceExecFlags = map[string]string{
	"variant-id": "string", "sku": "string", "all": "bool",
	"price": "string", "compare-price": "string",
}

func setPriceInputWithClient(t *testing.T, values map[string]string, baseURL string) common.ExecInput {
	in := newProductExecInput(t, setPriceExecFlags, values, false)
	in.Client = client.New(baseURL)
	return in
}

// validation ----------------------------------------------------------------

func TestSetPriceExecute_NeitherSelectorErrors(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"price": "9.99"}, false)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when neither --variant-id nor --sku given")
	}
}

func TestSetPriceExecute_AllWithVariantIDErrors(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{
		"variant-id": "v-1", "all": "true", "price": "9.99",
	}, false)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when --all is combined with --variant-id")
	}
}

func TestSetPriceExecute_InvalidPriceErrors(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"sku": "SKU-1", "price": "notanumber"}, false)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error for non-numeric --price")
	}
}

func TestSetPriceExecute_NegativePriceErrors(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"variant-id": "v-1", "price": "-1"}, false)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error for negative --price")
	}
}

func TestSetPriceExecute_InvalidComparePriceErrors(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"variant-id": "v-1", "price": "9.99", "compare-price": "x"}, false)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error for non-numeric --compare-price")
	}
}

// dry-run routing -----------------------------------------------------------

func TestSetPriceExecute_DryRun_VariantIDOnly(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"variant-id": "v-1", "price": "9.99"}, true)
	r, err := setPriceShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || r.Plans[0].Method != "PUT" || !strings.HasSuffix(r.Plans[0].Path, "/variants/v-1") {
		t.Errorf("variant-id path: got %+v", r.Plans)
	}
}

func TestSetPriceExecute_DryRun_SKUOnly(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"sku": "SKU-1", "price": "9.99"}, true)
	r, err := setPriceShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 || r.Plans[0].Method != "GET" || !strings.HasSuffix(r.Plans[0].Path, "/products/sku/SKU-1/variants") {
		t.Errorf("sku path: got %+v", r.Plans)
	}
}

func TestSetPriceExecute_DryRun_SKUAll(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"sku": "SKU-1", "all": "true", "price": "9.99"}, true)
	r, err := setPriceShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || r.Plans[0].Method != "PUT" || !strings.HasSuffix(r.Plans[0].Path, "/variants/sku/SKU-1") {
		t.Fatalf("sku --all path: got %+v", r.Plans)
	}
	body, _ := r.Plans[0].Body.(map[string]any)
	if body["refuse_multi_result"] != false {
		t.Errorf("--all body must set refuse_multi_result=false; got %v", r.Plans[0].Body)
	}
}

func TestSetPriceExecute_DryRun_Both(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"variant-id": "v-1", "sku": "SKU-1", "price": "9.99"}, true)
	r, err := setPriceShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 || r.Plans[0].Method != "GET" || !strings.HasSuffix(r.Plans[0].Path, "/variants/v-1") || r.Plans[1].Method != "PUT" {
		t.Errorf("both path: got %+v", r.Plans)
	}
}

// live cross-check / resolution --------------------------------------------

func TestSetPriceExecute_BothSKUMismatchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"variant": map[string]any{"id": "v-1", "sku": "REAL"}})
	}))
	defer srv.Close()
	in := setPriceInputWithClient(t, map[string]string{"variant-id": "v-1", "sku": "WRONG", "price": "9.99"}, srv.URL)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Fatal("expected error when --sku does not match the variant's actual SKU")
	}
}

func TestSetPriceExecute_BothSKUMatchUpdates(t *testing.T) {
	var putCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putCalled = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"variant": map[string]any{"id": "v-1", "sku": "MATCH"}})
	}))
	defer srv.Close()
	in := setPriceInputWithClient(t, map[string]string{"variant-id": "v-1", "sku": "MATCH", "price": "9.99"}, srv.URL)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !putCalled {
		t.Error("expected the update PUT to be sent when the SKU matches")
	}
}

func TestSetPriceExecute_SKUMultiMatchRefuses(t *testing.T) {
	var putCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putCalled = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"variants": []any{
			map[string]any{"id": "v-1", "sku": "DUP"},
			map[string]any{"id": "v-2", "sku": "DUP"},
		}})
	}))
	defer srv.Close()
	in := setPriceInputWithClient(t, map[string]string{"sku": "DUP", "price": "9.99"}, srv.URL)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Fatal("expected refuse error on multi-match")
	}
	if putCalled {
		t.Error("must NOT update when the SKU matches multiple variants")
	}
}

// ── tagShortcut.Execute ───────────────────────────────────────────────────────

var tagExecFlags = map[string]string{
	"id": "string", "add": "stringslice", "remove": "stringslice", "set": "stringslice",
}

func tagInputWithClient(t *testing.T, values map[string]string, baseURL string) common.ExecInput {
	in := newProductExecInput(t, tagExecFlags, values, false)
	in.Client = client.New(baseURL)
	return in
}

// putTagsRecorder serves GET with the given current tags and records the tags
// array sent by any PUT. The bool reports whether a PUT was received.
func putTagsRecorder(t *testing.T, current []any, gotPut *bool, putTags *[]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			*gotPut = true
			var body struct {
				Product struct {
					Tags []any `json:"tags"`
				} `json:"product"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*putTags = body.Product.Tags
			_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": "p-1", "tags": body.Product.Tags}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": "p-1", "tags": current}})
	}))
}

func TestTagExecute_NoMutationFlagErrors(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1"}, false)
	if _, err := tagShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when none of --add/--remove/--set is given")
	}
}

func TestTagExecute_SetWithAddErrors(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1", "set": "a", "add": "b"}, false)
	if _, err := tagShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when --set is combined with --add")
	}
}

func TestTagExecute_DryRun_Set(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1", "set": "x,y,x"}, true)
	r, err := tagShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || r.Plans[0].Method != "PUT" || !strings.HasSuffix(r.Plans[0].Path, "/products/p-1") {
		t.Fatalf("set should be a single PUT; got %+v", r.Plans)
	}
	body, _ := r.Plans[0].Body.(map[string]any)
	prod, _ := body["product"].(map[string]any)
	if tags, _ := prod["tags"].([]string); !reflect.DeepEqual(tags, []string{"x", "y"}) {
		t.Errorf("set body tags should be deduped [x y]; got %v", prod["tags"])
	}
}

func TestTagExecute_DryRun_AddRemove(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1", "add": "c"}, true)
	r, err := tagShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 || r.Plans[0].Method != "GET" || !strings.HasSuffix(r.Plans[0].Path, "/products/p-1") || r.Plans[1].Method != "PUT" {
		t.Errorf("add/remove should be GET then PUT; got %+v", r.Plans)
	}
}

func TestTagExecute_Set_NoGet(t *testing.T) {
	var gotGet bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gotGet = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": "p-1", "tags": []any{"a"}}})
	}))
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "set": "x,y"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotGet {
		t.Error("--set must replace without a GET (no read-merge needed)")
	}
}

func TestTagExecute_AddMergesWithExisting(t *testing.T) {
	var gotPut bool
	var putTags []any
	srv := putTagsRecorder(t, []any{"a", "b"}, &gotPut, &putTags)
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "add": "c"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotPut {
		t.Fatal("expected a PUT after merging")
	}
	if !reflect.DeepEqual(putTags, []any{"a", "b", "c"}) {
		t.Errorf("PUT tags should keep existing and append; got %v", putTags)
	}
}

func TestTagExecute_RemoveDropsTag(t *testing.T) {
	var gotPut bool
	var putTags []any
	srv := putTagsRecorder(t, []any{"a", "b", "c"}, &gotPut, &putTags)
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "remove": "b"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(putTags, []any{"a", "c"}) {
		t.Errorf("PUT tags should drop removed tag; got %v", putTags)
	}
}

func TestTagExecute_NoChangeSkipsPut(t *testing.T) {
	var gotPut bool
	var putTags []any
	srv := putTagsRecorder(t, []any{"a", "b"}, &gotPut, &putTags)
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "add": "b"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPut {
		t.Error("adding an already-present tag must not trigger a PUT")
	}
}
