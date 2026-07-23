package products

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

func TestStockShortcut_DeclarativeShape(t *testing.T) {
	if stockShortcut.Execute == nil {
		t.Fatal("+stock requires Execute (multi-step)")
	}
	if err := common.ValidateShortcut(stockShortcut); err != nil {
		t.Errorf("validate: %v", err)
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

func TestExtractInventoryLevelStock_Present(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"stock": float64(42)},
		},
	}
	got, err := extractInventoryLevelStock(resp)
	if err != nil || got != 42 {
		t.Errorf("got (%d, %v) want (42, nil)", got, err)
	}
}

func TestExtractInventoryLevelStock_ZeroWhenMissing(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"other_field": "x"},
		},
	}
	got, err := extractInventoryLevelStock(resp)
	if err != nil || got != 0 {
		t.Errorf("missing stock: got (%d, %v) want (0, nil)", got, err)
	}
}

func TestExtractInventoryLevelStock_EmptyList(t *testing.T) {
	resp := map[string]any{"inventory_levels": []any{}}
	got, err := extractInventoryLevelStock(resp)
	if err != nil || got != 0 {
		t.Errorf("empty list: got (%d, %v) want (0, nil)", got, err)
	}
}

func TestExtractInventoryLevelStock_MissingKey(t *testing.T) {
	_, err := extractInventoryLevelStock(map[string]any{})
	if err == nil {
		t.Error("expected error when inventory_levels key missing")
	}
}

func TestExtractInventoryLevelStock_BadStockType(t *testing.T) {
	resp := map[string]any{
		"inventory_levels": []any{
			map[string]any{"stock": "not-a-number"},
		},
	}
	_, err := extractInventoryLevelStock(resp)
	if err == nil {
		t.Error("expected error when stock has unexpected type")
	}
}

func TestWrapSingleLevel_Normal(t *testing.T) {
	row := map[string]any{"id": "il-1", "stock": float64(10)}
	resp := map[string]any{"inventory_levels": []any{row}}
	got := wrapSingleLevel(resp)
	wrapped, ok := got["inventory_level"].(map[string]any)
	if !ok {
		t.Fatalf("inventory_level not a map: %T", got["inventory_level"])
	}
	if wrapped["id"] != "il-1" {
		t.Errorf("id: got %v want il-1", wrapped["id"])
	}
}

func TestWrapSingleLevel_Empty(t *testing.T) {
	resp := map[string]any{"inventory_levels": []any{}}
	got := wrapSingleLevel(resp)
	wrapped, ok := got["inventory_level"].(map[string]any)
	if !ok || len(wrapped) != 0 {
		t.Errorf("empty list: expected empty map, got %v", got["inventory_level"])
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
