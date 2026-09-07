package products

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestStockShortcut_DeclarativeShape(t *testing.T) {
	if stockShortcut.Execute == nil {
		t.Fatal("+stock requires Execute (multi-step)")
	}
	if err := common.ValidateShortcut(stockShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
	// No flag may be Required: the target is one of three selectors, so cobra
	// must not reject the command before Execute can say which are accepted.
	for _, f := range stockShortcut.Flags {
		if f.Required {
			t.Errorf("--%s must not be Required; the target check belongs in Execute", f.Name)
		}
	}
}

func TestExtractInventoryItemID_OK(t *testing.T) {
	resp := map[string]any{
		"variant_inventory_items": []any{
			map[string]any{"inventory_item_id": "ii-1", "variant_id": "v-1"},
		},
	}
	got, err := extractInventoryItemID(resp)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "ii-1" {
		t.Errorf("got %q want ii-1", got)
	}
}

func TestExtractInventoryItemID_Empty(t *testing.T) {
	resp := map[string]any{"variant_inventory_items": []any{}}
	_, err := extractInventoryItemID(resp)
	if err == nil {
		t.Fatal("expected error on empty variant_inventory_items")
	}
	// An id that matches no variant is bad input, not a CLI bug.
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if ee.Code != output.ExitValidation {
		t.Errorf("exit %d want %d (validation)", ee.Code, output.ExitValidation)
	}
}

func TestExtractDefaultLocationID_NumericID(t *testing.T) {
	resp := map[string]any{"location": map[string]any{"id": float64(588599777604678400)}}
	got, err := extractDefaultLocationID(resp)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "588599777604678400" {
		t.Errorf("got %q want 588599777604678400", got)
	}
}

func TestExtractDefaultLocationID_OK(t *testing.T) {
	resp := map[string]any{"location": map[string]any{"id": "loc-1"}}
	got, err := extractDefaultLocationID(resp)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "loc-1" {
		t.Errorf("got %q want loc-1", got)
	}
}

func TestTranslateAdjustError_NegativeStockTranslated(t *testing.T) {
	httpErr := &client.HTTPError{
		StatusCode: 422,
		Body:       `{"error":"insufficient_stock","current_stock":5}`,
	}
	got := translateAdjustError(httpErr)
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitValidation {
		t.Errorf("Code: got %v want ExitValidation", exit.Code)
	}
	if !strings.Contains(exit.Error(), "5") {
		t.Errorf("error string should include current_stock=5; got: %q", exit.Error())
	}
}

func TestPlaceholderOr(t *testing.T) {
	if got := placeholderOr("real", "<ph>"); got != "real" {
		t.Errorf("non-empty: got %q want real", got)
	}
	if got := placeholderOr("", "<ph>"); got != "<ph>" {
		t.Errorf("empty: got %q want <ph>", got)
	}
}

func TestLevelRowFor_MatchAndCount(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"location_id": "loc-1", "stock": float64(42)},
			map[string]any{"location_id": "loc-2", "stock": float64(7)},
		},
	}
	row, n, err := levelRowFor(resp, "loc-2")
	if err != nil || n != 2 {
		t.Fatalf("got (n=%d, %v) want (2, nil)", n, err)
	}
	if row == nil || asString(row["location_id"]) != "loc-2" {
		t.Errorf("row: got %v want loc-2", row)
	}
}

func TestLevelRowFor_NoMatch(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"location_id": "loc-1", "stock": float64(42)},
		},
	}
	row, n, err := levelRowFor(resp, "loc-9")
	if err != nil || n != 1 || row != nil {
		t.Errorf("got (row=%v, n=%d, %v) want (nil, 1, nil)", row, n, err)
	}
}

func TestLevelRowFor_NumericLocationID(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"location_id": json.Number("583169022443404898"), "stock": float64(3)},
		},
	}
	row, _, err := levelRowFor(resp, "583169022443404898")
	if err != nil || row == nil {
		t.Errorf("numeric location_id should match its decimal string; got (%v, %v)", row, err)
	}
}

func TestLevelRowFor_MissingKey(t *testing.T) {
	if _, _, err := levelRowFor(map[string]any{}, "loc-1"); err == nil {
		t.Error("expected error when inventory_levels key missing")
	}
}

func TestStockOf_PresentMissingBad(t *testing.T) {
	if n, err := stockOf(map[string]any{"stock": float64(42)}); err != nil || n != 42 {
		t.Errorf("present: got (%d, %v) want (42, nil)", n, err)
	}
	if n, err := stockOf(map[string]any{"other": "x"}); err != nil || n != 0 {
		t.Errorf("missing: got (%d, %v) want (0, nil)", n, err)
	}
	if _, err := stockOf(map[string]any{"stock": "not-a-number"}); err == nil {
		t.Error("expected error when stock has unexpected type")
	}
}

func TestWrapLevelRow(t *testing.T) {
	got := wrapLevelRow(map[string]any{"id": "il-1"})
	wrapped, ok := got["inventory_level"].(map[string]any)
	if !ok || wrapped["id"] != "il-1" {
		t.Errorf("got %v want wrapped il-1", got)
	}
	got = wrapLevelRow(nil)
	wrapped, ok = got["inventory_level"].(map[string]any)
	if !ok || len(wrapped) != 0 {
		t.Errorf("nil row: expected empty map, got %v", got["inventory_level"])
	}
}

func TestAsInt_AllTypes(t *testing.T) {
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{json.Number("7"), 7, true},
		{json.Number("3.5"), 0, false},
		{float64(9), 9, true},
		{int(4), 4, true},
		{int64(11), 11, true},
		{"str", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := asInt(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("asInt(%v) = (%d, %t), want (%d, %t)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestAsString_AllTypes(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"hello", "hello"},
		{json.Number("123"), "123"},
		{float64(5), "5"},
		{float64(3.14), "3.14"},
		{int(7), "7"},
		{int64(99), "99"},
		{nil, ""},
		{true, ""},
	}
	for _, c := range cases {
		if got := asString(c.in); got != c.want {
			t.Errorf("asString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── extractDefaultLocationID ──────────────────────────────────────────────────

func TestExtractDefaultLocationID_Valid(t *testing.T) {
	resp := map[string]any{"location": map[string]any{"id": "loc-1"}}
	got, err := extractDefaultLocationID(resp)
	if err != nil || got != "loc-1" {
		t.Errorf("got (%q, %v) want (loc-1, nil)", got, err)
	}
}

func TestExtractDefaultLocationID_MissingLocation(t *testing.T) {
	resp := map[string]any{"other": "value"}
	_, err := extractDefaultLocationID(resp)
	if err == nil {
		t.Error("expected error for missing 'location' object")
	}
}

func TestExtractDefaultLocationID_MissingID(t *testing.T) {
	resp := map[string]any{"location": map[string]any{"name": "main"}}
	_, err := extractDefaultLocationID(resp)
	if err == nil {
		t.Error("expected error for missing location.id")
	}
}

// ── translateAdjustError ──────────────────────────────────────────────────────

func TestTranslateAdjustError_NonHTTP(t *testing.T) {
	orig := errors.New("network failure")
	got := translateAdjustError(orig)
	if got != orig {
		t.Errorf("non-HTTP should pass through; got %T", got)
	}
}

func TestTranslateAdjustError_Non422(t *testing.T) {
	orig := &client.HTTPError{StatusCode: 500, Body: "server error"}
	got := translateAdjustError(orig)
	if got != orig {
		t.Errorf("non-422 should pass through; got %T", got)
	}
}

func TestTranslateAdjustError_422WithCurrentStock(t *testing.T) {
	orig := &client.HTTPError{StatusCode: 422, Body: `{"current_stock":0}`}
	got := translateAdjustError(orig)
	if got == orig {
		t.Error("422 with current_stock should be reclassified")
	}
}

func TestTranslateAdjustError_422Generic(t *testing.T) {
	orig := &client.HTTPError{StatusCode: 422, Body: `{"error":"invalid"}`}
	got := translateAdjustError(orig)
	if got == orig {
		t.Error("generic 422 should be reclassified")
	}
}

func TestExtractInventoryItemID_NonMapItem(t *testing.T) {
	resp := map[string]any{"variant_inventory_items": []any{"not-a-map"}}
	_, err := extractInventoryItemID(resp)
	if err == nil {
		t.Error("expected error when first item is not a map")
	}
}

func TestExtractInventoryItemID_EmptyID(t *testing.T) {
	resp := map[string]any{"variant_inventory_items": []any{
		map[string]any{"inventory_item_id": ""},
	}}
	_, err := extractInventoryItemID(resp)
	if err == nil {
		t.Error("expected error when inventory_item_id is empty")
	}
}

func stockInputWithClient(t *testing.T, values map[string]string, baseURL string) common.ExecInput {
	in := newProductExecInput(t, stockExecFlags, values, false)
	in.Client = client.New(baseURL)
	return in
}

func stockBrokerServer(t *testing.T, opts stockBrokerOpts, calls *[]string) *httptest.Server {
	t.Helper()
	variantPut := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		*calls = append(*calls, r.Method+" "+path)
		switch {
		case strings.HasSuffix(path, "/inventory_items/variant"):
			json.NewEncoder(w).Encode(map[string]any{"variant_inventory_items": []any{
				map[string]any{"inventory_item_id": "ii-1", "variant_id": "v-1"},
			}})
		case strings.HasSuffix(path, "/locations/default"):
			json.NewEncoder(w).Encode(map[string]any{"location": map[string]any{"id": opts.defaultLoc}})
		case strings.HasSuffix(path, "/inventory_levels") && r.Method == http.MethodGet:
			rows := opts.levels
			if variantPut {
				rows = []map[string]any{{"location_id": opts.defaultLoc, "stock": opts.afterStock}}
			}
			json.NewEncoder(w).Encode(map[string]any{"inventory_levels": rows})
		case strings.HasSuffix(path, "/inventory_levels") && r.Method == http.MethodPut:
			json.NewEncoder(w).Encode(map[string]any{"inventory_level": map[string]any{"location_id": opts.defaultLoc}})
		case strings.Contains(path, "/variants/") && r.Method == http.MethodPut:
			variantPut = true
			json.NewEncoder(w).Encode(map[string]any{"variant": map[string]any{"id": "v-1"}})
		default:
			t.Errorf("unexpected call: %s %s", r.Method, path)
			http.NotFound(w, r)
		}
	}))
}

// stockBrokerServer mocks the four endpoints the slow path touches.
type stockBrokerOpts struct {
	defaultLoc string
	levels     []map[string]any // rows returned by GET /inventory_levels
	afterStock int              // stock reported on the re-read after a variant PUT
}

// planQueryString renders a plan's query values for substring assertions.
// Not json.Marshal: it HTML-escapes the < > in the step placeholders.
func planQueryString(t *testing.T, p common.PlannedRequest) string {
	t.Helper()
	return fmt.Sprintf("%v", p.Query)
}

var stockExecFlags = map[string]string{
	"variant-id": "string", "sku": "string", "product-id": "string",
	"location-id": "string",
	"set":         "int", "adjust": "int",
}

func TestStockExecute_NoTargetErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{"adjust": "5"}, false)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatal("expected error when no target selector is given")
	}
	if !strings.Contains(err.Error(), "--product-id") {
		t.Errorf("the error should name every accepted selector; got %v", err)
	}
}

func TestStockExecute_TwoTargetsErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "product-id": "p-1", "adjust": "5",
	}, false)
	if _, err := stockShortcut.Execute(context.Background(), in); err == nil {
		t.Fatal("expected error when two selectors are given")
	}
}

// A bad amount must fail before the target costs a network round trip.
func TestStockExecute_AmountGateRunsBeforeResolution(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"product-id": "p-1", "adjust": "0",
	}, false)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "--adjust") {
		t.Fatalf("expected the --adjust gate to fire without resolving; got %v", err)
	}
}

func TestStockExecute_ProductIDDryRun_PrependsResolveStep(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"product-id": "p-1", "adjust": "5",
	}, true)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) < 3 {
		t.Fatalf("expected ≥3 plans (resolve + item + write), got %d", len(result.Plans))
	}
	if result.Plans[0].Method != "GET" || !strings.HasSuffix(result.Plans[0].Path, "/products/p-1/variants") {
		t.Errorf("plan 0 should be the product-scoped variant list; got %+v", result.Plans[0])
	}
	// The item lookup consumes the variant id the resolve step produces.
	if !strings.Contains(planQueryString(t, result.Plans[1]), stepRef(0)) {
		t.Errorf("plan 1 should reference %s; got %+v", stepRef(0), result.Plans[1])
	}
}

// The renumbering regression: prepending a resolve step must shift every
// downstream <resolved-from-step-N>, and must NOT shift them when absent.
func TestStockExecute_PlaceholderIndicesShiftWithResolveStep(t *testing.T) {
	direct := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "set": "10",
	}, true)
	res, err := stockShortcut.Execute(context.Background(), direct)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Plans: item lookup, default location, levels(step-0), writes.
	if got := planQueryString(t, res.Plans[2]); !strings.Contains(got, stepRef(0)) {
		t.Errorf("--variant-id path: levels plan should reference %s; got %s", stepRef(0), got)
	}

	viaSKU := newProductExecInput(t, stockExecFlags, map[string]string{
		"sku": "TEE-RED-L", "set": "10",
	}, true)
	res, err = stockShortcut.Execute(context.Background(), viaSKU)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Plans: resolve, item lookup, default location, levels(step-1), writes.
	got := planQueryString(t, res.Plans[3])
	if !strings.Contains(got, stepRef(1)) {
		t.Errorf("--sku path: levels plan should reference %s; got %s", stepRef(1), got)
	}
	if strings.Contains(got, stepRef(0)) {
		t.Errorf("--sku path: %s is the resolve step, not the inventory item; got %s", stepRef(0), got)
	}
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
	if _, err := stockShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when --set is negative")
	}
}

func TestStockExecute_AdjustZeroErrors(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "adjust": "0",
	}, false)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Error("expected error when --adjust is 0")
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

func TestStockExecute_AdjustNegativeDryRun_PreviewsVariantPut(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "adjust": "-3",
	}, true)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	last := result.Plans[len(result.Plans)-1]
	if last.Method != "PUT" || !strings.HasSuffix(last.Path, "/variants/v-1") {
		t.Errorf("decrement preview should end on PUT /variants/v-1, got %+v", last)
	}
}

func TestStockExecute_SetLower_DecrementsViaVariantPut(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc: "loc-1",
		levels:     []map[string]any{{"location_id": "loc-1", "stock": 10}},
		afterStock: 4,
	}, &calls)
	defer srv.Close()

	in := stockInputWithClient(t, map[string]string{"variant-id": "v-1", "set": "4"}, srv.URL)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	sawVariantPut := false
	for _, c := range calls {
		if strings.HasPrefix(c, "PUT") && strings.Contains(c, "/variants/v-1") {
			sawVariantPut = true
		}
	}
	if !sawVariantPut {
		t.Errorf("expected PUT /variants/v-1, calls: %v", calls)
	}
	level, _ := result.Body["inventory_level"].(map[string]any)
	if got, _ := asInt(level["stock"]); got != 4 {
		t.Errorf("returned stock: got %v want 4", level["stock"])
	}
}

func TestStockExecute_AdjustNegative_BelowZeroErrors(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc: "loc-1",
		levels:     []map[string]any{{"location_id": "loc-1", "stock": 5}},
	}, &calls)
	defer srv.Close()

	in := stockInputWithClient(t, map[string]string{"variant-id": "v-1", "adjust": "-8"}, srv.URL)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "below 0") {
		t.Errorf("expected below-0 validation error, got %v", err)
	}
}

func TestStockExecute_Decrement_NonDefaultLocationErrors(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc: "loc-1",
		levels:     []map[string]any{{"location_id": "loc-2", "stock": 9}},
	}, &calls)
	defer srv.Close()

	in := stockInputWithClient(t, map[string]string{"variant-id": "v-1", "adjust": "-3", "location-id": "loc-2"}, srv.URL)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "default location") {
		t.Errorf("expected default-location gate, got %v", err)
	}
}

func TestStockExecute_Decrement_MultiLocationErrors(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc: "loc-1",
		levels: []map[string]any{
			{"location_id": "loc-1", "stock": 9},
			{"location_id": "loc-2", "stock": 3},
		},
	}, &calls)
	defer srv.Close()

	in := stockInputWithClient(t, map[string]string{"variant-id": "v-1", "adjust": "-3"}, srv.URL)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "multiple locations") {
		t.Errorf("expected multi-location gate, got %v", err)
	}
}

func TestStockExecute_SetHigher_AddsViaInventoryLevels(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc: "loc-1",
		levels:     []map[string]any{{"location_id": "loc-1", "stock": 4}},
	}, &calls)
	defer srv.Close()

	in := stockInputWithClient(t, map[string]string{"variant-id": "v-1", "set": "10"}, srv.URL)
	_, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "PUT") && strings.Contains(c, "/variants/") {
			t.Errorf("increase must not touch the variant endpoint, calls: %v", calls)
		}
	}
}

func TestStockExecute_SetEqual_NoWrite(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc: "loc-1",
		levels:     []map[string]any{{"location_id": "loc-1", "stock": 6}},
	}, &calls)
	defer srv.Close()

	in := stockInputWithClient(t, map[string]string{"variant-id": "v-1", "set": "6"}, srv.URL)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "PUT") {
			t.Errorf("no-op must not write, calls: %v", calls)
		}
	}
	if _, ok := result.Body["inventory_level"]; !ok {
		t.Errorf("no-op should still return an inventory_level, got %v", result.Body)
	}
}
