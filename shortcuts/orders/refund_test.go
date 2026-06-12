package orders

import (
	"strings"
	"testing"
)

func TestChoosePaymentLine_SingleAutoSelected(t *testing.T) {
	order := map[string]any{
		"payment_lines": []any{
			map[string]any{"id": "pl-1"},
		},
	}
	got, err := choosePaymentLine(order, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "pl-1" {
		t.Errorf("got %q want pl-1", got)
	}
}

func TestChoosePaymentLine_MultiRequiresExplicit(t *testing.T) {
	order := map[string]any{
		"payment_lines": []any{
			map[string]any{"id": "pl-1"},
			map[string]any{"id": "pl-2"},
		},
	}
	_, err := choosePaymentLine(order, "")
	if err == nil {
		t.Fatal("expected validation error for ambiguous payment_lines")
	}
	if !strings.Contains(err.Error(), "pl-1") || !strings.Contains(err.Error(), "pl-2") {
		t.Errorf("error should list both ids; got: %v", err)
	}
}

func TestChoosePaymentLine_ExplicitMustExist(t *testing.T) {
	order := map[string]any{
		"payment_lines": []any{
			map[string]any{"id": "pl-1"},
			map[string]any{"id": "pl-2"},
		},
	}
	_, err := choosePaymentLine(order, "pl-999")
	if err == nil {
		t.Fatal("expected error for unknown payment_line_id")
	}
}

func TestChoosePaymentLine_ExplicitFound(t *testing.T) {
	order := map[string]any{
		"payment_lines": []any{
			map[string]any{"id": "pl-1"},
			map[string]any{"id": "pl-2"},
		},
	}
	got, err := choosePaymentLine(order, "pl-2")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "pl-2" {
		t.Errorf("got %q want pl-2", got)
	}
}

func TestChoosePaymentLine_NoPaymentLines(t *testing.T) {
	order := map[string]any{"payment_lines": []any{}}
	_, err := choosePaymentLine(order, "")
	if err == nil {
		t.Fatal("expected error when payment_lines is empty")
	}
}

func TestBuildRefundBody_Basic(t *testing.T) {
	body := buildRefundBody("pl-1", "29.99", "", false, nil)
	payments, ok := body["refund_payments"].([]map[string]any)
	if !ok || len(payments) != 1 {
		t.Fatalf("refund_payments shape wrong: %v", body["refund_payments"])
	}
	if payments[0]["payment_line_id"] != "pl-1" {
		t.Errorf("payment_line_id: got %v want pl-1", payments[0]["payment_line_id"])
	}
	if payments[0]["refund_price"] != "29.99" {
		t.Errorf("refund_price: got %v want 29.99", payments[0]["refund_price"])
	}
	if body["refund_total"] != "29.99" {
		t.Errorf("refund_total: got %v want 29.99", body["refund_total"])
	}
	if _, hasNote := body["note"]; hasNote {
		t.Error("note should be absent when empty")
	}
	if _, hasItems := body["refund_line_items"]; hasItems {
		t.Error("refund_line_items should be absent when returnItems=false")
	}
}

func TestBuildRefundBody_WithNote(t *testing.T) {
	body := buildRefundBody("pl-1", "10.00", "damaged", false, nil)
	if body["note"] != "damaged" {
		t.Errorf("note: got %v want damaged", body["note"])
	}
}

func TestBuildRefundBody_ReturnItems(t *testing.T) {
	lineItems := []any{
		map[string]any{"id": "li-1"},
		map[string]any{"id": "li-2"},
	}
	body := buildRefundBody("pl-1", "50.00", "", true, lineItems)
	annotated, ok := body["refund_line_items"].([]map[string]any)
	if !ok {
		t.Fatalf("refund_line_items missing or wrong type: %T", body["refund_line_items"])
	}
	if len(annotated) != 2 {
		t.Fatalf("expected 2 refund_line_items, got %d", len(annotated))
	}
	for _, item := range annotated {
		if item["return_inventory"] != true {
			t.Errorf("return_inventory should be true, got %v", item["return_inventory"])
		}
	}
}

func TestBuildRefundBody_ReturnItemsFalseSkipsLineItems(t *testing.T) {
	lineItems := []any{map[string]any{"id": "li-1"}}
	body := buildRefundBody("pl-1", "5.00", "", false, lineItems)
	if _, ok := body["refund_line_items"]; ok {
		t.Error("refund_line_items should be absent when returnItems=false")
	}
}

func TestBuildRefundBody_ReturnItems_SkipsNonMapAndEmptyID(t *testing.T) {
	lineItems := []any{
		"not-a-map",
		map[string]any{"id": ""},
		map[string]any{"id": "li-valid"},
	}
	body := buildRefundBody("pl-1", "5.00", "", true, lineItems)
	annotated, _ := body["refund_line_items"].([]map[string]any)
	if len(annotated) != 1 || annotated[0]["line_item_id"] != "li-valid" {
		t.Errorf("only valid items should be annotated; got %v", annotated)
	}
}
