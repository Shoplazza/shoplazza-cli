package products

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// ── parseOptionSpecs ──────────────────────────────────────────────────────────

func TestParseOptionSpecs(t *testing.T) {
	dims, err := parseOptionSpecs([]string{"Color:Red, Blue", "Size:S,M,"}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []optionDim{
		{Name: "Color", Values: []string{"Red", "Blue"}},
		{Name: "Size", Values: []string{"S", "M"}},
	}
	if !reflect.DeepEqual(dims, want) {
		t.Errorf("got %+v, want %+v", dims, want)
	}
}

func TestParseOptionSpecs_BareName(t *testing.T) {
	dims, err := parseOptionSpecs([]string{"Size"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 1 || dims[0].Name != "Size" || dims[0].Values != nil {
		t.Errorf("bare name must parse to nil values under remove, got %+v", dims)
	}
	if _, err := parseOptionSpecs([]string{"Size"}, false); err == nil {
		t.Error("bare name must be rejected outside remove")
	}
}

func TestParseOptionSpecs_Errors(t *testing.T) {
	cases := []struct {
		name  string
		specs []string
		want  string
	}{
		{"empty", nil, "--option"},
		{"empty name", []string{":Red"}, `"Name:v1,v2,..."`},
		{"no values", []string{"Color: ,"}, "no values"},
		{"dup name", []string{"Color:Red", "color:Blue"}, "duplicate option name"},
		{"dup value", []string{"Color:Red,red"}, "duplicate value"},
		{"four dims", []string{"A:1", "B:2", "C:3", "D:4"}, "at most 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOptionSpecs(tc.specs, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCartesianOrder(t *testing.T) {
	combos := cartesian([]optionDim{
		{Name: "Color", Values: []string{"Red", "Blue"}},
		{Name: "Size", Values: []string{"S", "M"}},
	})
	want := [][]string{{"Red", "S"}, {"Red", "M"}, {"Blue", "S"}, {"Blue", "M"}}
	if !reflect.DeepEqual(combos, want) {
		t.Errorf("got %v, want %v", combos, want)
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

// ── applyMatrixAction ─────────────────────────────────────────────────────────

func TestApplyMatrixAction(t *testing.T) {
	current := []optionDim{
		{Name: "Color", Values: []string{"Red", "Blue"}},
		{Name: "Size", Values: []string{"S", "M"}},
	}
	t.Run("add value dedups and keeps spelling", func(t *testing.T) {
		out, err := applyMatrixAction(current, "add", []optionDim{{Name: "color", Values: []string{"RED", "White"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out[0].Values, []string{"Red", "Blue", "White"}) {
			t.Errorf("got %v", out[0].Values)
		}
	})
	t.Run("add new dimension appends", func(t *testing.T) {
		out, err := applyMatrixAction(current, "add", []optionDim{{Name: "Fit", Values: []string{"Slim"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 3 || out[2].Name != "Fit" {
			t.Errorf("got %+v", out)
		}
	})
	t.Run("update replaces values", func(t *testing.T) {
		out, err := applyMatrixAction(current, "update", []optionDim{{Name: "Color", Values: []string{"Green"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out[0].Values, []string{"Green"}) {
			t.Errorf("got %v", out[0].Values)
		}
	})
	t.Run("update missing dimension errors with add hint", func(t *testing.T) {
		_, err := applyMatrixAction(current, "update", []optionDim{{Name: "Fit", Values: []string{"Slim"}}})
		if err == nil || !strings.Contains(err.Error(), "--action add") {
			t.Errorf("want existence-assertion error, got %v", err)
		}
	})
	t.Run("remove value", func(t *testing.T) {
		out, err := applyMatrixAction(current, "remove", []optionDim{{Name: "Color", Values: []string{"red"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out[0].Values, []string{"Blue"}) {
			t.Errorf("got %v", out[0].Values)
		}
	})
	t.Run("remove bare name drops dimension", func(t *testing.T) {
		out, err := applyMatrixAction(current, "remove", []optionDim{{Name: "Size"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0].Name != "Color" {
			t.Errorf("got %+v", out)
		}
	})
	t.Run("remove all values drops dimension", func(t *testing.T) {
		out, err := applyMatrixAction(current, "remove", []optionDim{{Name: "Size", Values: []string{"S", "M"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 {
			t.Errorf("got %+v", out)
		}
	})
	t.Run("remove missing is a no-op", func(t *testing.T) {
		out, err := applyMatrixAction(current, "remove", []optionDim{{Name: "Fit"}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out, current) {
			t.Errorf("got %+v", out)
		}
	})
	t.Run("removing every dimension errors", func(t *testing.T) {
		_, err := applyMatrixAction(current, "remove", []optionDim{{Name: "Color"}, {Name: "Size"}})
		if err == nil {
			t.Error("want error")
		}
	})
	t.Run("exceeding 3 dimensions errors", func(t *testing.T) {
		three := append(append([]optionDim{}, current...), optionDim{Name: "Fit", Values: []string{"Slim"}})
		_, err := applyMatrixAction(three, "add", []optionDim{{Name: "Length", Values: []string{"Long"}}})
		if err == nil || !strings.Contains(err.Error(), "at most 3") {
			t.Errorf("want cap error, got %v", err)
		}
	})
}

// ── validation gates ──────────────────────────────────────────────────────────

func TestSetVariants_RequiresID(t *testing.T) {
	in := newSetVariantsInput(t, []string{"Color:Red"}, map[string]string{"action": "add"}, false)
	_, err := setVariantsShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "--id") {
		t.Errorf("want id-required error, got %v", err)
	}
}

func TestSetVariants_ActionValidation(t *testing.T) {
	in := newSetVariantsInput(t, []string{"Color:Red"}, map[string]string{"id": "p-1"}, false)
	_, err := setVariantsShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "--action") {
		t.Errorf("want action error, got %v", err)
	}

	in = newSetVariantsInput(t, []string{"Color:Red"}, map[string]string{"id": "p-1", "action": "merge"}, false)
	_, err = setVariantsShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "unknown --action") {
		t.Errorf("want unknown-action error, got %v", err)
	}
}

func TestSetVariants_BadPrice(t *testing.T) {
	in := newSetVariantsInput(t, []string{"Color:Red"}, map[string]string{"id": "p-1", "action": "add", "price": "abc"}, false)
	if _, err := setVariantsShortcut.Execute(context.Background(), in); err == nil {
		t.Error("want price parse error")
	}
}

// ── update mode ───────────────────────────────────────────────────────────────

// existingColorProduct returns a GET body: options [Color(Red,Blue)], two
// variants with ids/skus/stock.
func existingColorProduct() map[string]any {
	return map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red", "Blue"}, "position": float64(1)},
		},
		"variants": []any{
			map[string]any{"id": "v-red", "option1": "Red", "sku": "SKU-RED", "inventory_quantity": float64(7)},
			map[string]any{"id": "v-blue", "option1": "Blue", "sku": "SKU-BLUE", "inventory_quantity": float64(9)},
		},
	}
}

func TestSetVariants_AddValue(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color:White"}, map[string]string{"id": "p-1", "action": "add", "price": "10"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
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
	if _, hasPrice := red["price"]; hasPrice {
		t.Error("matched variant must not receive --price")
	}
	white := variantByOptions(t, variants, "White", "", "")
	if _, hasID := white["id"]; hasID {
		t.Error("new combo must not carry an id")
	}
	if white["price"] != float64(10) {
		t.Errorf("new combo needs --price, got %v", white["price"])
	}

	if res.Body["inherited"] != 2 || res.Body["created"] != 1 || res.Body["deleted"] != 0 {
		t.Errorf("summary counts wrong: %+v", res.Body)
	}
	if res.Body["action"] != "add" || res.Body["dimension_change"] != false {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

func TestSetVariants_RemoveValueReportsDeleted(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color:Blue"}, map[string]string{"id": "p-1", "action": "remove"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["deleted"] != 1 || res.Body["inherited"] != 1 {
		t.Fatalf("summary: want deleted=1 inherited=1, got %+v", res.Body)
	}
	detail := res.Body["deleted_detail"].([]map[string]any)
	if detail[0]["id"] != "v-blue" || detail[0]["sku"] != "SKU-BLUE" || detail[0]["inventory_quantity"] != 9 {
		t.Errorf("deleted_detail must carry the old variant's id/sku/stock: %+v", detail[0])
	}
}

// Removing something already absent converges without a write.
func TestSetVariants_RemoveMissingIsNoOpWithoutWrite(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color:Green"}, map[string]string{"id": "p-1", "action": "remove"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if captured != nil {
		t.Error("a no-change delta must not send the PUT")
	}
	if res.Body["no_change"] != true || res.Body["inherited"] != 2 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

// Adding an already-present value converges too (retry safety).
func TestSetVariants_AddExistingValueConverges(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color:Red"}, map[string]string{"id": "p-1", "action": "add", "price": "10"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if captured != nil {
		t.Error("adding an existing value must not send the PUT")
	}
	if res.Body["no_change"] != true {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

func TestSetVariants_NewCombosWithoutPriceErrors(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color:White"}, map[string]string{"id": "p-1", "action": "add"}, false)
	in.Client = client.New(srv.URL)

	_, err := setVariantsShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "--price") {
		t.Errorf("want price-required error naming a combo, got %v", err)
	}
	if captured != nil {
		t.Error("the PUT must not be sent when validation fails")
	}
}

// Value matching is trim/case-insensitive, but the written value is the caller's.
func TestSetVariants_UpdateNormalizedValueMatch(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color: RED , blue "}, map[string]string{"id": "p-1", "action": "update"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["inherited"] != 2 || res.Body["deleted"] != 0 {
		t.Errorf("case/space variants must still match: %+v", res.Body)
	}
	variants := capturedVariants(t, captured)
	if variants[0]["option1"] != "RED" {
		t.Errorf("written value must keep the caller's spelling, got %v", variants[0]["option1"])
	}
	red := variantByOptions(t, variants, "RED", "", "")
	if red["id"] != "v-red" {
		t.Errorf("respelled value must keep the old id, got %v", red["id"])
	}
}

// A value rename is delete+create.
func TestSetVariants_UpdateRenameIsDeletePlusCreate(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Color:Crimson,Blue"}, map[string]string{"id": "p-1", "action": "update", "price": "10"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body["created"] != 1 || res.Body["inherited"] != 1 || res.Body["deleted"] != 1 {
		t.Errorf("rename must be delete+create: %+v", res.Body)
	}
}

// ── dimension change → rebuild ────────────────────────────────────────────────

func TestSetVariants_AddDimensionRebuilds(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Size:S,M"}, map[string]string{"id": "p-1", "action": "add", "price": "12"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	variants := capturedVariants(t, captured)
	if len(variants) != 4 {
		t.Fatalf("want 4 variants (Color×Size), got %d", len(variants))
	}
	for _, v := range variants {
		if _, hasID := v["id"]; hasID {
			t.Errorf("rebuild must not carry any old id, got %+v", v)
		}
		if v["price"] != float64(12) {
			t.Errorf("every rebuilt variant needs --price, got %v", v["price"])
		}
	}
	// Existing dimension stays in position 1 (Fatal inside if missing).
	variantByOptions(t, variants, "Red", "S", "")

	if res.Body["dimension_change"] != true || res.Body["created"] != 4 || res.Body["inherited"] != 0 || res.Body["deleted"] != 2 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
	detail := res.Body["deleted_detail"].([]map[string]any)
	if len(detail) != 2 || detail[0]["sku"] != "SKU-RED" {
		t.Errorf("rebuild must list every deleted variant with sku/stock: %+v", detail)
	}
}

func TestSetVariants_RemoveWholeDimensionRebuilds(t *testing.T) {
	product := map[string]any{
		"id": "p-1",
		"options": []any{
			map[string]any{"name": "Color", "values": []any{"Red"}, "position": float64(1)},
			map[string]any{"name": "Size", "values": []any{"S", "M"}, "position": float64(2)},
		},
		"variants": []any{
			map[string]any{"id": "v-1", "option1": "Red", "option2": "S", "sku": "K1", "inventory_quantity": float64(3)},
			map[string]any{"id": "v-2", "option1": "Red", "option2": "M", "sku": "K2", "inventory_quantity": float64(4)},
		},
	}
	var captured map[string]any
	srv := matrixServer(t, product, &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Size"}, map[string]string{"id": "p-1", "action": "remove", "price": "9"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	variants := capturedVariants(t, captured)
	if len(variants) != 1 {
		t.Fatalf("want 1 variant (Red), got %d", len(variants))
	}
	if res.Body["dimension_change"] != true || res.Body["deleted"] != 2 || res.Body["created"] != 1 {
		t.Errorf("summary wrong: %+v", res.Body)
	}
}

func TestSetVariants_UpdateMissingDimensionErrors(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Fit:Slim"}, map[string]string{"id": "p-1", "action": "update", "price": "9"}, false)
	in.Client = client.New(srv.URL)

	_, err := setVariantsShortcut.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "--action add") {
		t.Errorf("want existence-assertion error, got %v", err)
	}
	if captured != nil {
		t.Error("the PUT must not be sent when validation fails")
	}
}

// The sku template may reference dimensions NOT named in --option (they exist
// on the product); validation must run against the merged matrix.
func TestSetVariants_SkuTemplateValidatedAgainstMergedDims(t *testing.T) {
	var captured map[string]any
	srv := matrixServer(t, existingColorProduct(), &captured, nil)
	defer srv.Close()

	in := newSetVariantsInput(t, []string{"Size:S"}, map[string]string{
		"id": "p-1", "action": "add", "price": "9", "sku-template": "X-{Color}-{Size}",
	}, false)
	in.Client = client.New(srv.URL)

	if _, err := setVariantsShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("template referencing an existing dim must validate: %v", err)
	}
	variants := capturedVariants(t, captured)
	if variants[0]["sku"] != "X-Red-S" {
		t.Errorf("template must render merged dims, got %v", variants[0]["sku"])
	}
}

// ── dry-run ───────────────────────────────────────────────────────────────────

func TestSetVariants_DryRunUpdate(t *testing.T) {
	in := newSetVariantsInput(t, []string{"Color:White"}, map[string]string{"id": "p-1", "action": "add", "price": "9"}, true)
	res, err := setVariantsShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plans) != 2 || res.Plans[0].Method != "GET" || res.Plans[1].Method != "PUT" {
		t.Fatalf("want [GET, PUT] plans, got %+v", res.Plans)
	}
}

// ── detail cap ────────────────────────────────────────────────────────────────

func TestSetVariants_DetailCap(t *testing.T) {
	oldVars := make([]any, detailCap+20)
	for i := range oldVars {
		oldVars[i] = map[string]any{
			"id": fmt.Sprintf("v-%d", i), "option1": fmt.Sprintf("c%d", i), "inventory_quantity": float64(1),
		}
	}
	product := map[string]any{
		"id":       "p-1",
		"options":  []any{map[string]any{"name": "Color", "position": float64(1)}},
		"variants": oldVars,
	}
	var captured map[string]any
	srv := matrixServer(t, product, &captured, nil)
	defer srv.Close()

	// Adding a dimension rebuilds and deletes all detailCap+20 old variants.
	in := newSetVariantsInput(t, []string{"Size:y"}, map[string]string{"id": "p-1", "action": "add", "price": "1"}, false)
	in.Client = client.New(srv.URL)

	res, err := setVariantsShortcut.Execute(context.Background(), in)
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
