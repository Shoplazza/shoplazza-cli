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

// ── values on an unchanged dimension set ──────────────────────────────────────

func TestSetVariants_AddValue(t *testing.T) {
	res, captured, err := runEdit(t, existingColorProduct(), "--add=Color:White", "--new-price=10")
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
		t.Errorf("new combo needs --new-price, got %v", white["price"])
	}

	if res.Body["kept"] != 2 || res.Body["created"] != 1 || res.Body["deleted"] != 0 || res.Body["price_applied"] != 1 {
		t.Errorf("summary counts wrong: %+v", res.Body)
	}
	if res.Body["dimension_change"] != false {
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
			res, captured, err := runEdit(t, colorSizeProduct("Red/M"), "--add="+tc.option, "--new-price=5")
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
	res, _, err := runEdit(t, existingColorProduct(), "--remove=Color:Blue")
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

// Removing a value never needs --new-price, and a value left with no variant is dropped.
func TestSetVariants_RemoveValueOnSparseProductPrunes(t *testing.T) {
	res, captured, err := runEdit(t, colorSizeProduct("Red/M"), "--remove=Size:S")
	if err != nil {
		t.Fatalf("a pure delete creates nothing, so --new-price must not be required: %v", err)
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
	res, captured, err := runEdit(t, caseTwinProduct(), "--add=Color:Blue", "--new-price=5")
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
	res, captured, err := runEdit(t, caseTwinProduct(), "--remove=Size:s")
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
func TestSetVariants_SetRespellsInPlace(t *testing.T) {
	res, captured, err := runEdit(t, existingColorProduct(), "--set=Color: RED , blue ")
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

// New values are laid out against the combinations that exist before the
// replaced values go.
func TestSetVariants_SetReplacingEveryValue(t *testing.T) {
	res, captured, err := runEdit(t, colorSizeProduct(), "--set=Color:Green", "--new-price=5")
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
	res, captured, err := runEdit(t, product, "--add=Color:Green", "--new-price=7")
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

// ── dimension changes ─────────────────────────────────────────────────────────

// Existing variants take the --existing value and keep their id;
// the other values get siblings copying everything but sku/barcode/stock.
func TestSetVariants_AddDimensionKeepsExistingVariants(t *testing.T) {
	res, captured, err := runEdit(t, existingColorProduct(), "--add=Size:S,M", "--existing=Size:S")
	if err != nil {
		t.Fatalf("every sibling has a price to copy, so --new-price must not be required: %v", err)
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
		t.Error("price_applied is reported only when --new-price is passed")
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

// A dimension can be dropped when every remaining combination keeps one variant.
func TestSetVariants_DropDimensionOneToOne(t *testing.T) {
	res, captured, err := runEdit(t, colorSizeProduct("Red/M", "Blue/S"), "--drop=Size")
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
	res, captured, err := runEdit(t, colorSizeProduct(), "--rename=Color:red=Wine")
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
	res, captured, err := runEdit(t, colorSizeProduct(), "--rename=Color=Colour")
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
	_, captured, err := runEdit(t, imageProduct(), "--add=Size:S,M", "--existing=Size:S")
	if err != nil {
		t.Fatal(err)
	}
	if v := variantByOptions(t, capturedVariants(t, captured), "Blue", "M", ""); v["image_id"] != "img-2" {
		t.Errorf("Blue/M must copy Blue's image, got %+v", v)
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
		args    []string
		want    string
	}{
		{"brand-new value needs a price mid-reshape",
			redOnlyProduct(map[string]any{"id": "v-red", "option1": "Red", "price": "88.00"}),
			[]string{"--add=Color:Green", "--add=Size:S"}, "--new-price"},
		{"sibling with no price to copy needs one",
			redOnlyProduct(map[string]any{"id": "v-red", "option1": "Red", "sku": "SKU-RED"}),
			[]string{"--add=Size:S,M", "--existing=Size:S"}, "--new-price"},
		{"value differing only by case", existingColorProduct(), []string{"--add=Color:red", "--new-price=5"}, "only by case"},
		{"remove by a case-folded spelling", existingColorProduct(), []string{"--remove=Color:red"}, `did you mean "Red"?`},
		{"remove a missing value", existingColorProduct(), []string{"--remove=Color:Green"}, "existing: Red,Blue"},
		{"remove every value of a dimension", colorSizeProduct(), []string{"--remove=Size:S,M"}, `--drop "Size"`},
		{"rename onto a case twin", existingColorProduct(), []string{"--rename=Color:Red=blue"}, "only by case"},
		{"rename onto another dimension", colorSizeProduct(), []string{"--rename=Color=size"}, "already exists"},
		{"rename a missing value", existingColorProduct(), []string{"--rename=Color:Green=Lime"}, "no value"},
		{"repeat of an applied rename", existingColorProduct(), []string{"--rename=Color:Crimson=Red"}, "was it renamed already?"},
		{"repeat of an applied dimension rename", existingColorProduct(), []string{"--rename=Colour=Color"}, "was it renamed already?"},
		{"rename onto an existing value", existingColorProduct(), []string{"--rename=Color:Red=Blue"}, `"Blue" already exists`},
		{"rename swap", existingColorProduct(), []string{"--rename=Color:Red=Blue,Blue=Red"}, "already exists"},
		{"rename chain", existingColorProduct(), []string{"--rename=Color:Red=Green,Green=Yellow"}, `no value "Green"`},
		{"two values renamed to one", existingColorProduct(), []string{"--rename=Color:Red=X,Blue=X"}, `two values to "X"`},
		{"dimension rename chain", colorSizeProduct(), []string{"--rename=Color=Colour", "--rename=Colour:Red=Wine"}, `"Colour" does not exist`},
		{"set matching one value twice by case", existingColorProduct(), []string{"--set=Color:red,RED", "--new-price=5"}, "both match"},
		{"set adding a case twin", existingColorProduct(), []string{"--set=Color:Red,Blue,red", "--new-price=5"}, "differ only by case"},
		{"new dimension with case twins", existingColorProduct(), []string{"--add=Size:S,s", "--existing=Size:S"}, "differ only by case"},
		{"negative --set-stock", existingColorProduct(), []string{"--add=Color:Green", "--new-price=5", "--set-stock=-1"}, "--set-stock"},
		{"two variants on one combination", map[string]any{"id": "p-1", "options": []any{},
			"variants": []any{map[string]any{"id": "v-1", "price": "1"}, map[string]any{"id": "v-2", "price": "1"}}},
			[]string{"--add=Color:Red"}, "both have options"},
		{"new variant on a stale stored combination", staleSlotProduct(), []string{"--add=Size:M,S", "--existing=Size:M", "--new-price=5"}, "stale options"},
		{"a dimension with no values", valuelessDimension, []string{"--add=Size:S,M"}, `"Color"`},
		{"a fourth dimension", threeDims, []string{"--add=D:d", "--new-price=5"}, "at most 3"},
		{"--new-image on a product without variant images", existingColorProduct(), []string{"--add=Color:Green", "--new-price=5", "--new-image=x"}, "--new-image applies only"},
		{"--sku-template rendering duplicates", existingColorProduct(), []string{"--add=Size:S,M", "--existing=Size:S", "--sku-template=TS-{Color}"}, "reference every dimension"},
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

// --new-price never touches an existing price; passing it when nothing needs it warns.
func TestSetVariants_UnusedPriceWarns(t *testing.T) {
	res, captured, err := runEdit(t, existingColorProduct(), "--add=Size:S", "--new-price=19.9")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range capturedVariants(t, captured) {
		mustNotHave(t, v, "price")
	}
	if res.Body["price_applied"] != 0 || res.Body["warnings"] == nil {
		t.Errorf("an unused --new-price must be reported: %+v", res.Body)
	}
}

// ── sku template ──────────────────────────────────────────────────────────────

// A template may reference dimensions not named in --add, so validation
// runs against the merged matrix.
func TestSetVariants_SkuTemplateValidatedAgainstMergedDims(t *testing.T) {
	_, captured, err := runEdit(t, existingColorProduct(), "--add=Size:S", "--sku-template=X-{Color}-{Size}")
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
	res, captured, err := dryRunEdit(t, existingColorProduct(), "--add=Color:White", "--new-price=9")
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
	res, _, err := dryRunEdit(t, existingColorProduct(), "--add=Color:Red", "--new-price=9")
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

	res, _, err := runEdit(t, product, "--remove=Color:"+strings.Join(drop, ","))
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
	res, captured, err := runEdit(t, idleValueProduct(), "--remove=Color:Blue")
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

// Adding or setting a value that exists without variants gives it variants.
func TestSetVariants_IdleValueGetsVariants(t *testing.T) {
	for _, arg := range []string{"--add=Color:Green", "--set=Color:Red,Blue,Green"} {
		t.Run(arg, func(t *testing.T) {
			res, captured, err := runEdit(t, idleValueProduct(), arg, "--new-price=5")
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
	_, captured, err := runEdit(t, p, "--remove=Color:RED")
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
	res, captured, err := runEdit(t, product, "--add=Color:Red")
	if err != nil || captured != nil || res.Body["no_change"] != true {
		t.Fatalf("want no_change without a write, got err=%v write=%v %+v", err, captured != nil, res.Body)
	}
	res, _, err = runEdit(t, product, "--add=Color:Green", "--new-price=2")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Body["remapped_detail"]; ok {
		t.Errorf("re-slotting alone is not a move: %+v", res.Body["remapped_detail"])
	}
}

// An unused --new-price is reported even when nothing is written.
func TestSetVariants_UnusedPriceOnNoOp(t *testing.T) {
	res, _, err := runEdit(t, existingColorProduct(), "--add=Color:Red", "--new-price=9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["no_change"] != true || res.Body["price_applied"] != 0 || res.Body["warnings"] == nil {
		t.Errorf("want no_change with the unused --new-price reported: %+v", res.Body)
	}
}

// Several --set payloads: a value one payload lists keeps variants even when a
// later payload removes the ones it had, whatever the order.
func TestSetVariants_SetPayloadsKeepListedValues(t *testing.T) {
	for _, args := range [][]string{
		{"--set=Size:S,L", "--set=Color:Blue"},
		{"--set=Color:Blue", "--set=Size:S,L"},
	} {
		t.Run(strings.Join(args, "+"), func(t *testing.T) {
			_, captured, err := runEdit(t, colorSizeProduct("Blue/S"), append(args, "--new-price=5")...)
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

// ── combined edits ────────────────────────────────────────────────────────────

// runEdit runs the shortcut against a stub serving product and returns the
// captured write body (nil when nothing was written).
func runEdit(t *testing.T, product map[string]any, args ...string) (common.ExecResult, map[string]any, error) {
	t.Helper()
	srv := newEditServer(t)
	return srv.run(t, product, args...)
}

func dryRunEdit(t *testing.T, product map[string]any, args ...string) (common.ExecResult, map[string]any, error) {
	t.Helper()
	srv := newEditServer(t)
	srv.dryRun = true
	return srv.run(t, product, args...)
}

// editServer serves one product per run from a single listener, so a loop of
// runs does not open a socket each.
type editServer struct {
	product  map[string]any
	captured map[string]any
	url      string
	dryRun   bool
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
	in := common.ExecInput{Flags: common.NewCobraFlagSet(cmd), Client: client.New(s.url), DryRun: s.dryRun}
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
