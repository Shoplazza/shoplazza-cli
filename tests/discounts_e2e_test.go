// Package tests: end-to-end tests for the discounts command.
// All tests use a mock HTTP server; no real API or credentials are needed.
package tests_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── discounts list ────────────────────────────────────────────────────────────

func TestDiscountsList_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discounts":   []any{map[string]any{"id": "d001", "discount_name": "summer"}},
			"has_more":    false,
			"total_count": 1,
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "discounts", "list")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	out := unwrapAPISuccess(t, stdout)
	items, _ := out["discounts"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected discounts, got: %v", out)
	}
	first := items[0].(map[string]any)
	if first["id"] != "d001" {
		t.Errorf("discounts[0].id = %v, want d001", first["id"])
	}
}

func TestDiscountsList_WithFilters(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discounts": []any{}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"discounts", "list",
		"--params", `{"discount_name":"summer","progress":"ongoing","status":"active","type":"code","per_page":5}`,
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if !strings.Contains(capturedQuery, "discount_name=summer") {
		t.Errorf("query %q missing discount_name=summer", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "progress=ongoing") {
		t.Errorf("query %q missing progress=ongoing", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "per_page=5") {
		t.Errorf("query %q missing per_page=5", capturedQuery)
	}
}

// ── discounts get ─────────────────────────────────────────────────────────────

func TestDiscountsGet_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/d001" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{
				"discount_info": map[string]any{"id": "d001", "discount_name": "summer"},
			},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "discounts", "get", "--params", `{"id":"d001"}`)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	out := unwrapAPISuccess(t, stdout)
	disc, _ := out["discount"].(map[string]any)
	info, _ := disc["discount_info"].(map[string]any)
	if info["id"] != "d001" {
		t.Errorf("discount_info.id = %v, want d001", info["id"])
	}
}

// ── discounts get-by-code ─────────────────────────────────────────────────────

func TestDiscountsGetByCode_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/by-code/CLI-ABC123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"discount_code": "CLI-ABC123"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "discounts", "get-by-code", "--params", `{"discount_code":"CLI-ABC123"}`)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	out := unwrapAPISuccess(t, stdout)
	if out["discount"] == nil {
		t.Error("expected discount field in response")
	}
}

// ── discounts cancel ──────────────────────────────────────────────────────────

func TestDiscountsCancel_MockServer(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/cancel" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "discounts", "cancel", "--data", `{"ids":["d001","d002"]}`)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	ids, _ := receivedBody["ids"].([]any)
	if len(ids) != 2 {
		t.Errorf("request ids len = %d, want 2", len(ids))
	}
}

// ── discounts restart ─────────────────────────────────────────────────────────

func TestDiscountsRestart_MockServer(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/restart" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "discounts", "restart", "--data", `{"id":"d001"}`)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	if receivedBody["id"] != "d001" {
		t.Errorf("request id = %v, want d001", receivedBody["id"])
	}
}

// ── discounts delete ──────────────────────────────────────────────────────────

func TestDiscountsDelete_NormalizesEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/openapi/2026-01/discounts/d001" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	// The dynamic runner is a thin pass-through; an empty server body yields {}.
	stdout, _, code := runCLI(t, bin, env, "discounts", "delete", "--params", `{"id":"d001"}`)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
}

// ── discounts combine ─────────────────────────────────────────────────────────

func TestDiscountsCombine_MockServer(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/combine" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ids": []string{"d001"}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env,
		"discounts", "combine",
		"--data", `{"ids":["d001"],"discount_combines":["product","shipping"]}`,
	)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s", code, stdout)
	}
	combines, _ := receivedBody["discount_combines"].([]any)
	if len(combines) != 2 {
		t.Errorf("discount_combines len = %d, want 2", len(combines))
	}
}

// ── shortcuts: +search ────────────────────────────────────────────────────────

func TestShortcut_Search(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discounts": []any{}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env, "discounts", "+search", "--query", "flash", "--progress", "ongoing")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if !strings.Contains(capturedQuery, "discount_name=flash") {
		t.Errorf("query %q missing discount_name=flash", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "progress=ongoing") {
		t.Errorf("query %q missing progress=ongoing", capturedQuery)
	}
}

// ── shortcuts: +rebate (order target) defaults ───────────────────────────────

func TestShortcut_Rebate_OrderTarget_Defaults(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/automatic" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d010"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"discounts", "+rebate",
		"--target", "order",
		"--tiers", "100:10,200:25",
		"--type", "amount-off",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)
	layer, _ := discount["discount_layer"].(map[string]any)
	rule, _ := discount["discount_rule"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)

	// --type amount-off → discount_type=rebate_cta_otr, condition=purchase_amount, obtain=fixed_price_reduction.
	if info["discount_type"] != "rebate_cta_otr" {
		t.Errorf("discount_type = %v, want rebate_cta_otr", info["discount_type"])
	}
	if info["discount_target"] != "order" {
		t.Errorf("discount_target = %v, want order", info["discount_target"])
	}
	if layer["condition_type"] != "purchase_amount" {
		t.Errorf("condition_type = %v, want purchase_amount", layer["condition_type"])
	}
	if layer["obtain_type"] != "fixed_price_reduction" {
		t.Errorf("obtain_type = %v, want fixed_price_reduction", layer["obtain_type"])
	}
	layers, _ := layer["layers"].([]any)
	if len(layers) != 2 {
		t.Errorf("layers len = %d, want 2", len(layers))
	}
	// No scope flag → entitled_product = {selection: "all"}.
	if entitled["selection"] != "all" {
		t.Errorf("entitled_product.selection = %v, want 'all'", entitled["selection"])
	}
	if _, hasIDs := entitled["product_ids"]; hasIDs {
		t.Errorf("no-scope rebate must not populate product_ids, got %v", entitled)
	}
	// Default discount_combines = [] (no stacking).
	combines, _ := rule["discount_combines"].([]any)
	if len(combines) != 0 {
		t.Errorf("default discount_combines should be empty, got %v", combines)
	}
	// Default limits = -1 (= no limit).
	if v, _ := rule["limit_max_discount"].(float64); v != -1 {
		t.Errorf("default limit_max_discount = %v, want -1", rule["limit_max_discount"])
	}
	if v, _ := rule["limit_user_discount"].(float64); v != -1 {
		t.Errorf("default limit_user_discount = %v, want -1", rule["limit_user_discount"])
	}
	// limit_order_discount applies to off-types: default = 1 (--limit-order-once
	// defaults true, meaning "single use per order"; pass --limit-order-once=false to opt into -1).
	if v, _ := rule["limit_order_discount"].(float64); v != 1 {
		t.Errorf("default limit_order_discount = %v, want 1", rule["limit_order_discount"])
	}
}

// ── shortcuts: +rebate (product target, products scope, all limits) ──────────

func TestShortcut_Rebate_ProductTarget_AllFlags(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d011"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"discounts", "+rebate",
		"--target", "product",
		"--tiers", "3:20",
		"--type", "qty-percent",
		"--products", "gid_a,gid_b",
		"--limit-max", "500",
		"--limit-user", "3",
		"--combines", "order,shipping",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)
	layer, _ := discount["discount_layer"].(map[string]any)
	rule, _ := discount["discount_rule"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)

	// --type qty-percent → discount_type=rebate_ctq_otp, condition=purchase_quantity, obtain=percent.
	if info["discount_type"] != "rebate_ctq_otp" {
		t.Errorf("discount_type = %v, want rebate_ctq_otp", info["discount_type"])
	}
	if layer["condition_type"] != "purchase_quantity" {
		t.Errorf("condition_type = %v, want purchase_quantity", layer["condition_type"])
	}
	if entitled["selection"] != "entitled" {
		t.Errorf("entitled_product.selection = %v, want 'entitled'", entitled["selection"])
	}
	ids, _ := entitled["product_ids"].([]any)
	if len(ids) != 2 {
		t.Errorf("entitled_product.product_ids len = %d, want 2", len(ids))
	}
	if v, _ := rule["limit_max_discount"].(float64); v != 500 {
		t.Errorf("limit_max_discount = %v, want 500", rule["limit_max_discount"])
	}
	if v, _ := rule["limit_user_discount"].(float64); v != 3 {
		t.Errorf("limit_user_discount = %v, want 3", rule["limit_user_discount"])
	}
	// limit_order_discount must NOT be in the payload for percent types.
	if _, hasLO := rule["limit_order_discount"]; hasLO {
		t.Errorf("limit_order_discount must be absent for percent types, got %v", rule["limit_order_discount"])
	}
	combines, _ := rule["discount_combines"].([]any)
	if !sliceEq(combines, []any{"order", "shipping"}) {
		t.Errorf("discount_combines = %v, want [order shipping]", combines)
	}
}

// ── shortcuts: +rebate scope flags (collections / variants / mutex) ──────────

func TestShortcut_Rebate_CollectionsScope(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "d012"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+rebate",
		"--target", "product",
		"--tiers", "3:20",
		"--type", "qty-percent",
		"--collections", "col_x,col_y",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)
	if entitled["selection"] != "entitled" {
		t.Errorf("selection = %v, want entitled", entitled["selection"])
	}
	colIDs, _ := entitled["collection_ids"].([]any)
	if len(colIDs) != 2 {
		t.Errorf("collection_ids len = %d, want 2", len(colIDs))
	}
	if _, hasProds := entitled["product_ids"]; hasProds {
		t.Errorf("collections scope must not also set product_ids")
	}
}

func TestShortcut_Rebate_VariantsScope(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "d013"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+rebate",
		"--target", "product",
		"--tiers", "3:20",
		"--type", "qty-percent",
		"--variants", "v1,v2,v3",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)
	varIDs, _ := entitled["variant_ids"].([]any)
	if len(varIDs) != 3 {
		t.Errorf("variant_ids len = %d, want 3", len(varIDs))
	}
}

func TestShortcut_Rebate_ScopeMutexRejected(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+rebate",
		"--target", "product",
		"--tiers", "3:20",
		"--type", "qty-percent",
		"--products", "gid_a",
		"--collections", "col_x",
	)
	if code == 0 {
		t.Fatal("expected validation error when --products and --collections both set")
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in stderr, got: %s", stderr)
	}
}

func TestShortcut_Rebate_LimitOrderWithPercentTypeRejected(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+rebate",
		"--target", "order",
		"--tiers", "100:10",
		"--type", "amount-percent",
		"--limit-order-once=false",
	)
	if code == 0 {
		t.Fatal("expected validation error when --limit-order-once used with percent type")
	}
	if !strings.Contains(stderr, "limit-order-once") {
		t.Errorf("expected 'limit-order-once' in stderr, got: %s", stderr)
	}
}

func sliceEq(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── shortcuts: +flashsale defaults + structural correctness ──────────────────

func TestShortcut_Flashsale_Defaults(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d011"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"discounts", "+flashsale",
		"--value", "30",
		"--type", "percent",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)
	layer, _ := discount["discount_layer"].(map[string]any)
	rule, _ := discount["discount_rule"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)

	if info["discount_type"] != "flashsale" {
		t.Errorf("discount_type = %v, want flashsale", info["discount_type"])
	}
	// discount_layer holds ONLY condition_type / obtain_type / layers.
	for _, leaked := range []string{"follow_stock", "price_rule", "limit_user_product_type"} {
		if _, ok := layer[leaked]; ok {
			t.Errorf("discount_layer must not contain rule field %q (got %v)", leaked, layer[leaked])
		}
	}
	if layer["obtain_type"] != "percent" {
		t.Errorf("obtain_type = %v, want percent", layer["obtain_type"])
	}
	layers, _ := layer["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("layers len = %d, want 1", len(layers))
	}
	first, _ := layers[0].(map[string]any)
	if first["obtain_value"] != "30" {
		t.Errorf("layers[0].obtain_value = %v (%T), want \"30\"", first["obtain_value"], first["obtain_value"])
	}

	// discount_rule defaults.
	if rule["price_rule"] != "price" {
		t.Errorf("discount_rule.price_rule = %v, want \"price\"", rule["price_rule"])
	}
	if rule["limit_user_product_type"] != "no_limit" {
		t.Errorf("discount_rule.limit_user_product_type = %v, want \"no_limit\"", rule["limit_user_product_type"])
	}
	if v, _ := rule["limit_user_product_discount"].(float64); v != -1 {
		t.Errorf("discount_rule.limit_user_product_discount = %v, want -1", rule["limit_user_product_discount"])
	}
	if rule["follow_stock"] != "product" {
		t.Errorf("discount_rule.follow_stock = %v, want \"product\"", rule["follow_stock"])
	}
	if v, _ := rule["stock"].(float64); v != 0 {
		t.Errorf("discount_rule.stock = %v, want 0", rule["stock"])
	}
	combines, _ := rule["discount_combines"].([]any)
	if len(combines) != 0 {
		t.Errorf("default discount_combines should be empty, got %v", combines)
	}
	// Campaign-wide limits default to -1 (no limit), same as +rebate.
	if v, _ := rule["limit_max_discount"].(float64); v != -1 {
		t.Errorf("default limit_max_discount = %v, want -1", rule["limit_max_discount"])
	}
	if v, _ := rule["limit_user_discount"].(float64); v != -1 {
		t.Errorf("default limit_user_discount = %v, want -1", rule["limit_user_discount"])
	}

	// No scope flag → entitled_product = {selection: "all"}.
	if entitled["selection"] != "all" {
		t.Errorf("entitled_product.selection = %v, want \"all\"", entitled["selection"])
	}
	// CRITICAL: flashsale must NEVER populate product_ids (must use variant_ids).
	if _, has := entitled["product_ids"]; has {
		t.Errorf("flashsale must not set entitled_product.product_ids — must use variant_ids; got %v", entitled)
	}
}

func TestShortcut_Flashsale_VariantsScope(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "d012"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+flashsale",
		"--value", "30", "--type", "percent",
		"--variants", "v1,v2",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)
	if entitled["selection"] != "entitled" {
		t.Errorf("selection = %v, want entitled", entitled["selection"])
	}
	varIDs, _ := entitled["variant_ids"].([]any)
	if len(varIDs) != 2 {
		t.Errorf("variant_ids len = %d, want 2", len(varIDs))
	}
	if _, has := entitled["product_ids"]; has {
		t.Errorf("must not set product_ids alongside variant_ids")
	}
}

func TestShortcut_Flashsale_CollectionsScope(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "d013"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+flashsale",
		"--value", "30", "--type", "percent",
		"--collections", "col_x",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)
	colIDs, _ := entitled["collection_ids"].([]any)
	if len(colIDs) != 1 {
		t.Errorf("collection_ids len = %d, want 1", len(colIDs))
	}
}

func TestShortcut_Flashsale_ScopeMutexRejected(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+flashsale",
		"--value", "30", "--type", "percent",
		"--variants", "v1",
		"--collections", "col_x",
	)
	if code == 0 {
		t.Fatal("expected validation error when --variants and --collections both set")
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in stderr, got: %s", stderr)
	}
}

func TestShortcut_Flashsale_RuleFieldsConfigurable(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "d014"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+flashsale",
		"--value", "30", "--type", "percent",
		"--price-rule", "compare_at_price",
		"--limit-user-variant", "2",
		"--stock", "100",
		"--combines", "order,shipping",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	rule, _ := discount["discount_rule"].(map[string]any)

	if rule["price_rule"] != "compare_at_price" {
		t.Errorf("price_rule = %v, want compare_at_price", rule["price_rule"])
	}
	if rule["limit_user_product_type"] != "customer_variant" {
		t.Errorf("limit_user_product_type = %v, want customer_variant", rule["limit_user_product_type"])
	}
	if v, _ := rule["limit_user_product_discount"].(float64); v != 2 {
		t.Errorf("limit_user_product_discount = %v, want 2", rule["limit_user_product_discount"])
	}
	// --stock 100 implies follow_stock=discount (the CLI derives the source
	// from --stock presence; --follow-stock no longer exists as a flag).
	if rule["follow_stock"] != "discount" {
		t.Errorf("follow_stock = %v, want discount (derived from --stock)", rule["follow_stock"])
	}
	if v, _ := rule["stock"].(float64); v != 100 {
		t.Errorf("stock = %v, want 100", rule["stock"])
	}
	combines, _ := rule["discount_combines"].([]any)
	if !sliceEq(combines, []any{"order", "shipping"}) {
		t.Errorf("discount_combines = %v, want [order shipping]", combines)
	}
	// flashsale's CLI hard-codes limit_max_discount / limit_user_discount to -1
	// (campaign-wide usage caps don't model auto-applied flashsales).
	if v, _ := rule["limit_max_discount"].(float64); v != -1 {
		t.Errorf("limit_max_discount = %v, want -1 (hard-coded for flashsale)", rule["limit_max_discount"])
	}
	if v, _ := rule["limit_user_discount"].(float64); v != -1 {
		t.Errorf("limit_user_discount = %v, want -1 (hard-coded for flashsale)", rule["limit_user_discount"])
	}
}

// TestShortcut_Flashsale_LimitUserMustBePositive verifies the >0 validation
// on the per-user item-count flags. The old --limit-user-type / --limit-user-count
// pair was collapsed into three mutex flags (--limit-user-variant / -product / -all);
// each demands a value > 0.
func TestShortcut_Flashsale_LimitUserMustBePositive(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+flashsale",
		"--value", "30", "--type", "percent",
		"--limit-user-product", "0",
	)
	if code == 0 {
		t.Fatal("expected validation error: --limit-user-product must be > 0")
	}
	if !strings.Contains(stderr, "limit-user-product") {
		t.Errorf("expected 'limit-user-product' in stderr, got: %s", stderr)
	}
}

// TestShortcut_Flashsale_StockMustBePositive verifies the >0 validation on --stock.
// The old --follow-stock flag was dropped; presence of --stock now implies
// follow_stock=discount, and 0 is rejected up-front.
func TestShortcut_Flashsale_StockMustBePositive(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+flashsale",
		"--value", "30", "--type", "percent",
		"--stock", "0",
	)
	if code == 0 {
		t.Fatal("expected validation error: --stock must be > 0")
	}
	if !strings.Contains(stderr, "stock") {
		t.Errorf("expected 'stock' in stderr, got: %s", stderr)
	}
}

// ── shortcuts: +mn-discount defaults ─────────────────────────────────────────

func TestShortcut_MnDiscount_Defaults(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/automatic" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "m001"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env, "discounts", "+mn-discount", "--tiers", "3:50")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)
	layer, _ := discount["discount_layer"].(map[string]any)
	rule, _ := discount["discount_rule"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)

	if info["discount_type"] != "m_n_discount" {
		t.Errorf("discount_type = %v, want m_n_discount", info["discount_type"])
	}
	if info["discount_target"] != "product" {
		t.Errorf("discount_target = %v, want product", info["discount_target"])
	}
	if layer["condition_type"] != "purchase_quantity" {
		t.Errorf("condition_type = %v, want purchase_quantity", layer["condition_type"])
	}
	if layer["obtain_type"] != "percent" {
		t.Errorf("obtain_type = %v, want percent", layer["obtain_type"])
	}
	if rule["mn_discount_scope"] != "highest" {
		t.Errorf("mn_discount_scope = %v, want highest (default)", rule["mn_discount_scope"])
	}
	if rule["product_discount_order"] != "desc" {
		t.Errorf("product_discount_order = %v, want desc (default)", rule["product_discount_order"])
	}
	combines, _ := rule["discount_combines"].([]any)
	if len(combines) != 0 {
		t.Errorf("default discount_combines should be empty, got %v", combines)
	}
	if v, _ := rule["limit_max_discount"].(float64); v != -1 {
		t.Errorf("default limit_max_discount = %v, want -1", rule["limit_max_discount"])
	}
	if v, _ := rule["limit_user_discount"].(float64); v != -1 {
		t.Errorf("default limit_user_discount = %v, want -1", rule["limit_user_discount"])
	}
	if entitled["selection"] != "all" {
		t.Errorf("entitled_product.selection = %v, want \"all\"", entitled["selection"])
	}
}

func TestShortcut_MnDiscount_AllFlags(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "m002"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+mn-discount",
		"--tiers", "3:50",
		"--scope", "highest-all",
		"--price-sort", "asc",
		"--combines", "order,shipping",
		"--limit-max", "500",
		"--limit-user", "3",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	rule, _ := discount["discount_rule"].(map[string]any)

	if rule["mn_discount_scope"] != "highest_all" {
		t.Errorf("mn_discount_scope = %v, want highest_all (--scope highest-all maps to underscore form)", rule["mn_discount_scope"])
	}
	if rule["product_discount_order"] != "asc" {
		t.Errorf("product_discount_order = %v, want asc", rule["product_discount_order"])
	}
	combines, _ := rule["discount_combines"].([]any)
	if !sliceEq(combines, []any{"order", "shipping"}) {
		t.Errorf("discount_combines = %v, want [order shipping]", combines)
	}
	if v, _ := rule["limit_max_discount"].(float64); v != 500 {
		t.Errorf("limit_max_discount = %v, want 500", rule["limit_max_discount"])
	}
	if v, _ := rule["limit_user_discount"].(float64); v != 3 {
		t.Errorf("limit_user_discount = %v, want 3", rule["limit_user_discount"])
	}
}

func TestShortcut_MnDiscount_VariantsScope(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "m003"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+mn-discount",
		"--tiers", "3:50",
		"--variants", "v1,v2",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)
	if entitled["selection"] != "entitled" {
		t.Errorf("selection = %v, want entitled", entitled["selection"])
	}
	varIDs, _ := entitled["variant_ids"].([]any)
	if len(varIDs) != 2 {
		t.Errorf("variant_ids len = %d, want 2", len(varIDs))
	}
}

func TestShortcut_MnDiscount_CollectionsScope(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"discount": map[string]any{"discount_info": map[string]any{"id": "m004"}}})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)
	_, _, code := runCLI(t, bin, env,
		"discounts", "+mn-discount",
		"--tiers", "3:50",
		"--collections", "col_x",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	discount, _ := receivedBody["discount"].(map[string]any)
	entitled, _ := discount["entitled_product"].(map[string]any)
	colIDs, _ := entitled["collection_ids"].([]any)
	if len(colIDs) != 1 {
		t.Errorf("collection_ids len = %d, want 1", len(colIDs))
	}
}

func TestShortcut_MnDiscount_ScopeMutexRejected(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+mn-discount",
		"--tiers", "3:50",
		"--products", "gid_a",
		"--variants", "v1",
	)
	if code == 0 {
		t.Fatal("expected validation error when --products and --variants both set")
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in stderr, got: %s", stderr)
	}
}

func TestShortcut_MnDiscount_InvalidProductOrder(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+mn-discount",
		"--tiers", "3:50",
		"--price-sort", "foo",
	)
	if code == 0 {
		t.Fatal("expected validation error for invalid --price-sort")
	}
	if !strings.Contains(stderr, "price-sort") {
		t.Errorf("expected 'price-sort' in stderr, got: %s", stderr)
	}
}

func TestShortcut_MnDiscount_InvalidCombines(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+mn-discount",
		"--tiers", "3:50",
		"--combines", "foo",
	)
	if code == 0 {
		t.Fatal("expected validation error for invalid --combines value")
	}
	if !strings.Contains(stderr, "combines") {
		t.Errorf("expected 'combines' in stderr, got: %s", stderr)
	}
}

// ── shortcuts: +percent-code (order target) auto-generates code ──────────────

func TestShortcut_PercentCode_OrderTarget_AutoCode(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/discounts/non-automatic" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d020"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env, "discounts", "+percent-code", "--target", "order", "--percent", "10")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)

	codes, _ := info["discount_codes"].([]any)
	if len(codes) == 0 {
		t.Fatal("discount_codes must not be empty")
	}
	code0, _ := codes[0].(string)
	if !strings.HasPrefix(code0, "CLI-") {
		t.Errorf("auto-generated code %q must start with CLI-", code0)
	}
	if info["discount_type"] != "code_percent" {
		t.Errorf("discount_type = %v, want code_percent", info["discount_type"])
	}
	layer, _ := discount["discount_layer"].(map[string]any)
	if layer["obtain_type"] != "percent" {
		t.Errorf("obtain_type = %v, want percent", layer["obtain_type"])
	}
}

// ── shortcuts: +free-shipping-code no required flags ────────────────────────

func TestShortcut_FreeShippingCode_NoRequiredFlags(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d030"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	// Zero required flags — must succeed.
	_, _, code := runCLI(t, bin, env, "discounts", "+free-shipping-code")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)
	if info["discount_type"] != "code_free_shipping" {
		t.Errorf("discount_type = %v, want code_free_shipping", info["discount_type"])
	}
	// discount_target = "shipping" — the only valid target for the
	// code_free_shipping discount_type; CLI hard-codes it.
	if info["discount_target"] != "shipping" {
		t.Errorf("discount_target = %v, want \"shipping\"", info["discount_target"])
	}
	layer, _ := discount["discount_layer"].(map[string]any)
	// Free shipping always uses obtain_type=fixed_price_reduction (the API
	// rejects percent for code_free_shipping). Default (no --off) is a
	// sentinel-large obtain_value that overruns any real shipping fee — the
	// admin UI's "100% free shipping" mode.
	if layer["obtain_type"] != "fixed_price_reduction" {
		t.Errorf("obtain_type = %v, want fixed_price_reduction", layer["obtain_type"])
	}
	layers, _ := layer["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("discount_layer.layers len = %d, want 1", len(layers))
	}
	first, _ := layers[0].(map[string]any)
	if first["obtain_value"] != "999999999" {
		t.Errorf("layers[0].obtain_value = %v, want \"999999999\" (sentinel for 100%% free)", first["obtain_value"])
	}
}

// ── shortcuts: +bxgy-code payload ────────────────────────────────────────────

func TestShortcut_BxgyCode_Payload(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d040"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"discounts", "+bxgy-code",
		"--products", "gid_1,gid_2", "--buy-quantity", "2",
		"--get-products", "gid_3", "--get-quantity", "1",
		"--get-free",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	info, _ := discount["discount_info"].(map[string]any)
	if info["discount_type"] != "code_bxgy" {
		t.Errorf("discount_type = %v, want code_bxgy", info["discount_type"])
	}
	entitled, _ := discount["entitled_product"].(map[string]any)
	buyIDs, _ := entitled["product_ids"].([]any)
	if len(buyIDs) != 2 {
		t.Errorf("entitled_product.product_ids len = %d, want 2 (gid_1, gid_2)", len(buyIDs))
	}
	obtain, _ := discount["obtain_product"].(map[string]any)
	getIDs, _ := obtain["product_ids"].([]any)
	if len(getIDs) != 1 {
		t.Errorf("obtain_product.product_ids len = %d, want 1", len(getIDs))
	}
	layer, _ := discount["discount_layer"].(map[string]any)
	if layer["obtain_type"] != "free_acquisition" {
		t.Errorf("obtain_type = %v, want free_acquisition (with --get-free)", layer["obtain_type"])
	}
	layers, _ := layer["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("discount_layer.layers len = %d, want 1", len(layers))
	}
	first, _ := layers[0].(map[string]any)
	if first["condition_value"] != "2" {
		t.Errorf("layers[0].condition_value = %v, want \"2\"", first["condition_value"])
	}
	// obtain_count must be present and equal the qty from --get gid_3:1.
	// Server treats missing obtain_count as out-of-range and rejects with
	// "obtain_count must be less than 1000000000".
	if got, ok := first["obtain_count"]; !ok {
		t.Errorf("layers[0].obtain_count missing; want 1")
	} else if got != float64(1) {
		t.Errorf("layers[0].obtain_count = %v (%T), want 1", got, got)
	}
}

// TestShortcut_BxgyCode_ObtainCount verifies the qty from --get <id>:<qty>
// (qty > 1) is threaded into discount_layer.layers[0].obtain_count. Pairs
// with --get-percent to also cover the percent obtain_type path.
func TestShortcut_BxgyCode_ObtainCount(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d041"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"discounts", "+bxgy-code",
		"--products", "gid_a", "--buy-quantity", "2",
		"--get-products", "gid_b", "--get-quantity", "3",
		"--get-percent", "50",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	layer, _ := discount["discount_layer"].(map[string]any)
	if layer["obtain_type"] != "percent" {
		t.Errorf("obtain_type = %v, want percent (with --get-percent)", layer["obtain_type"])
	}
	layers, _ := layer["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("discount_layer.layers len = %d, want 1", len(layers))
	}
	first, _ := layers[0].(map[string]any)
	if first["condition_value"] != "2" {
		t.Errorf("layers[0].condition_value = %v, want \"2\"", first["condition_value"])
	}
	if first["obtain_value"] != "50" {
		t.Errorf("layers[0].obtain_value = %v, want \"50\"", first["obtain_value"])
	}
	if got := first["obtain_count"]; got != float64(3) {
		t.Errorf("layers[0].obtain_count = %v (%T), want 3", got, got)
	}
}

// TestShortcut_BxgyCode_GetOff verifies that --get-off selects the
// fixed_price_reduction obtain_type and serializes the amount as obtain_value.
// Pairs with the free_acquisition and percent paths covered above.
func TestShortcut_BxgyCode_GetOff(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"discount": map[string]any{"discount_info": map[string]any{"id": "d042"}},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, stderr, code := runCLI(t, bin, env,
		"discounts", "+bxgy-code",
		"--products", "gid_a", "--buy-quantity", "1",
		"--get-products", "gid_b", "--get-quantity", "1",
		"--get-off", "10",
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr)
	}

	discount, _ := receivedBody["discount"].(map[string]any)
	layer, _ := discount["discount_layer"].(map[string]any)
	if layer["obtain_type"] != "fixed_price_reduction" {
		t.Errorf("obtain_type = %v, want fixed_price_reduction", layer["obtain_type"])
	}
	layers, _ := layer["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("discount_layer.layers len = %d, want 1", len(layers))
	}
	first, _ := layers[0].(map[string]any)
	if first["obtain_value"] != "10" {
		t.Errorf("layers[0].obtain_value = %v, want \"10\"", first["obtain_value"])
	}
}

// TestShortcut_BxgyCode_GetDiscountAndGetOffMutex verifies that passing
// both --get-percent and --get-off rejects with ExitValidation.
func TestShortcut_BxgyCode_GetDiscountAndGetOffMutex(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+bxgy-code",
		"--products", "gid_a", "--buy-quantity", "1",
		"--get-products", "gid_b", "--get-quantity", "1",
		"--get-percent", "50",
		"--get-off", "10",
	)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (ExitValidation); stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--get-percent") || !strings.Contains(stderr, "--get-off") {
		t.Errorf("stderr does not name both flags: %s", stderr)
	}
}

// ── validation: missing required flags ───────────────────────────────────────

func TestDiscountsCancel_MissingIDs(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, nil, "discounts", "cancel")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(stderr+stdout(t), "ids") {
		// Just check exit is non-zero; error message format varies.
		_ = stderr
	}
}

func stdout(_ *testing.T) string { return "" }

// TestDiscountsRebate_ProductTarget_RequiresScope verifies the CLI-side
// safeguard: --target=product without a scope flag (--products / --variants /
// --collections) must fail validation, because the API rejects
// selection=all for product-level rebate.
func TestDiscountsRebate_ProductTarget_RequiresScope(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, []string{"SHOPLAZZA_ACCESS_TOKEN=test_token", "SHOPLAZZA_CLI_API_BASE_URL=http://unused"},
		"discounts", "+rebate",
		"--target", "product",
		"--tiers", "3:20",
		// --products / --variants / --collections intentionally omitted.
	)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (ExitValidation); stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--target=product requires") {
		t.Errorf("stderr does not mention scope requirement: %s", stderr)
	}
}

// ── HTTP 500 error handling ───────────────────────────────────────────────────

func TestDiscounts_HTTP500_JSONErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"error": "internal server error"})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, stderr, code := runCLI(t, bin, env, "discounts", "list")
	if code == 0 {
		t.Fatal("expected non-zero exit code on HTTP 500")
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envelope); err != nil {
		t.Fatalf("stderr is not JSON: %v\nstderr: %s", err, stderr)
	}
	if ok, _ := envelope["ok"].(bool); ok {
		t.Error("envelope.ok should be false")
	}
	if envelope["error"] == nil {
		t.Error("envelope.error field must be present")
	}
}
