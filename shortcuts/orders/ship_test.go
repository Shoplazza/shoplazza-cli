package orders

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
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

func shippedOrder() map[string]any {
	return map[string]any{
		"line_items": []any{
			map[string]any{"id": "li-1", "quantity": float64(7)},
			map[string]any{"id": "li-2", "quantity": float64(1)},
			map[string]any{"id": "li-3", "quantity": float64(7)},
		},
		"fulfillments": []any{
			map[string]any{"status": "shipped", "line_items": []any{
				map[string]any{"id": "li-1", "ship_quantity": float64(5)},
				map[string]any{"id": "li-3", "ship_quantity": float64(7)},
			}},
			map[string]any{"status": "cancelled", "line_items": []any{
				map[string]any{"id": "li-2", "ship_quantity": float64(1)},
			}},
		},
	}
}

func TestShipLineItems_AllFulfillableByDefault(t *testing.T) {
	items, err := shipLineItems(shippedOrder(), "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("fully shipped li-3 must be left out; got %v", items)
	}
	// Map iteration is non-deterministic — verify by id rather than index.
	byID := map[string]int{}
	for _, it := range items {
		id, _ := it["id"].(string)
		qty, _ := it["ship_quantity"].(int)
		byID[id] = qty
	}
	if byID["li-1"] != 2 || byID["li-2"] != 1 {
		t.Errorf("want ordered minus shipped, ignoring cancelled fulfillments: li-1=2 li-2=1; got %v", byID)
	}

	order := shippedOrder()
	order["line_items"] = order["line_items"].([]any)[2:]
	if _, err := shipLineItems(order, ""); err == nil || !strings.Contains(err.Error(), "already been shipped") {
		t.Errorf("an order with nothing left to ship must be refused, got %v", err)
	}
}

func TestShipLineItems_QtyExceedsFulfillable(t *testing.T) {
	for _, arg := range []string{"li-1:3", "li-3:1"} {
		_, err := shipLineItems(shippedOrder(), arg)
		if err == nil {
			t.Fatalf("%s: expected validation error", arg)
		}
		if !strings.Contains(err.Error(), arg[:4]) || !strings.Contains(err.Error(), "left to ship") {
			t.Errorf("%s: error should name the line item and what is left to ship; got: %v", arg, err)
		}
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
	for _, f := range shipShortcut.Flags {
		if f.Name == "notify" {
			t.Error("+ship must not offer --notify: creating a fulfillment has no field to email the buyer")
		}
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
		map[string]any{"id": "li-1", "quantity": float64(3)},
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
		map[string]any{"id": "", "quantity": float64(1)},
	}}
	out, err := extractFulfillableQuantities(order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty-id items should be skipped; got %v", out)
	}
}

func TestExtractFulfillableQuantities_OrderedMinusShipped(t *testing.T) {
	out, err := extractFulfillableQuantities(shippedOrder())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]int{"li-1": 2, "li-2": 1, "li-3": 0}
	if len(out) != len(want) || out["li-1"] != 2 || out["li-2"] != 1 || out["li-3"] != 0 {
		t.Errorf("got %v want %v", out, want)
	}
}

// ── shipLineItems edge cases ──────────────────────────────────────────────────

func TestShipLineItems_LineItemNotFound(t *testing.T) {
	_, err := shipLineItems(shippedOrder(), "unknown-id:1")
	if err == nil {
		t.Error("expected error for non-existent line item ID")
	}
}

func TestParseLineItemsArg_ZeroQty(t *testing.T) {
	_, err := parseLineItemsArg("li-1:0")
	if err == nil {
		t.Error("expected error for qty=0")
	}
}

var shipExecFlags = declaredFlags(shipShortcut)

func TestShipExecute_DryRunReturnsTwoPlans(t *testing.T) {
	in := newOrderExecInput(t, shipExecFlags, map[string]string{
		"order-id": "ord-1", "tracking": "TRK123",
	}, true)
	result, err := shipShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans (GET + POST), got %d", len(result.Plans))
	}
}

func TestShipExecute_DryRunBody(t *testing.T) {
	in := newOrderExecInput(t, shipExecFlags, map[string]string{
		"order-id": "ord-1", "tracking": "TRK123", "line-items": "li-1:1",
		"company-code": "sf", "tracking-url": "https://t.example/TRK123",
	}, true)
	result, err := shipShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body := result.Plans[1].Body.(map[string]any)["fulfillment"].(map[string]any)
	if _, ok := body["send_email"]; ok {
		t.Errorf("the create body must not carry send_email (the API has no such field): %v", body)
	}
	if body["tracking_company_code"] != "sf" || body["tracking_url"] != "https://t.example/TRK123" {
		t.Errorf("--company-code / --tracking-url must reach the body: %v", body)
	}
}
