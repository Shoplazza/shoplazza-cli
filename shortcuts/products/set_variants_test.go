package products

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/spf13/cobra"
)

// ── flag/plumbing helpers ─────────────────────────────────────────────────────

// newSetVariantsInput builds an ExecInput for setVariantsShortcut. options uses
// StringArray semantics (one element per --option occurrence, commas kept).
func newSetVariantsInput(t *testing.T, options []string, values map[string]string, dryRun bool) common.ExecInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.Flags().StringArray("option", nil, "")
	cmd.Flags().String("action", "", "")
	cmd.Flags().String("id", "", "")
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("price", "", "")
	cmd.Flags().String("sku-template", "", "")
	cmd.Flags().Int("stock", 0, "")
	cmd.Flags().Bool("published", false, "")
	var args []string
	for _, o := range options {
		args = append(args, "--option="+o)
	}
	for name, val := range values {
		args = append(args, "--"+name+"="+val)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	return common.ExecInput{Flags: common.NewCobraFlagSet(cmd), DryRun: dryRun}
}

// matrixServer serves GET /products/{id} from product and captures the PUT/POST
// body into captured.
func matrixServer(t *testing.T, product map[string]any, captured *map[string]any, putResponse map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"product": product})
		case http.MethodPut, http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(captured); err != nil {
				t.Errorf("decode captured body: %v", err)
			}
			resp := putResponse
			if resp == nil {
				resp = *captured
			}
			json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
}

func capturedVariants(t *testing.T, captured map[string]any) []map[string]any {
	t.Helper()
	prod, _ := captured["product"].(map[string]any)
	if prod == nil {
		t.Fatal("captured body has no product")
	}
	raw, _ := prod["variants"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, rv := range raw {
		m, ok := rv.(map[string]any)
		if !ok {
			t.Fatalf("variant is %T, want map", rv)
		}
		out = append(out, m)
	}
	return out
}

// variantByOptions finds the captured variant with the given option values ("" = absent).
func variantByOptions(t *testing.T, variants []map[string]any, o1, o2, o3 string) map[string]any {
	t.Helper()
	for _, v := range variants {
		get := func(k string) string { s, _ := v[k].(string); return s }
		if get("option1") == o1 && get("option2") == o2 && get("option3") == o3 {
			return v
		}
	}
	t.Fatalf("no variant with options (%q,%q,%q)", o1, o2, o3)
	return nil
}

// ── parsing ───────────────────────────────────────────────────────────────────

func TestParseOptionSpecs(t *testing.T) {
	specs, err := parseOptionSpecs([]string{"Color:Red, Blue", "Size:S,M,"}, "add")
	if err != nil {
		t.Fatal(err)
	}
	want := []optionSpec{
		{Name: "Color", Values: []string{"Red", "Blue"}},
		{Name: "Size", Values: []string{"S", "M"}},
	}
	if !reflect.DeepEqual(specs, want) {
		t.Errorf("got %+v, want %+v", specs, want)
	}
}

func TestParseOptionSpecs_BareName(t *testing.T) {
	specs, err := parseOptionSpecs([]string{"Size"}, "remove")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "Size" || specs[0].Values != nil {
		t.Errorf("bare name must parse to nil values under remove, got %+v", specs)
	}
	if _, err := parseOptionSpecs([]string{"Size"}, "add"); err == nil {
		t.Error("bare name must be rejected outside remove")
	}
}

// Values that differ only by case are distinct on the server.
func TestParseOptionSpecs_CaseOnlyValuesAreDistinct(t *testing.T) {
	specs, err := parseOptionSpecs([]string{"Size:S,s"}, "add")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(specs[0].Values, []string{"S", "s"}) {
		t.Errorf("got %v", specs[0].Values)
	}
}

func TestParseOptionSpecs_Errors(t *testing.T) {
	cases := []struct {
		name   string
		specs  []string
		action string
		want   string
	}{
		{"empty", nil, "add", "--option"},
		{"empty name", []string{":Red"}, "add", `"Name:v1,v2,..."`},
		{"no values", []string{"Color: ,"}, "add", "no values"},
		{"dup name", []string{"Color:Red", "color:Blue"}, "add", "duplicate option name"},
		{"dup value", []string{"Color:Red,Red"}, "add", "duplicate value"},
		{"four dims", []string{"A:1", "B:2", "C:3", "D:4"}, "add", "at most 3"},
		{"rename without pair", []string{"Color:Red"}, "rename", `"old=new"`},
		{"rename empty side", []string{"Color="}, "rename", "OldName=NewName"},
		{"rename twice", []string{"Color:Red=A,Red=B"}, "rename", "twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOptionSpecs(tc.specs, tc.action)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseOptionSpecs_Rename(t *testing.T) {
	specs, err := parseOptionSpecs([]string{"Color:Red=Wine Red, Blue=Navy", "Size=Fit"}, "rename")
	if err != nil {
		t.Fatal(err)
	}
	want := []optionSpec{
		{Name: "Color", Renames: [][2]string{{"Red", "Wine Red"}, {"Blue", "Navy"}}},
		{Name: "Size", NewName: "Fit"},
	}
	if !reflect.DeepEqual(specs, want) {
		t.Errorf("got %+v, want %+v", specs, want)
	}
}

func TestLookup(t *testing.T) {
	have := []string{"S", "s", "Red"}
	cases := []struct {
		in        string
		idx       int
		ambiguous bool
	}{
		{"s", 1, false},   // exact beats case-insensitive
		{"red", 2, false}, // the only case-insensitive candidate
		{"RED", 2, false},
		{"M", -1, false},
	}
	for _, tc := range cases {
		if i, amb := lookup(have, tc.in); i != tc.idx || amb != tc.ambiguous {
			t.Errorf("lookup(%q) = %d,%v want %d,%v", tc.in, i, amb, tc.idx, tc.ambiguous)
		}
	}
	if i, amb := lookup([]string{"Ab", "aB"}, "AB"); i != -1 || !amb {
		t.Errorf("two case-insensitive candidates must be ambiguous, got %d,%v", i, amb)
	}
}

func TestValidateSKUTemplate(t *testing.T) {
	dims := []optionDim{{Name: "Color", Values: []string{"Red"}}}
	if err := validateSKUTemplate("TS-{Color}", dims); err != nil {
		t.Errorf("valid template rejected: %v", err)
	}
	if err := validateSKUTemplate("TS-{Colour}", dims); err == nil {
		t.Error("unknown placeholder accepted")
	}
	if err := validateSKUTemplate("TS-{Color", dims); err == nil {
		t.Error("unclosed placeholder accepted")
	}
}

// Rendering matches placeholders by the same normalized rule validation uses:
// a template that passes validation always substitutes.
func TestRenderSKU_CaseInsensitivePlaceholders(t *testing.T) {
	dims := []optionDim{
		{Name: "Color", Values: []string{"Red"}},
		{Name: "Size", Values: []string{"S"}},
	}
	combo := []string{"Red", "S"}
	if err := validateSKUTemplate("TS-{color}-{SIZE}", dims); err != nil {
		t.Fatalf("template should validate: %v", err)
	}
	if got := renderSKU("TS-{color}-{SIZE}", dims, combo); got != "TS-Red-S" {
		t.Errorf("case-mismatched placeholders must still render, got %q", got)
	}
	if got := renderSKU("A-{Color}x", dims, combo); got != "A-Redx" {
		t.Errorf("exact-case render broken: %q", got)
	}
}

func TestSkuMayBeStale(t *testing.T) {
	cases := []struct {
		sku  string
		old  [3]string
		now  []string
		want bool
	}{
		{"Ocean-6lenses-Black", [3]string{"6lenses", "Black"}, []string{"Black"}, true},
		{"TS-Red-S", [3]string{"Red", "S"}, []string{"Wine", "S"}, true},
		{"TS-Red-S", [3]string{"Red", "S"}, []string{"Red", "s"}, false}, // case-only respell
		{"TS-Red", [3]string{"Red"}, []string{"Red", "S"}, false},        // nothing removed
		{"TSX-1", [3]string{"Blue", "S"}, []string{"Blue"}, false},       // "S" only inside a token
		{"WR-01", [3]string{"Wine Red"}, []string{"Navy"}, false},
		{"wine red-01", [3]string{"Wine Red"}, []string{"Navy"}, true},
	}
	for _, tc := range cases {
		if got := skuMayBeStale(&oldVariant{SKU: tc.sku, Options: tc.old}, tc.now); got != tc.want {
			t.Errorf("skuMayBeStale(%q, %v→%v) = %v, want %v", tc.sku, tc.old, tc.now, got, tc.want)
		}
	}
}

// ── product fixtures ──────────────────────────────────────────────────────────

// existingColorProduct returns a GET body: options [Color(Red,Blue)], two
// variants with ids/skus/prices/stock.
func existingColorProduct() map[string]any {
	return map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red", "Blue"}, "position": float64(1)},
		},
		"variants": []any{
			map[string]any{"id": "v-red", "option1": "Red", "sku": "SKU-RED", "price": "88.00", "inventory_quantity": float64(7),
				"compare_at_price": "120.00", "weight": float64(1.5), "weight_unit": "kg"},
			map[string]any{"id": "v-blue", "option1": "Blue", "sku": "SKU-BLUE", "price": "77.00", "inventory_quantity": float64(9),
				"compare_at_price": "99.00", "barcode": "BC-BLUE", "weight": float64(2)},
		},
	}
}

// colorSizeProduct returns a GET body: Color(Red,Blue) × Size(S,M) minus the
// combos in skip, each colour at its own price, every variant with its own sku.
func colorSizeProduct(skip ...string) map[string]any {
	variants := []any{}
	for _, c := range []struct{ name, price string }{{"Red", "88.00"}, {"Blue", "77.00"}} {
		for _, size := range []string{"S", "M"} {
			if slices.Contains(skip, c.name+"/"+size) {
				continue
			}
			variants = append(variants, map[string]any{
				"id": "v-" + c.name + "-" + size, "option1": c.name, "option2": size,
				"sku": "TS-" + c.name + "-" + size, "price": c.price, "inventory_quantity": float64(5),
			})
		}
	}
	return map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red", "Blue"}, "position": float64(1)},
			map[string]any{"name": "Size", "values": []any{"S", "M"}, "position": float64(2)},
		},
		"variants": variants,
	}
}

// caseTwinProduct returns Color(Red) × Size(S,s): S and s are distinct values.
func caseTwinProduct() map[string]any {
	return map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red"}, "position": float64(1)},
			map[string]any{"name": "Size", "values": []any{"S", "s"}, "position": float64(2)},
		},
		"variants": []any{
			map[string]any{"id": "v-S", "option1": "Red", "option2": "S", "sku": "C-S", "price": "10.00"},
			map[string]any{"id": "v-s", "option1": "Red", "option2": "s", "sku": "C-s", "price": "11.00"},
		},
	}
}

// redOnlyProduct wraps one variant in a Color(Red) product.
func redOnlyProduct(variant map[string]any) map[string]any {
	return map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red"}, "position": float64(1)},
		},
		"variants": []any{variant},
	}
}

// imageProduct requires variant images; each colour has its own.
func imageProduct() map[string]any {
	p := existingColorProduct()
	p["need_variant_image"] = true
	p["images"] = []any{
		map[string]any{"id": "img-1", "src": "//cdn.example/a.png"},
		map[string]any{"id": "img-2", "src": "//cdn.example/b.png"},
	}
	vars := p["variants"].([]any)
	vars[0].(map[string]any)["image_id"] = "img-1"
	vars[1].(map[string]any)["image_id"] = "img-2"
	return p
}

// ── runner ────────────────────────────────────────────────────────────────────

// runSetVariants runs the shortcut against a stub serving product; the second
// return is the captured write body, nil when no write was sent.
func runSetVariants(t *testing.T, product map[string]any, options []string, values map[string]string) (common.ExecResult, map[string]any, error) {
	t.Helper()
	return execSetVariants(t, product, options, values, false)
}

func dryRunSetVariants(t *testing.T, product map[string]any, options []string, values map[string]string) (common.ExecResult, map[string]any, error) {
	t.Helper()
	return execSetVariants(t, product, options, values, true)
}

func execSetVariants(t *testing.T, product map[string]any, options []string, values map[string]string, dryRun bool) (common.ExecResult, map[string]any, error) {
	t.Helper()
	var captured map[string]any
	srv := matrixServer(t, product, &captured, nil)
	t.Cleanup(srv.Close)
	in := newSetVariantsInput(t, options, values, dryRun)
	in.Client = client.New(srv.URL)
	res, err := setVariantsShortcut.Execute(context.Background(), in)
	return res, captured, err
}

func capturedOptions(t *testing.T, captured map[string]any) map[string][]any {
	t.Helper()
	prod, _ := captured["product"].(map[string]any)
	raw, _ := prod["options"].([]any)
	out := map[string][]any{}
	for _, ro := range raw {
		m := ro.(map[string]any)
		out[m["name"].(string)], _ = m["values"].([]any)
	}
	return out
}

// mustNotHave fails when v carries any of keys.
func mustNotHave(t *testing.T, v map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := v[k]; ok {
			t.Errorf("variant %v must not carry %q (got %v)", v["id"], k, v[k])
		}
	}
}

// ── flag gates (refused before the product is read) ───────────────────────────

func TestSetVariants_FlagGates(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"missing id", map[string]string{"action": "add"}, "--id"},
		{"missing action", map[string]string{"id": "p-1"}, "--action"},
		{"unknown action", map[string]string{"id": "p-1", "action": "merge"}, "unknown --action"},
		{"unparsable price", map[string]string{"id": "p-1", "action": "add", "price": "abc"}, "--price"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := newSetVariantsInput(t, []string{"Color:Red"}, tc.values, false)
			_, err := setVariantsShortcut.Execute(context.Background(), in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// ── values on an unchanged dimension set ──────────────────────────────────────

func TestSetVariants_AddValue(t *testing.T) {
	res, captured, err := runSetVariants(t, existingColorProduct(),
		[]string{"Color:White"}, map[string]string{"id": "p-1", "action": "add", "price": "10"})
	if err != nil {
		t.Fatal(err)
	}

	variants := capturedVariants(t, captured)
	if len(variants) != 3 {
		t.Fatalf("want 3 variants (existing preserved + White), got %d", len(variants))
	}
	red := variantByOptions(t, variants, "Red", "", "")
	if red["id"] != "v-red" {
		t.Errorf("Red must carry its old id, got %v", red["id"])
	}
	mustNotHave(t, red, "price", "sku")
	white := variantByOptions(t, variants, "White", "", "")
	mustNotHave(t, white, "id")
	if white["price"] != float64(10) {
		t.Errorf("new combo needs --price, got %v", white["price"])
	}

	if res.Body["kept"] != 2 || res.Body["created"] != 1 || res.Body["deleted"] != 0 || res.Body["price_applied"] != 1 {
		t.Errorf("summary counts wrong: %+v", res.Body)
	}
	if res.Body["action"] != "add" || res.Body["dimension_change"] != false {
		t.Errorf("summary wrong: %+v", res.Body)
	}
	if d := res.Body["created_detail"].([]map[string]any); len(d) != 1 || d[0]["options"] != "White" {
		t.Errorf("created_detail must name the new combo: %+v", d)
	}
}

// A new value follows the combinations that exist; one deleted earlier stays deleted.
func TestSetVariants_AddValueFollowsExistingCombinations(t *testing.T) {
	cases := []struct {
		option string
		want   [][2]string
	}{
		{"Color:Green", [][2]string{{"Green", "S"}, {"Green", "M"}}},
		{"Size:L", [][2]string{{"Red", "L"}, {"Blue", "L"}}},
	}
	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			res, captured, err := runSetVariants(t, colorSizeProduct("Red/M"),
				[]string{tc.option}, map[string]string{"id": "p-1", "action": "add", "price": "5"})
			if err != nil {
				t.Fatal(err)
			}
			variants := capturedVariants(t, captured)
			if len(variants) != 3+len(tc.want) {
				t.Fatalf("want %d variants, got %d: %+v", 3+len(tc.want), len(variants), variants)
			}
			for _, w := range tc.want {
				mustNotHave(t, variantByOptions(t, variants, w[0], w[1], ""), "id")
			}
			for _, v := range variants {
				if v["option1"] == "Red" && v["option2"] == "M" {
					t.Error("the deleted Red/M must not come back")
				}
			}
			if res.Body["created"] != len(tc.want) || res.Body["kept"] != 3 {
				t.Errorf("summary wrong: %+v", res.Body)
			}
		})
	}
}

func TestSetVariants_RemoveValueReportsDeleted(t *testing.T) {
	res, _, err := runSetVariants(t, existingColorProduct(),
		[]string{"Color:Blue"}, map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["deleted"] != 1 || res.Body["kept"] != 1 {
		t.Fatalf("summary: want deleted=1 kept=1, got %+v", res.Body)
	}
	detail := res.Body["deleted_detail"].([]map[string]any)
	want := map[string]any{"id": "v-blue", "options": "Blue", "sku": "SKU-BLUE", "price": "77.00",
		"inventory_quantity": 9, "compare_at_price": "99.00", "barcode": "BC-BLUE", "weight": json.Number("2")}
	if !reflect.DeepEqual(detail[0], want) {
		t.Errorf("deleted_detail must carry what the deletion loses:\n got %+v\nwant %+v", detail[0], want)
	}
}

// Removing a value never needs --price, and a value left with no variant is dropped.
func TestSetVariants_RemoveValueOnSparseProductPrunes(t *testing.T) {
	res, captured, err := runSetVariants(t, colorSizeProduct("Red/M"),
		[]string{"Size:S"}, map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatalf("a pure delete creates nothing, so --price must not be required: %v", err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 1 || variants[0]["id"] != "v-Blue-M" {
		t.Fatalf("only Blue/M survives, got %+v", variants)
	}
	if colors := capturedOptions(t, captured)["Color"]; !reflect.DeepEqual(colors, []any{"Blue"}) {
		t.Errorf("Red has no variant left and must be pruned, got %v", colors)
	}
	want := []map[string]any{{"name": "Color", "values": []string{"Red"}}}
	if !reflect.DeepEqual(res.Body["pruned_values"], want) {
		t.Errorf("pruned_values wrong: %+v", res.Body["pruned_values"])
	}
}

// S and s are distinct values: an unrelated edit must keep both as they are.
func TestSetVariants_CaseTwinsStayDistinct(t *testing.T) {
	res, captured, err := runSetVariants(t, caseTwinProduct(),
		[]string{"Color:Blue"}, map[string]string{"id": "p-1", "action": "add", "price": "5"})
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	for _, tc := range []struct{ size, id string }{{"S", "v-S"}, {"s", "v-s"}} {
		v := variantByOptions(t, variants, "Red", tc.size, "")
		if v["id"] != tc.id {
			t.Errorf("Red/%s must keep %s, got %v", tc.size, tc.id, v["id"])
		}
		mustNotHave(t, v, "price", "sku")
	}
	if res.Body["kept"] != 2 || res.Body["created"] != 2 || res.Body["deleted"] != 0 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

func TestSetVariants_RemoveExactCaseTwin(t *testing.T) {
	res, captured, err := runSetVariants(t, caseTwinProduct(),
		[]string{"Size:s"}, map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 1 || variants[0]["id"] != "v-S" {
		t.Fatalf("only Red/s goes, got %+v", variants)
	}
	if res.Body["deleted"] != 1 || res.Body["dimension_change"] != false {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

// Value matching falls back to case-insensitive; the written value is the caller's.
func TestSetVariants_UpdateRespellsInPlace(t *testing.T) {
	res, captured, err := runSetVariants(t, existingColorProduct(),
		[]string{"Color: RED , blue "}, map[string]string{"id": "p-1", "action": "update"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["kept"] != 2 || res.Body["deleted"] != 0 {
		t.Errorf("case/space variants must still match: %+v", res.Body)
	}
	red := variantByOptions(t, capturedVariants(t, captured), "RED", "", "")
	if red["id"] != "v-red" {
		t.Errorf("respelled value must keep the old id, got %v", red["id"])
	}
	if d := res.Body["remapped_detail"].([]map[string]any); len(d) != 2 || d[0]["from"] != "Red" || d[0]["to"] != "RED" {
		t.Errorf("remapped_detail must show the respelling: %+v", d)
	}
}

func TestSetVariants_UpdateReplacesValues(t *testing.T) {
	res, _, err := runSetVariants(t, existingColorProduct(),
		[]string{"Color:Crimson,Blue"}, map[string]string{"id": "p-1", "action": "update", "price": "10"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["created"] != 1 || res.Body["kept"] != 1 || res.Body["deleted"] != 1 {
		t.Errorf("update must delete Red and create Crimson: %+v", res.Body)
	}
}

// New values are laid out against the combinations that exist before the
// replaced values go.
func TestSetVariants_UpdateReplacingEveryValue(t *testing.T) {
	res, captured, err := runSetVariants(t, colorSizeProduct(),
		[]string{"Color:Green"}, map[string]string{"id": "p-1", "action": "update", "price": "5"})
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 2 {
		t.Fatalf("want Green/S and Green/M, got %+v", variants)
	}
	variantByOptions(t, variants, "Green", "S", "")
	variantByOptions(t, variants, "Green", "M", "")
	if res.Body["created"] != 2 || res.Body["deleted"] != 4 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

// Option slots follow the server's real position values, not array order:
// positions [1,3] still match every variant, and the vacated slot is cleared.
func TestSetVariants_NonContiguousPositionsStillMatch(t *testing.T) {
	product := map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Fit", "values": []any{"Slim"}, "position": float64(3)},
			map[string]any{"name": "Color", "values": []any{"Red"}, "position": float64(1)},
		},
		"variants": []any{
			map[string]any{"id": "v-1", "option1": "Red", "option3": "Slim", "sku": "K", "inventory_quantity": float64(3)},
		},
	}
	res, captured, err := runSetVariants(t, product,
		[]string{"Color:Green"}, map[string]string{"id": "p-1", "action": "add", "price": "7"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["kept"] != 1 || res.Body["created"] != 1 || res.Body["deleted"] != 0 {
		t.Errorf("old variant in slots 1/3 must match: %+v", res.Body)
	}
	red := variantByOptions(t, capturedVariants(t, captured), "Red", "Slim", "")
	if red["id"] != "v-1" {
		t.Errorf("Red/Slim must carry the old id, got %v", red["id"])
	}
	if o3, ok := red["option3"]; !ok || o3 != "" {
		t.Errorf("Slim moved to option2, so option3 must be cleared explicitly, got %v", red)
	}
}

// A delta that is already satisfied converges without a write (retry safety).
func TestSetVariants_ConvergesWithoutWrite(t *testing.T) {
	cases := []struct {
		name   string
		option string
		values map[string]string
	}{
		{"add a value that already exists", "Color:Red", map[string]string{"id": "p-1", "action": "add", "price": "10"}},
		{"update with the same values", "Color:Red,Blue", map[string]string{"id": "p-1", "action": "update"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, captured, err := runSetVariants(t, existingColorProduct(), []string{tc.option}, tc.values)
			if err != nil {
				t.Fatal(err)
			}
			if captured != nil {
				t.Error("a no-change delta must not send the PUT")
			}
			if res.Body["no_change"] != true || res.Body["kept"] != 2 {
				t.Errorf("summary wrong: %+v", res.Body)
			}
		})
	}
}

// ── dimension changes ─────────────────────────────────────────────────────────

// Existing variants take the new dimension's first value and keep their id;
// the other values get siblings copying everything but sku/barcode/stock.
func TestSetVariants_AddDimensionPinsFirstValue(t *testing.T) {
	res, captured, err := runSetVariants(t, existingColorProduct(),
		[]string{"Size:S,M"}, map[string]string{"id": "p-1", "action": "add"})
	if err != nil {
		t.Fatalf("every sibling has a price to copy, so --price must not be required: %v", err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 4 {
		t.Fatalf("want 4 variants (Color×Size), got %d", len(variants))
	}
	for _, tc := range []struct{ color, id string }{{"Red", "v-red"}, {"Blue", "v-blue"}} {
		v := variantByOptions(t, variants, tc.color, "S", "")
		if v["id"] != tc.id {
			t.Errorf("%s/S must keep %s, got %v", tc.color, tc.id, v["id"])
		}
		mustNotHave(t, v, "price", "sku", "inventory_quantity")
	}
	redM := variantByOptions(t, variants, "Red", "M", "")
	mustNotHave(t, redM, "id", "sku", "barcode", "inventory_quantity")
	if redM["price"] != "88.00" || redM["compare_at_price"] != "120.00" || redM["weight"] != float64(1.5) || redM["weight_unit"] != "kg" {
		t.Errorf("Red/M must copy Red's price/compare-at/weight, got %+v", redM)
	}
	if blueM := variantByOptions(t, variants, "Blue", "M", ""); blueM["price"] != "77.00" {
		t.Errorf("Blue/M must copy Blue's price, got %+v", blueM)
	}

	if res.Body["dimension_change"] != true || res.Body["kept"] != 2 || res.Body["cloned"] != 2 ||
		res.Body["created"] != 0 || res.Body["deleted"] != 0 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
	if _, ok := res.Body["price_applied"]; ok {
		t.Error("price_applied is reported only when --price is passed")
	}
	remapped := res.Body["remapped_detail"].([]map[string]any)
	if len(remapped) != 2 || remapped[0]["from"] != "Red" || remapped[0]["to"] != "Red/S" {
		t.Errorf("remapped_detail must show where each old variant went: %+v", remapped)
	}
	created := res.Body["created_detail"].([]map[string]any)
	if len(created) != 2 || created[0]["options"] != "Red/M" || created[0]["copied_from"] != "v-red" {
		t.Errorf("created_detail must name each sibling and its source: %+v", created)
	}
}

// The default variant of a product with no options takes the first combination.
func TestSetVariants_DefaultVariantTakesFirstCombination(t *testing.T) {
	product := map[string]any{
		"id":       "p-1",
		"options":  []any{},
		"variants": []any{map[string]any{"id": "v-0", "price": "29.99", "sku": "DEF", "inventory_quantity": float64(4)}},
	}
	res, captured, err := runSetVariants(t, product,
		[]string{"Color:Red,Blue"}, map[string]string{"id": "p-1", "action": "add"})
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if red := variantByOptions(t, variants, "Red", "", ""); red["id"] != "v-0" {
		t.Errorf("the default variant must become Red, got %+v", red)
	}
	if blue := variantByOptions(t, variants, "Blue", "", ""); blue["price"] != "29.99" {
		t.Errorf("Blue must copy the default variant's price, got %+v", blue)
	}
	if res.Body["kept"] != 1 || res.Body["cloned"] != 1 || res.Body["deleted"] != 0 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

// A dimension can be dropped when every remaining combination keeps one variant.
func TestSetVariants_RemoveDimensionOneToOne(t *testing.T) {
	res, captured, err := runSetVariants(t, colorSizeProduct("Red/M", "Blue/S"),
		[]string{"Size"}, map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 2 {
		t.Fatalf("want Red and Blue, got %+v", variants)
	}
	red := variantByOptions(t, variants, "Red", "", "")
	if red["id"] != "v-Red-S" || red["option2"] != "" {
		t.Errorf("Red must keep v-Red-S with option2 cleared, got %+v", red)
	}
	if res.Body["dimension_change"] != true || res.Body["kept"] != 2 || res.Body["deleted"] != 0 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
	stale := res.Body["sku_may_be_stale"].([]map[string]any)
	if len(stale) != 2 || stale[0]["sku"] != "TS-Red-S" {
		t.Errorf("skus still naming the dropped size must be flagged: %+v", stale)
	}
}

func TestSetVariants_RenameValue(t *testing.T) {
	res, captured, err := runSetVariants(t, colorSizeProduct(),
		[]string{"Color:red=Wine"}, map[string]string{"id": "p-1", "action": "rename"})
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 4 {
		t.Fatalf("rename keeps every variant, got %d", len(variants))
	}
	for _, v := range variants {
		if _, ok := v["id"]; !ok {
			t.Errorf("every variant must keep its id: %+v", v)
		}
	}
	if v := variantByOptions(t, variants, "Wine", "S", ""); v["id"] != "v-Red-S" {
		t.Errorf("Wine/S must be the old Red/S, got %+v", v)
	}
	if colors := capturedOptions(t, captured)["Color"]; !reflect.DeepEqual(colors, []any{"Wine", "Blue"}) {
		t.Errorf("the value keeps its position, got %v", colors)
	}
	if res.Body["dimension_change"] != false || res.Body["kept"] != 4 || res.Body["deleted"] != 0 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
	if stale := res.Body["sku_may_be_stale"].([]map[string]any); len(stale) != 2 {
		t.Errorf("TS-Red-* skus must be flagged: %+v", stale)
	}
}

func TestSetVariants_RenameDimension(t *testing.T) {
	res, captured, err := runSetVariants(t, colorSizeProduct(),
		[]string{"Color=Colour"}, map[string]string{"id": "p-1", "action": "rename"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := capturedOptions(t, captured)["Colour"]; !ok {
		t.Errorf("dimension must be renamed: %+v", capturedOptions(t, captured))
	}
	if res.Body["kept"] != 4 || res.Body["dimension_change"] != false {
		t.Errorf("summary wrong: %+v", res.Body)
	}
	if _, ok := res.Body["remapped_detail"]; ok {
		t.Error("a name change moves no variant")
	}
}

// ── images ────────────────────────────────────────────────────────────────────

func TestSetVariants_ImageCopiedToSiblings(t *testing.T) {
	_, captured, err := runSetVariants(t, imageProduct(),
		[]string{"Size:S,M"}, map[string]string{"id": "p-1", "action": "add"})
	if err != nil {
		t.Fatal(err)
	}
	if v := variantByOptions(t, capturedVariants(t, captured), "Blue", "M", ""); v["image_id"] != "img-2" {
		t.Errorf("Blue/M must copy Blue's image, got %+v", v)
	}
}

func TestSetVariants_NewVariantImage(t *testing.T) {
	values := map[string]string{"id": "p-1", "action": "add", "price": "5"}
	if _, captured, err := runSetVariants(t, imageProduct(), []string{"Color:Green"}, values); err == nil ||
		!strings.Contains(err.Error(), "--image") || captured != nil {
		t.Errorf("a required image must be asked for before writing, got %v", err)
	}

	values["image"] = "https://cdn.example/b.png"
	_, captured, err := runSetVariants(t, imageProduct(), []string{"Color:Green"}, values)
	if err != nil {
		t.Fatal(err)
	}
	if v := variantByOptions(t, capturedVariants(t, captured), "Green", "", ""); v["image_id"] != "img-2" {
		t.Errorf("--image must resolve to the product image id, got %+v", v)
	}

	values["image"] = "https://cdn.example/new.png"
	if _, _, err := runSetVariants(t, imageProduct(), []string{"Color:Green"}, values); err == nil ||
		!strings.Contains(err.Error(), "products images create") {
		t.Errorf("an image outside the product must be refused, got %v", err)
	}
}

// ── refusals: the product is read, but nothing is written ─────────────────────

func TestSetVariants_RefusesWithoutWriting(t *testing.T) {
	// A dimension with no determinable values cannot be mapped.
	valuelessDimension := map[string]any{
		"id":       "p-1",
		"options":  []any{map[string]any{"name": "Color", "position": float64(1)}},
		"variants": []any{map[string]any{"id": "v-1", "sku": "K", "inventory_quantity": float64(3)}},
	}
	threeDims := map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "A", "values": []any{"a"}, "position": float64(1)},
			map[string]any{"name": "B", "values": []any{"b"}, "position": float64(2)},
			map[string]any{"name": "C", "values": []any{"c"}, "position": float64(3)},
		},
		"variants": []any{map[string]any{"id": "v-1", "option1": "a", "option2": "b", "option3": "c", "price": "1"}},
	}
	cases := []struct {
		name    string
		product map[string]any
		options []string
		values  map[string]string
		want    string
	}{
		{"brand-new value needs a price", existingColorProduct(), []string{"Color:White"},
			map[string]string{"id": "p-1", "action": "add"}, "--price"},
		{"brand-new value needs a price mid-reshape",
			redOnlyProduct(map[string]any{"id": "v-red", "option1": "Red", "price": "88.00"}),
			[]string{"Color:Green", "Size:S"}, map[string]string{"id": "p-1", "action": "add"}, "--price"},
		{"sibling with no price to copy needs one",
			redOnlyProduct(map[string]any{"id": "v-red", "option1": "Red", "sku": "SKU-RED"}),
			[]string{"Size:S,M"}, map[string]string{"id": "p-1", "action": "add"}, "--price"},
		{"value differing only by case", existingColorProduct(), []string{"Color:red"},
			map[string]string{"id": "p-1", "action": "add", "price": "5"}, "only by case"},
		{"update asserts the dimension exists", existingColorProduct(), []string{"Fit:Slim"},
			map[string]string{"id": "p-1", "action": "update", "price": "9"}, "--action add"},
		{"remove by a case-folded spelling", existingColorProduct(), []string{"Color:red"},
			map[string]string{"id": "p-1", "action": "remove"}, `did you mean "Red"?`},
		{"remove a missing value", existingColorProduct(), []string{"Color:Green"},
			map[string]string{"id": "p-1", "action": "remove"}, "existing: Red,Blue"},
		{"remove a missing dimension", existingColorProduct(), []string{"尺码:S"},
			map[string]string{"id": "p-1", "action": "remove"}, "existing: Color[Red,Blue]"},
		{"remove every value of a dimension", colorSizeProduct(), []string{"Size:S,M"},
			map[string]string{"id": "p-1", "action": "remove"}, `bare name --option "Size"`},
		{"remove a dimension that would merge variants", colorSizeProduct(), []string{"Size"},
			map[string]string{"id": "p-1", "action": "remove"}, `merge variants into the same combination (Red/S + Red/M; Blue/S + Blue/M)`},
		{"remove the last dimension", existingColorProduct(), []string{"Color"},
			map[string]string{"id": "p-1", "action": "remove"}, "every dimension"},
		{"rename onto a case twin", existingColorProduct(), []string{"Color:Red=blue"},
			map[string]string{"id": "p-1", "action": "rename"}, "only by case"},
		{"rename onto another dimension", colorSizeProduct(), []string{"Color=size"},
			map[string]string{"id": "p-1", "action": "rename"}, "already exists"},
		{"rename a missing value", existingColorProduct(), []string{"Color:Green=Lime"},
			map[string]string{"id": "p-1", "action": "rename"}, "no value"},
		{"repeat of an applied rename", existingColorProduct(), []string{"Color:Crimson=Red"},
			map[string]string{"id": "p-1", "action": "rename"}, "was it renamed already?"},
		{"repeat of an applied dimension rename", existingColorProduct(), []string{"Colour=Color"},
			map[string]string{"id": "p-1", "action": "rename"}, "was it renamed already?"},
		{"rename onto an existing value", existingColorProduct(), []string{"Color:Red=Blue"},
			map[string]string{"id": "p-1", "action": "rename"}, `"Blue" already exists`},
		{"rename swap", existingColorProduct(), []string{"Color:Red=Blue,Blue=Red"},
			map[string]string{"id": "p-1", "action": "rename"}, "already exists"},
		{"rename chain", existingColorProduct(), []string{"Color:Red=Green,Green=Yellow"},
			map[string]string{"id": "p-1", "action": "rename"}, `no value "Green"`},
		{"two values renamed to one", existingColorProduct(), []string{"Color:Red=X,Blue=X"},
			map[string]string{"id": "p-1", "action": "rename"}, `two values to "X"`},
		{"dimension rename chain", colorSizeProduct(), []string{"Color=Colour", "Colour:Red=Wine"},
			map[string]string{"id": "p-1", "action": "rename"}, `"Colour" does not exist`},
		{"update matching one value twice by case", existingColorProduct(), []string{"Color:red,RED"},
			map[string]string{"id": "p-1", "action": "update", "price": "5"}, "both match"},
		{"update adding a case twin", existingColorProduct(), []string{"Color:Red,Blue,red"},
			map[string]string{"id": "p-1", "action": "update", "price": "5"}, "differ only by case"},
		{"new dimension with case twins", existingColorProduct(), []string{"Size:S,s"},
			map[string]string{"id": "p-1", "action": "add", "price": "5"}, "differ only by case"},
		{"negative --stock", existingColorProduct(), []string{"Color:Green"},
			map[string]string{"id": "p-1", "action": "add", "price": "5", "stock": "-1"}, "--stock"},
		{"two variants on one combination", map[string]any{"id": "p-1", "options": []any{},
			"variants": []any{map[string]any{"id": "v-1", "price": "1"}, map[string]any{"id": "v-2", "price": "1"}}},
			[]string{"Color:Red"}, map[string]string{"id": "p-1", "action": "add"}, "both have options"},
		{"new variant on a stale stored combination", staleSlotProduct(), []string{"Size:M,S"},
			map[string]string{"id": "p-1", "action": "add", "price": "5"}, "stale options"},
		{"a dimension with no values", valuelessDimension, []string{"Size:S,M"},
			map[string]string{"id": "p-1", "action": "add", "price": "5"}, `"Color"`},
		{"a fourth dimension", threeDims, []string{"D:d"},
			map[string]string{"id": "p-1", "action": "add", "price": "5"}, "at most 3"},
		{"--image on a product without variant images", existingColorProduct(), []string{"Color:Green"},
			map[string]string{"id": "p-1", "action": "add", "price": "5", "image": "x"}, "--image applies only"},
		{"--sku-template rendering duplicates", existingColorProduct(), []string{"Size:S,M"},
			map[string]string{"id": "p-1", "action": "add", "sku-template": "TS-{Color}"}, "reference every dimension"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, captured, err := runSetVariants(t, tc.product, tc.options, tc.values)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
			if captured != nil {
				t.Error("the PUT must not be sent when validation fails")
			}
		})
	}
}

// --price never touches an existing price; passing it when nothing needs it warns.
func TestSetVariants_UnusedPriceWarns(t *testing.T) {
	res, captured, err := runSetVariants(t, existingColorProduct(),
		[]string{"Size:S"}, map[string]string{"id": "p-1", "action": "add", "price": "19.9"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range capturedVariants(t, captured) {
		mustNotHave(t, v, "price")
	}
	if res.Body["price_applied"] != 0 || res.Body["warnings"] == nil {
		t.Errorf("an unused --price must be reported: %+v", res.Body)
	}
}

// ── sku template ──────────────────────────────────────────────────────────────

// A template may reference dimensions not named in --option, so validation
// runs against the merged matrix.
func TestSetVariants_SkuTemplateValidatedAgainstMergedDims(t *testing.T) {
	_, captured, err := runSetVariants(t, existingColorProduct(), []string{"Size:S"}, map[string]string{
		"id": "p-1", "action": "add", "sku-template": "X-{Color}-{Size}",
	})
	if err != nil {
		t.Fatalf("template referencing an existing dim must validate: %v", err)
	}
	if v := capturedVariants(t, captured)[0]; v["sku"] != "X-Red-S" {
		t.Errorf("template must render merged dims, got %v", v["sku"])
	}
}

// ── dry-run ───────────────────────────────────────────────────────────────────

// Dry-run reads the product and previews the real body.
func TestSetVariants_DryRunPreviewsTheRealBody(t *testing.T) {
	res, captured, err := dryRunSetVariants(t, existingColorProduct(),
		[]string{"Color:White"}, map[string]string{"id": "p-1", "action": "add", "price": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if captured != nil {
		t.Fatal("dry-run must not send the PUT")
	}
	if len(res.Plans) != 2 || res.Plans[0].Method != "GET" || res.Plans[1].Method != "PUT" {
		t.Fatalf("want [GET, PUT] plans, got %+v", res.Plans)
	}
	body, _ := res.Plans[1].Body.(map[string]any)
	prod, _ := body["product"].(map[string]any)
	if variants, _ := prod["variants"].([]any); len(variants) != 3 {
		t.Fatalf("preview body must carry the real variants array, got %+v", prod["variants"])
	}
	if res.Summary["created"] != 1 || res.Summary["kept"] != 2 {
		t.Errorf("dry-run must report the same summary as the live run: %+v", res.Summary)
	}
}

// Nothing to do: the preview shows the read and no write at all.
func TestSetVariants_DryRunNoChangeHasNoWrite(t *testing.T) {
	res, _, err := dryRunSetVariants(t, existingColorProduct(),
		[]string{"Color:Red"}, map[string]string{"id": "p-1", "action": "add", "price": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plans) != 1 || res.Plans[0].Method != "GET" {
		t.Fatalf("want the read alone, got %+v", res.Plans)
	}
	if res.Summary["no_change"] != true {
		t.Errorf("summary must say nothing changes: %+v", res.Summary)
	}
}

// ── detail cap ────────────────────────────────────────────────────────────────

func TestSetVariants_DetailCap(t *testing.T) {
	n := detailCap + 21
	oldVars := make([]any, n)
	drop := make([]string, n-1)
	for i := range oldVars {
		oldVars[i] = map[string]any{
			"id": fmt.Sprintf("v-%d", i), "option1": fmt.Sprintf("c%d", i), "inventory_quantity": float64(1),
		}
		if i < n-1 {
			drop[i] = fmt.Sprintf("c%d", i)
		}
	}
	product := map[string]any{
		"id":       "p-1",
		"options":  []any{map[string]any{"name": "Color", "position": float64(1)}},
		"variants": oldVars,
	}

	res, _, err := runSetVariants(t, product, []string{"Color:" + strings.Join(drop, ",")},
		map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["deleted"] != detailCap+20 {
		t.Errorf("deleted count must be the real total: %v", res.Body["deleted"])
	}
	if len(res.Body["deleted_detail"].([]map[string]any)) != detailCap {
		t.Error("deleted_detail must be capped")
	}
	if res.Body["deleted_detail_truncated"] != 20 {
		t.Errorf("truncated marker wrong: %v", res.Body["deleted_detail_truncated"])
	}
}

// staleSlotProduct has one dimension, but v-blue still stores option2 "S".
func staleSlotProduct() map[string]any {
	p := existingColorProduct()
	p["variants"].([]any)[1].(map[string]any)["option2"] = "S"
	return p
}

// idleValueProduct lists Green and L, which no variant uses.
func idleValueProduct() map[string]any {
	p := colorSizeProduct()
	opts := p["options"].([]any)
	opts[0].(map[string]any)["values"] = []any{"Red", "Blue", "Green"}
	opts[1].(map[string]any)["values"] = []any{"S", "M", "L"}
	return p
}

// A value listed with no variant is left alone by unrelated edits.
func TestSetVariants_IdleValuesSurviveUnrelatedEdits(t *testing.T) {
	res, captured, err := runSetVariants(t, idleValueProduct(),
		[]string{"Color:Blue"}, map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	opts := capturedOptions(t, captured)
	if !reflect.DeepEqual(opts["Color"], []any{"Red", "Green"}) || !reflect.DeepEqual(opts["Size"], []any{"S", "M", "L"}) {
		t.Errorf("idle Green and L must stay, got %v", opts)
	}
	if _, ok := res.Body["pruned_values"]; ok {
		t.Errorf("nothing lost its last variant: %+v", res.Body["pruned_values"])
	}
}

// Adding or updating to a value that exists without variants gives it variants.
func TestSetVariants_IdleValueGetsVariants(t *testing.T) {
	for _, tc := range []struct {
		action, option string
	}{{"add", "Color:Green"}, {"update", "Color:Red,Blue,Green"}} {
		t.Run(tc.action, func(t *testing.T) {
			res, captured, err := runSetVariants(t, idleValueProduct(),
				[]string{tc.option}, map[string]string{"id": "p-1", "action": tc.action, "price": "5"})
			if err != nil {
				t.Fatal(err)
			}
			variants := capturedVariants(t, captured)
			variantByOptions(t, variants, "Green", "S", "")
			variantByOptions(t, variants, "Green", "M", "")
			if res.Body["created"] != 2 || res.Body["kept"] != 4 {
				t.Errorf("summary wrong: %+v", res.Body)
			}
		})
	}
}

// Option values a variant uses but the options array omits are still matched exactly.
func TestSetVariants_VariantValueMissingFromOptions(t *testing.T) {
	p := existingColorProduct()
	p["variants"] = append(p["variants"].([]any), map[string]any{"id": "v-RED", "option1": "RED", "price": "1"})
	_, captured, err := runSetVariants(t, p, []string{"Color:RED"}, map[string]string{"id": "p-1", "action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, v := range capturedVariants(t, captured) {
		ids = append(ids, v["id"].(string))
	}
	if !reflect.DeepEqual(ids, []string{"v-red", "v-blue"}) {
		t.Errorf("only the RED variant goes, got %v", ids)
	}
}

// A no-op on stored positions [1,3] sends nothing, and a real edit there does
// not report the re-slotted variant as moved.
func TestSetVariants_NonContiguousNoOp(t *testing.T) {
	product := map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red"}, "position": float64(1)},
			map[string]any{"name": "Fit", "values": []any{"Slim"}, "position": float64(3)},
		},
		"variants": []any{map[string]any{"id": "v-1", "option1": "Red", "option3": "Slim", "price": "1"}},
	}
	res, captured, err := runSetVariants(t, product, []string{"Color:Red"}, map[string]string{"id": "p-1", "action": "add"})
	if err != nil || captured != nil || res.Body["no_change"] != true {
		t.Fatalf("want no_change without a write, got err=%v write=%v %+v", err, captured != nil, res.Body)
	}
	res, _, err = runSetVariants(t, product, []string{"Color:Green"}, map[string]string{"id": "p-1", "action": "add", "price": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Body["remapped_detail"]; ok {
		t.Errorf("re-slotting alone is not a move: %+v", res.Body["remapped_detail"])
	}
}

// An unused --price is reported even when nothing is written.
func TestSetVariants_UnusedPriceOnNoOp(t *testing.T) {
	res, _, err := runSetVariants(t, existingColorProduct(), []string{"Color:Red"},
		map[string]string{"id": "p-1", "action": "add", "price": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["no_change"] != true || res.Body["price_applied"] != 0 || res.Body["warnings"] == nil {
		t.Errorf("want no_change with the unused --price reported: %+v", res.Body)
	}
}

// Several update payloads: a value one payload lists keeps variants even when a
// later payload removes the ones it had, whatever the order.
func TestSetVariants_UpdatePayloadsKeepListedValues(t *testing.T) {
	for _, options := range [][]string{
		{"Size:S,L", "Color:Blue"},
		{"Color:Blue", "Size:S,L"},
	} {
		t.Run(strings.Join(options, "+"), func(t *testing.T) {
			_, captured, err := runSetVariants(t, colorSizeProduct("Blue/S"), options,
				map[string]string{"id": "p-1", "action": "update", "price": "5"})
			if err != nil {
				t.Fatal(err)
			}
			variants := capturedVariants(t, captured)
			if len(variants) != 2 {
				t.Fatalf("want Blue/S and Blue/L, got %+v", variants)
			}
			mustNotHave(t, variantByOptions(t, variants, "Blue", "S", ""), "id")
			mustNotHave(t, variantByOptions(t, variants, "Blue", "L", ""), "id")
		})
	}
}

// ── combined form ─────────────────────────────────────────────────────────────

// runEdit runs the combined form; args are raw flags ("--drop=lenses=6lenses").
func runEdit(t *testing.T, product map[string]any, args ...string) (common.ExecResult, map[string]any, error) {
	t.Helper()
	srv := newEditServer(t)
	return srv.run(t, product, args...)
}

// editServer serves one product per run from a single listener, so a loop of
// runs does not open a socket each.
type editServer struct {
	product  map[string]any
	captured map[string]any
	url      string
}

func newEditServer(t *testing.T) *editServer {
	t.Helper()
	s := &editServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"product": s.product})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&s.captured); err != nil {
			t.Errorf("decode captured body: %v", err)
		}
		json.NewEncoder(w).Encode(s.captured)
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

// run executes +set-variants with flags registered from the shortcut itself,
// so the test sees the real flag set.
func (s *editServer) run(t *testing.T, product map[string]any, args ...string) (common.ExecResult, map[string]any, error) {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for _, f := range setVariantsShortcut.Flags {
		switch f.Type {
		case common.FlagStringArray:
			cmd.Flags().StringArray(f.Name, nil, "")
		case common.FlagInt:
			cmd.Flags().Int(f.Name, 0, "")
		default:
			cmd.Flags().String(f.Name, "", "")
		}
	}
	cmd.SetArgs(append([]string{"--id=p-1"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	s.product, s.captured = product, nil
	in := common.ExecInput{Flags: common.NewCobraFlagSet(cmd), Client: client.New(s.url)}
	res, err := setVariantsShortcut.Execute(context.Background(), in)
	return res, s.captured, err
}

// lensProduct: Style(Ocean,Galaxy) × lenses × Color(Black 19.9, White 39.9), variant images required.
func lensProduct(lenses ...string) map[string]any {
	var variants []any
	for _, s := range []string{"Ocean", "Galaxy"} {
		for _, l := range lenses {
			for _, c := range []string{"Black", "White"} {
				price := 19.9
				if c == "White" {
					price = 39.9
				}
				variants = append(variants, map[string]any{"id": "v-" + s + "-" + l + "-" + c, "option1": s, "option2": l, "option3": c,
					"sku": "-" + s + "-" + l + "-" + c, "price": price, "image_id": "img-" + c, "inventory_quantity": float64(3)})
			}
		}
	}
	lensValues := []any{}
	for _, l := range lenses {
		lensValues = append(lensValues, l)
	}
	return map[string]any{
		"id": "p-1", "need_variant_image": true,
		"images": []any{map[string]any{"id": "img-Black", "src": "//b.webp"}, map[string]any{"id": "img-White", "src": "//w.webp"}},
		"options": []any{
			map[string]any{"name": "Style", "values": []any{"Ocean", "Galaxy"}, "position": float64(1)},
			map[string]any{"name": "lenses", "values": lensValues, "position": float64(2)},
			map[string]any{"name": "Color", "values": []any{"Black", "White"}, "position": float64(3)},
		},
		"variants": variants,
	}
}

func optionsJSON(t *testing.T, captured map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(captured["product"].(map[string]any)["options"])
	return string(b)
}

func TestSetVariants_CombinedEditInOneWrite(t *testing.T) {
	res, captured, err := runEdit(t, lensProduct("6lenses", "12lenses"),
		"--drop=lenses=6lenses", "--set=Style:Ocean", "--rename=Style:Ocean=2 lenses", "--set-price=19.9")
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 2 {
		t.Fatalf("want 2 variants, got %+v", variants)
	}
	for _, c := range []string{"Black", "White"} {
		v := variantByOptions(t, variants, "2 lenses", c, "")
		if v["id"] != "v-Ocean-6lenses-"+c || v["option3"] != "" || v["price"] != 19.9 {
			t.Errorf("%s: want the 6lenses variant kept, slot 3 cleared, price 19.9: %+v", c, v)
		}
	}
	if got := optionsJSON(t, captured); got != `[{"name":"Style","values":["2 lenses"]},{"name":"Color","values":["Black","White"]}]` {
		t.Errorf("options: %s", got)
	}
	want := []string{
		"dimension lenses dropped, keeping the variants with 6lenses (4 other variants deleted)",
		"Style: values Ocean, Galaxy → Ocean",
		"Style: Ocean renamed to 2 lenses",
		"price set to 19.9",
		"variants: 2 kept, 0 new, 6 deleted (8 → 2)",
	}
	if !reflect.DeepEqual(res.Body["changes"], want) {
		t.Errorf("changes = %q", res.Body["changes"])
	}
	if _, ok := res.Body["action"]; ok {
		t.Error("the combined form reports changes, not an action")
	}
}

func TestSetVariants_DropNeedsKeepWhenVariantsMerge(t *testing.T) {
	_, captured, err := runEdit(t, lensProduct("6lenses", "12lenses"), "--drop=lenses")
	if err == nil || !strings.Contains(err.Error(), `--drop "lenses=<value>"`) || captured != nil {
		t.Fatalf("want a refusal naming the =value form, got %v", err)
	}
	_, captured, err = runEdit(t, lensProduct("6lenses"), "--drop=lenses")
	if err != nil || len(capturedVariants(t, captured)) != 4 {
		t.Fatalf("a drop that merges nothing keeps every variant: %v", err)
	}
}

func TestSetVariants_AddDimensionWithExisting(t *testing.T) {
	_, _, err := runEdit(t, colorSizeProduct(), "--add=Fit:Slim,Regular")
	if err == nil || !strings.Contains(err.Error(), "--existing") {
		t.Fatalf("several values need --existing, got %v", err)
	}
	_, captured, err := runEdit(t, colorSizeProduct(), "--add=Fit:Slim,Regular,Loose", "--existing=Fit:Regular")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range capturedVariants(t, captured) {
		if _, kept := v["id"]; kept && v["option3"] != "Regular" {
			t.Errorf("kept variant must take Regular: %+v", v)
		}
	}
	if got := optionsJSON(t, captured); !strings.Contains(got, `["Slim","Regular","Loose"]`) {
		t.Errorf("the listed order is the storefront order: %s", got)
	}
	// A product with no dimension yet (fresh +create): its one variant takes
	// the first value of each added dimension, no --existing needed.
	specless := map[string]any{"id": "p-1", "options": []any{},
		"variants": []any{map[string]any{"id": "v-0", "price": "29.99", "sku": "DEF"}}}
	_, captured, err = runEdit(t, specless, "--add=Color:Red,Blue", "--add=Size:S,M")
	if err != nil {
		t.Fatal(err)
	}
	if v := variantByOptions(t, capturedVariants(t, captured), "Red", "S", ""); v["id"] != "v-0" {
		t.Errorf("the default variant must become Red/S: %+v", v)
	}
}

func TestSetVariants_PerValuePriceAndImage(t *testing.T) {
	// New colors with their own price and image; the rest fall back to the plain value.
	_, captured, err := runEdit(t, lensProduct("6lenses"), "--add=Color:Red,Green",
		"--new-price=Color:Red=9", "--new-price=7", "--new-image=Color:Red=https://b.webp", "--new-image=img-White", "--new-stock=8")
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	red := variantByOptions(t, variants, "Ocean", "6lenses", "Red")
	green := variantByOptions(t, variants, "Ocean", "6lenses", "Green")
	if red["price"] != 9.0 || red["image_id"] != "img-Black" || red["inventory_quantity"] != 8.0 {
		t.Errorf("red: %+v", red)
	}
	if green["price"] != 7.0 || green["image_id"] != "img-White" {
		t.Errorf("green: %+v", green)
	}
	// --set-price per value reprices existing variants with that value only.
	_, captured, err = runEdit(t, colorSizeProduct(), "--set-price=Size:S=20,M=30")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range capturedVariants(t, captured) {
		if want := map[string]float64{"S": 20, "M": 30}[v["option2"].(string)]; v["price"] != want {
			t.Errorf("want %v: %+v", want, v)
		}
	}
	_, captured, err = runEdit(t, colorSizeProduct(), "--set-price=Size:S=20")
	if err != nil {
		t.Fatal(err)
	}
	mustNotHave(t, variantByOptions(t, capturedVariants(t, captured), "Red", "M", ""), "price")
}

func TestSetVariants_CombinedRefusals(t *testing.T) {
	cases := []struct {
		name    string
		product map[string]any
		args    []string
		want    string
	}{
		{"one dimension in two edits", colorSizeProduct(), []string{"--set=Color:Red", "--add=Color:Green", "--new-price=1"}, "together"},
		{"one dimension twice", colorSizeProduct(), []string{"--remove=Color:Red", "--remove=Color:Blue"}, "twice"},
		{"removed and renamed", colorSizeProduct(), []string{"--remove=Color:Red", "--rename=Color:Red=Wine"}, "both removed and renamed"},
		{"renamed but not kept by --set", colorSizeProduct(), []string{"--set=Color:Blue", "--rename=Color:Red=Wine"}, "not kept by --set"},
		{"dropped and renamed", colorSizeProduct(), []string{"--drop=Size", "--rename=Size=Fit"}, "both dropped and renamed"},
		{"missing dimension", colorSizeProduct(), []string{"--remove=Colour:Red"}, "does not exist"},
		{"nothing to do", colorSizeProduct(), nil, "nothing to do"},
		{"mixed with the older form", colorSizeProduct(), []string{"--action=add", "--option=Color:Green", "--new-price=1"}, "cannot be mixed"},
		{"drop the last dimension", existingColorProduct(), []string{"--drop=Color"}, "every dimension"},
		{"--existing for no added dimension", colorSizeProduct(), []string{"--add=Color:Green", "--existing=Fit:Slim", "--new-price=1"}, "--existing"},
		{"new value without a price", colorSizeProduct(), []string{"--add=Color:Green"}, "--new-price"},
		{"per-value price not covering a new value", colorSizeProduct(), []string{"--add=Color:Green,Pink", "--new-price=Color:Green=1"}, "Pink"},
		{"new variant without an image", lensProduct("6lenses"), []string{"--add=Color:Red", "--new-price=9"}, "--new-image"},
		{"image not on the product", lensProduct("6lenses"), []string{"--add=Color:Red", "--new-price=9", "--new-image=//x.webp"}, "not one of the product's images"},
		{"per-value price on a missing value", colorSizeProduct(), []string{"--set-price=Size:XL=5"}, "no value"},
		{"per-value forms on two dimensions", colorSizeProduct(), []string{"--set-price=Size:S=5", "--set-price=Color:Red=6"}, "one dimension"},
		{"more than 300 variants", colorSizeProduct(), []string{"--add=A:" + manyValues(80), "--existing=A:a0", "--new-price=1"}, "at most 300"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, captured, err := runEdit(t, tc.product, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
			if captured != nil {
				t.Error("the PUT must not be sent when validation fails")
			}
		})
	}
}

// Every edit names dimensions as read: a rename does not change what the
// other flags refer to.
func TestSetVariants_CombinedNamesResolveAsRead(t *testing.T) {
	_, captured, err := runEdit(t, colorSizeProduct(), "--rename=Color=Colour", "--add=Color:Green", "--new-price=1")
	if err != nil {
		t.Fatal(err)
	}
	if got := optionsJSON(t, captured); !strings.Contains(got, `{"name":"Colour","values":["Red","Blue","Green"]}`) {
		t.Errorf("options: %s", got)
	}
}

func manyValues(n int) string {
	values := make([]string, n)
	for i := range values {
		values[i] = fmt.Sprintf("a%d", i)
	}
	return strings.Join(values, ",")
}
