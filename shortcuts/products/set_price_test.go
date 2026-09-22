package products

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestSetPriceShortcut_ValidationFields(t *testing.T) {
	if setPriceShortcut.Service != "products" || setPriceShortcut.Command != "+set-price" {
		t.Errorf("identity: got %q/%q", setPriceShortcut.Service, setPriceShortcut.Command)
	}
	if setPriceShortcut.Execute == nil {
		t.Fatal("+set-price requires Execute (handles variant-id / sku / --all branching)")
	}
	if err := common.ValidateShortcut(setPriceShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

// Identifier-resolution tests live in resolve_test.go alongside the code.

func TestVariantSKU(t *testing.T) {
	resp := map[string]any{"variant": map[string]any{"id": "v-1", "sku": "ABC"}}
	if got := variantSKU(resp); got != "ABC" {
		t.Errorf("variantSKU = %q want ABC", got)
	}
	if got := variantSKU(map[string]any{}); got != "" {
		t.Errorf("variantSKU(empty) = %q want empty", got)
	}
}

func setPriceInputWithClient(t *testing.T, values map[string]string, baseURL string) common.ExecInput {
	in := newProductExecInput(t, setPriceExecFlags, values, false)
	in.Client = client.New(baseURL)
	return in
}

var setPriceExecFlags = map[string]string{
	"variant-id": "string", "sku": "string", "product-id": "string", "all": "bool",
	"price": "string", "compare-price": "string",
}

// Bad selector or price combinations are refused before any request goes out.
func TestSetPriceExecute_Refusals(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
	}{
		{"neither --variant-id nor --sku", map[string]string{"price": "9.99"}},
		{"--all with --variant-id", map[string]string{"variant-id": "v-1", "all": "true", "price": "9.99"}},
		{"non-numeric --price", map[string]string{"sku": "SKU-1", "price": "notanumber"}},
		{"negative --price", map[string]string{"variant-id": "v-1", "price": "-1"}},
		{"non-numeric --compare-price", map[string]string{"variant-id": "v-1", "price": "9.99", "compare-price": "x"}},
		{"--product-id with --sku", map[string]string{"product-id": "p-1", "sku": "S", "price": "9.99"}},
		{"--product-id with --all", map[string]string{"product-id": "p-1", "all": "true", "price": "9.99"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newProductExecInput(t, setPriceExecFlags, c.flags, false)
			if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
				t.Error("expected a refusal")
			}
		})
	}
}

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

func TestSetPriceExecute_DryRun_ProductIDOnly(t *testing.T) {
	in := newProductExecInput(t, setPriceExecFlags, map[string]string{"product-id": "p-1", "price": "9.99"}, true)
	r, err := setPriceShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 {
		t.Fatalf("expected resolve + update, got %d plans", len(r.Plans))
	}
	if r.Plans[0].Method != "GET" || !strings.HasSuffix(r.Plans[0].Path, "/products/p-1/variants") {
		t.Errorf("plan 0 should be the product-scoped variant list; got %+v", r.Plans[0])
	}
	if !strings.HasSuffix(r.Plans[1].Path, "/variants/"+stepRef(0)) {
		t.Errorf("plan 1 should target %s; got %+v", stepRef(0), r.Plans[1])
	}
}

func TestSetPriceExecute_ProductIDSingleVariantUpdates(t *testing.T) {
	var putPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{"variant": map[string]any{"id": "v-9"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"variants": []any{
			map[string]any{"id": "v-9", "sku": "ONLY"},
		}})
	}))
	defer srv.Close()
	in := setPriceInputWithClient(t, map[string]string{"product-id": "p-1", "price": "9.99"}, srv.URL)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasSuffix(putPath, "/variants/v-9") {
		t.Errorf("should update the product's only variant; PUT went to %q", putPath)
	}
}

func TestSetPriceExecute_ProductIDMultiVariantRefuses(t *testing.T) {
	var putCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putCalled = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"variants": []any{
			map[string]any{"id": "v-1", "option1": "S"},
			map[string]any{"id": "v-2", "option1": "M"},
		}})
	}))
	defer srv.Close()
	in := setPriceInputWithClient(t, map[string]string{"product-id": "p-1", "price": "9.99"}, srv.URL)
	if _, err := setPriceShortcut.Execute(context.Background(), in); err == nil {
		t.Fatal("expected refusal on a multi-variant product")
	}
	if putCalled {
		t.Error("must NOT reprice a guessed variant when the product has several")
	}
}
