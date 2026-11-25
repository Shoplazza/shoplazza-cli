package orders

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
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

var refundExecFlags = map[string]string{
	"order-id": "string", "amount": "string",
	"payment-line-id": "string", "note": "string", "return-items": "bool",
}

func TestRefundExecute_DryRunMarksResolvedValues(t *testing.T) {
	cases := []struct {
		name            string
		values          map[string]string
		wantPaymentLine string
		wantLineItems   any
	}{
		{"payment line resolved at run time", map[string]string{"order-id": "ord-1", "amount": "10.00"}, "<resolved-from-step-0>", nil},
		{"payment line given", map[string]string{"order-id": "ord-1", "amount": "10.00", "payment-line-id": "pl-9"}, "pl-9", nil},
		{"return items resolved at run time", map[string]string{"order-id": "ord-1", "amount": "10.00", "return-items": "true"}, "<resolved-from-step-0>", "<resolved-from-step-0>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := refundShortcut.Execute(context.Background(), newOrderExecInput(t, refundExecFlags, c.values, true))
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(result.Plans) != 2 {
				t.Fatalf("expected 2 plans (GET + POST), got %d", len(result.Plans))
			}
			refund := result.Plans[1].Body.(map[string]any)["refund"].(map[string]any)
			payments := refund["refund_payments"].([]map[string]any)
			if got := payments[0]["payment_line_id"]; got != c.wantPaymentLine {
				t.Errorf("payment_line_id = %v, want %v", got, c.wantPaymentLine)
			}
			if got := refund["refund_line_items"]; got != c.wantLineItems {
				t.Errorf("refund_line_items = %v, want %v", got, c.wantLineItems)
			}
		})
	}
}

func TestRefundExecute_AmountMustBePositiveNumber(t *testing.T) {
	cases := []struct {
		amount  string
		dryRun  bool
		wantErr bool
	}{
		{"-5", true, true},
		{"0", true, true},
		{"0.00", true, true},
		{"abc", true, true},
		{"1e3", true, true},
		{"abc", false, true},
		{"29.99", true, false},
		{"10", true, false},
	}
	for _, c := range cases {
		t.Run(c.amount, func(t *testing.T) {
			in := newOrderExecInput(t, refundExecFlags, map[string]string{"order-id": "ord-1", "amount": c.amount}, c.dryRun)
			_, err := refundShortcut.Execute(context.Background(), in)
			if !c.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Param != "--amount" {
				t.Fatalf("want a validation error on --amount, got %#v", err)
			}
			if want := "--amount must be a number greater than 0"; ee.Detail.Message != want {
				t.Fatalf("message = %q, want %q", ee.Detail.Message, want)
			}
		})
	}
}
