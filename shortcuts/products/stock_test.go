package products

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestPlaceholderOr(t *testing.T) {
	if got := placeholderOr("real", "<ph>"); got != "real" {
		t.Errorf("non-empty: got %q want real", got)
	}
	if got := placeholderOr("", "<ph>"); got != "<ph>" {
		t.Errorf("empty: got %q want <ph>", got)
	}
}

func TestLevelRowFor_Match(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"location_id": "loc-1", "stock": float64(42)},
			map[string]any{"location_id": "loc-2", "stock": float64(7)},
		},
	}
	row, err := levelRowFor(resp, "loc-2")
	if err != nil {
		t.Fatalf("levelRowFor: %v", err)
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
	row, err := levelRowFor(resp, "loc-9")
	if err != nil || row != nil {
		t.Errorf("got (row=%v, %v) want (nil, nil)", row, err)
	}
}

func TestLevelRowFor_NumericLocationID(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"location_id": json.Number("583169022443404898"), "stock": float64(3)},
		},
	}
	row, err := levelRowFor(resp, "583169022443404898")
	if err != nil || row == nil {
		t.Errorf("numeric location_id should match its decimal string; got (%v, %v)", row, err)
	}
}

func TestLevelRowFor_MissingKey(t *testing.T) {
	if _, err := levelRowFor(map[string]any{}, "loc-1"); err == nil {
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
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		*calls = append(*calls, strings.TrimSpace(r.Method+" "+path+" "+string(raw)))
		switch {
		case strings.HasSuffix(path, "/inventory_items/variant"):
			json.NewEncoder(w).Encode(map[string]any{"variant_inventory_items": []any{
				map[string]any{"inventory_item_id": "ii-1", "variant_id": "v-1"},
			}})
		case strings.HasSuffix(path, "/inventory_items/ii-1") && r.Method == http.MethodGet:
			item := map[string]any{"id": "ii-1"}
			if !opts.untracked {
				item["tracking"] = true
			}
			json.NewEncoder(w).Encode(map[string]any{"inventory_item": item})
		case strings.HasSuffix(path, "/locations/default"):
			json.NewEncoder(w).Encode(map[string]any{"location": map[string]any{"id": opts.defaultLoc}})
		case strings.HasSuffix(path, "/inventory_levels") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"inventory_levels": opts.levels})
		case strings.HasSuffix(path, "/inventory_levels") && r.Method == http.MethodPut:
			if opts.adjustStatus != 0 {
				w.Header().Set("Request-Id", "req-422")
				w.WriteHeader(opts.adjustStatus)
				w.Write([]byte(`{"code":"Unprocessable","message":"rejected"}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"inventory_level": map[string]any{"location_id": opts.defaultLoc}})
		case strings.HasSuffix(path, "/inventory_levels/set") && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{"inventory_level": map[string]any{"location_id": opts.defaultLoc}})
		case strings.Contains(path, "/variants/") && r.Method == http.MethodPut:
			json.NewEncoder(w).Encode(map[string]any{"variant": map[string]any{"id": "v-1"}})
		default:
			t.Errorf("unexpected call: %s %s", r.Method, path)
			http.NotFound(w, r)
		}
	}))
}

// stockBrokerServer mocks the endpoints +stock touches.
type stockBrokerOpts struct {
	defaultLoc   string
	levels       []map[string]any // rows returned by GET /inventory_levels
	untracked    bool
	adjustStatus int // non-zero: PUT /inventory_levels fails with this status
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

// --set and --adjust are mutually exclusive, one is required, and an adjust
// of 0 is a no-op the command refuses rather than sends.
func TestStockExecute_SetAdjustRefusals(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
	}{
		{"both --set and --adjust", map[string]string{"variant-id": "v-1", "set": "10", "adjust": "5"}},
		{"neither --set nor --adjust", map[string]string{"variant-id": "v-1"}},
		{"--adjust 0", map[string]string{"variant-id": "v-1", "adjust": "0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newProductExecInput(t, stockExecFlags, c.flags, false)
			if _, err := stockShortcut.Execute(context.Background(), in); err == nil {
				t.Error("expected a refusal")
			}
		})
	}
}

// Dry-run previews the whole chain: a lookup plus the write. --set previews
// one more step than --adjust because it must read the current level first.
func TestStockExecute_DryRunPlanCount(t *testing.T) {
	cases := []struct {
		name      string
		flags     map[string]string
		wantPlans int
	}{
		{"adjust, no location", map[string]string{"variant-id": "v-1", "adjust": "5"}, 5},
		{"adjust, with location", map[string]string{"variant-id": "v-1", "adjust": "3", "location-id": "loc-1"}, 4},
		{"adjust negative, no location", map[string]string{"variant-id": "v-1", "adjust": "-3"}, 5},
		{"set, no location", map[string]string{"variant-id": "v-1", "set": "10"}, 6},
		{"set negative", map[string]string{"variant-id": "v-1", "set": "-1"}, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newProductExecInput(t, stockExecFlags, c.flags, true)
			result, err := stockShortcut.Execute(context.Background(), in)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(result.Plans) != c.wantPlans {
				t.Errorf("expected %d plans, got %d", c.wantPlans, len(result.Plans))
			}
			sawItemGet := false
			for _, p := range result.Plans {
				if p.Method == "GET" && strings.HasSuffix(p.Path, "/inventory_items/"+stepRef(0)) {
					sawItemGet = true
				}
				if strings.Contains(p.Path, "/variants/") {
					t.Errorf("dry-run should not preview a variant write, got %+v", p)
				}
			}
			if !sawItemGet {
				t.Errorf("dry-run should preview GET /inventory_items/%s, plans: %+v", stepRef(0), result.Plans)
			}
		})
	}
}

func TestStockExecute_AdjustNegativeDryRun_PreviewsSetStock(t *testing.T) {
	in := newProductExecInput(t, stockExecFlags, map[string]string{
		"variant-id": "v-1", "adjust": "-3",
	}, true)
	result, err := stockShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	last := result.Plans[len(result.Plans)-1]
	body, _ := last.Body.(map[string]any)
	if last.Method != "POST" || !strings.HasSuffix(last.Path, "/inventory_levels/set") || body["stock"] != -3 {
		t.Errorf("decrease preview should end on POST /inventory_levels/set with stock -3, got %+v", last)
	}
}

func TestStockExecute_DecreaseViaSetStock(t *testing.T) {
	cases := []struct {
		name   string
		flags  map[string]string
		levels []map[string]any
		want   string
	}{
		{"set lower", map[string]string{"set": "4"}, []map[string]any{{"location_id": "loc-1", "stock": 10}},
			`{"inventory_item_id":"ii-1","location_id":"loc-1","stock":-6}`},
		{"set negative", map[string]string{"set": "-3"}, []map[string]any{{"location_id": "loc-1", "stock": 2}},
			`{"inventory_item_id":"ii-1","location_id":"loc-1","stock":-5}`},
		{"multi-location", map[string]string{"set": "1"}, []map[string]any{{"location_id": "loc-1", "stock": 9}, {"location_id": "loc-2", "stock": 3}},
			`{"inventory_item_id":"ii-1","location_id":"loc-1","stock":-8}`},
		{"adjust below zero", map[string]string{"adjust": "-8"}, []map[string]any{{"location_id": "loc-1", "stock": 5}},
			`{"inventory_item_id":"ii-1","location_id":"loc-1","stock":-8}`},
		{"non-default location", map[string]string{"adjust": "-3", "location-id": "loc-2"}, []map[string]any{{"location_id": "loc-2", "stock": 9}},
			`{"inventory_item_id":"ii-1","location_id":"loc-2","stock":-3}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			srv := stockBrokerServer(t, stockBrokerOpts{defaultLoc: "loc-1", levels: c.levels}, &calls)
			defer srv.Close()

			c.flags["variant-id"] = "v-1"
			result, err := stockShortcut.Execute(context.Background(), stockInputWithClient(t, c.flags, srv.URL))
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			var writes []string
			for _, call := range calls {
				if !strings.HasPrefix(call, "GET") {
					writes = append(writes, call)
				}
			}
			if len(writes) != 1 || !strings.HasSuffix(writes[0], "/inventory_levels/set "+c.want) {
				t.Errorf("writes: got %q want one POST /inventory_levels/set %s", writes, c.want)
			}
			if _, ok := result.Body["inventory_level"]; !ok {
				t.Errorf("want the inventory_level response, got %v", result.Body)
			}
		})
	}
}

func TestStockExecute_APIErrorPassesThrough(t *testing.T) {
	var calls []string
	srv := stockBrokerServer(t, stockBrokerOpts{
		defaultLoc:   "loc-1",
		levels:       []map[string]any{{"location_id": "loc-1", "stock": 0}},
		adjustStatus: 422,
	}, &calls)
	defer srv.Close()

	_, err := stockShortcut.Execute(context.Background(), stockInputWithClient(t, map[string]string{"variant-id": "v-1", "adjust": "5"}, srv.URL))
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 422 || httpErr.RequestID != "req-422" {
		t.Errorf("want the 422 HTTPError with its request id, got %#v", err)
	}
}

func TestStockExecute_RefusedBeforeWrite(t *testing.T) {
	const untracked = "this variant's product does not track inventory, so its stock cannot be changed"
	const notStocked = "this variant is not stocked at location loc-2 (stocked at: loc-1)"
	cases := []struct {
		name      string
		flags     map[string]string
		untracked bool
		wantMsg   string
	}{
		{"untracked decrease", map[string]string{"set": "4"}, true, untracked},
		{"untracked add", map[string]string{"adjust": "2"}, true, untracked},
		{"not stocked, set", map[string]string{"set": "4", "location-id": "loc-2"}, false, notStocked},
		{"not stocked, add", map[string]string{"adjust": "2", "location-id": "loc-2"}, false, notStocked},
		{"not stocked, subtract", map[string]string{"adjust": "-2", "location-id": "loc-2"}, false, notStocked},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			srv := stockBrokerServer(t, stockBrokerOpts{
				defaultLoc: "loc-1",
				levels:     []map[string]any{{"location_id": "loc-1", "stock": 10}},
				untracked:  c.untracked,
			}, &calls)
			defer srv.Close()

			c.flags["variant-id"] = "v-1"
			_, err := stockShortcut.Execute(context.Background(), stockInputWithClient(t, c.flags, srv.URL))
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Message != c.wantMsg {
				t.Fatalf("want validation error %q, got %v", c.wantMsg, err)
			}
			if c.wantMsg == notStocked && (ee.Detail.Param != "--location-id" || !strings.Contains(ee.Detail.Hint, `create-level --data '{"inventory_item_id":"ii-1","location_id":"loc-2"}'`)) {
				t.Errorf("want --location-id and a create-level hint, got param=%q hint=%q", ee.Detail.Param, ee.Detail.Hint)
			}
			for _, call := range calls {
				if !strings.HasPrefix(call, "GET") {
					t.Errorf("no write should be sent, calls: %v", calls)
				}
			}
		})
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
