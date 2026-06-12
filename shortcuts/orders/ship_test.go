package orders

import (
	"encoding/json"
	"strings"
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

func TestParseLineItemsArg_ValidPairs(t *testing.T) {
	got, err := parseLineItemsArg("li-1:2,li-2:5")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got["li-1"] != 2 || got["li-2"] != 5 {
		t.Errorf("parsed pairs wrong: %v", got)
	}
}

func TestParseLineItemsArg_InvalidFormat(t *testing.T) {
	_, err := parseLineItemsArg("li-1=2")
	if err == nil {
		t.Fatal("expected error for missing colon separator")
	}
}

func TestParseLineItemsArg_NonNumericQty(t *testing.T) {
	_, err := parseLineItemsArg("li-1:abc")
	if err == nil {
		t.Fatal("expected error for non-numeric qty")
	}
}

func TestBuildShipBody_AllFulfillableByDefault(t *testing.T) {
	order := map[string]any{
		"line_items": []any{
			map[string]any{"id": "li-1", "fulfillable_quantity": float64(2)},
			map[string]any{"id": "li-2", "fulfillable_quantity": float64(1)},
		},
	}
	body, err := buildShipBody(order, "", "T1", "DHL", "", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	items, ok := body["line_items"].([]map[string]any)
	if !ok {
		t.Fatalf("line_items not []map[string]any; got %T", body["line_items"])
	}
	if len(items) != 2 {
		t.Fatalf("line_items len: got %d want 2", len(items))
	}
	// Map iteration is non-deterministic — verify by id rather than index.
	byID := map[string]int{}
	for _, it := range items {
		id, _ := it["id"].(string)
		qty, _ := it["ship_quantity"].(int)
		byID[id] = qty
	}
	if byID["li-1"] != 2 || byID["li-2"] != 1 {
		t.Errorf("expected li-1=2 li-2=1; got %v", byID)
	}
}

func TestBuildShipBody_QtyExceedsFulfillable(t *testing.T) {
	order := map[string]any{
		"line_items": []any{
			map[string]any{"id": "li-1", "fulfillable_quantity": float64(2)},
		},
	}
	_, err := buildShipBody(order, "li-1:5", "T1", "", "", false)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "li-1") {
		t.Errorf("error should mention the offending line item; got: %v", err)
	}
}

func TestShipShortcut_DeclarativeFields(t *testing.T) {
	if shipShortcut.Service != "orders" || shipShortcut.Command != "+ship" {
		t.Errorf("identity: got %q/%q want orders/+ship", shipShortcut.Service, shipShortcut.Command)
	}
	if shipShortcut.Execute == nil {
		t.Fatal("shipShortcut.Execute is nil; multi-step orchestration requires Execute")
	}
	if shipShortcut.Plan != nil {
		t.Fatal("shipShortcut.Plan should be nil (use Execute for multi-step)")
	}
	if err := common.ValidateShortcut(shipShortcut); err != nil {
		t.Errorf("ValidateShortcut: %v", err)
	}
}

func TestDryRunLineItemsFromArg_Valid(t *testing.T) {
	items, err := dryRunLineItemsFromArg("li-1:3,li-2:1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	byID := map[string]int{}
	for _, m := range items {
		id, _ := m["id"].(string)
		qty, _ := m["ship_quantity"].(int)
		byID[id] = qty
	}
	if byID["li-1"] != 3 || byID["li-2"] != 1 {
		t.Errorf("item quantities wrong: %v", byID)
	}
}

func TestDryRunLineItemsFromArg_InvalidPropagatesError(t *testing.T) {
	_, err := dryRunLineItemsFromArg("bad-format")
	if err == nil {
		t.Fatal("expected error for malformed arg")
	}
}

// TestAsInt covers each numeric form asInt accepts plus the reject paths.
func TestAsInt(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{"json.Number integer", json.Number("42"), 42, true},
		{"json.Number non-integer", json.Number("4.2"), 0, false},
		{"float64", float64(7), 7, true},
		{"int", 5, 5, true},
		{"int64", int64(9), 9, true},
		{"string is rejected", "nope", 0, false},
		{"nil is rejected", nil, 0, false},
	}
	for _, c := range cases {
		got, ok := asInt(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: asInt(%v) = (%d, %t), want (%d, %t)", c.name, c.in, got, ok, c.want, c.ok)
		}
	}
}

// ── extractFulfillableQuantities edge cases ───────────────────────────────────

func TestExtractFulfillableQuantities_MissingLineItems(t *testing.T) {
	_, err := extractFulfillableQuantities(map[string]any{"other": "val"})
	if err == nil {
		t.Error("expected error for missing line_items")
	}
}

func TestExtractFulfillableQuantities_NonMapItem(t *testing.T) {
	order := map[string]any{"line_items": []any{
		"not-a-map",
		map[string]any{"id": "li-1", "fulfillable_quantity": float64(3)},
	}}
	out, err := extractFulfillableQuantities(order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["li-1"] != 3 {
		t.Errorf("li-1: got %d want 3", out["li-1"])
	}
}

func TestExtractFulfillableQuantities_EmptyID(t *testing.T) {
	order := map[string]any{"line_items": []any{
		map[string]any{"id": "", "fulfillable_quantity": float64(1)},
	}}
	out, err := extractFulfillableQuantities(order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty-id items should be skipped; got %v", out)
	}
}

func TestExtractFulfillableQuantities_QuantityFallback(t *testing.T) {
	order := map[string]any{"line_items": []any{
		map[string]any{"id": "li-1", "quantity": float64(5)},
	}}
	out, err := extractFulfillableQuantities(order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["li-1"] != 5 {
		t.Errorf("quantity fallback: got %d want 5", out["li-1"])
	}
}

func TestExtractFulfillableQuantities_BothZero(t *testing.T) {
	order := map[string]any{"line_items": []any{
		map[string]any{"id": "li-1", "fulfillable_quantity": float64(0), "quantity": float64(0)},
	}}
	out, err := extractFulfillableQuantities(order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := out["li-1"]; ok {
		t.Error("zero-quantity item should be skipped")
	}
}

// ── buildShipBody edge cases ──────────────────────────────────────────────────

func TestBuildShipBody_LineItemNotFound(t *testing.T) {
	order := map[string]any{"line_items": []any{
		map[string]any{"id": "li-1", "fulfillable_quantity": float64(2)},
	}}
	_, err := buildShipBody(order, "unknown-id:1", "T1", "", "", false)
	if err == nil {
		t.Error("expected error for non-existent line item ID")
	}
}

func TestBuildShipBody_NotifySetsEmailFlag(t *testing.T) {
	order := map[string]any{"line_items": []any{
		map[string]any{"id": "li-1", "fulfillable_quantity": float64(2)},
	}}
	body, err := buildShipBody(order, "", "T1", "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["send_email"] != true {
		t.Errorf("send_email should be true when notify=true, got %v", body["send_email"])
	}
}

func TestParseLineItemsArg_ZeroQty(t *testing.T) {
	_, err := parseLineItemsArg("li-1:0")
	if err == nil {
		t.Error("expected error for qty=0")
	}
}
