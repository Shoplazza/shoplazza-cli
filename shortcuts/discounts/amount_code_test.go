package discounts

import (
	"errors"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// amountCodeFlags includes all flags used by amountCodeShortcut.
func amountCodeFlags() map[string]string {
	m := discountCodeFlags()
	m["off"] = "float"
	m["limit-order-once"] = "bool"
	return m
}

func TestAmountCodePlan_ZeroOffErrors(t *testing.T) {
	in := newPlanInput(t, "amount-code", amountCodeFlags(), map[string]string{"target": "order", "off": "0"})
	_, err := amountCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when --off=0")
	}
}

func TestAmountCodePlan_NegativeOffErrors(t *testing.T) {
	in := newPlanInput(t, "amount-code", amountCodeFlags(), map[string]string{"target": "order", "off": "-5"})
	_, err := amountCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when --off<0")
	}
}

func TestAmountCodePlan_OrderTargetSuccess(t *testing.T) {
	in := newPlanInput(t, "amount-code", amountCodeFlags(), map[string]string{"target": "order", "off": "10"})
	_, err := amountCodeShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAmountCodePlan_LimitOrderOnceFalse(t *testing.T) {
	in := newPlanInput(t, "amount-code", amountCodeFlags(), map[string]string{"target": "order", "off": "10", "limit-order-once": "false"})
	_, err := amountCodeShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAmountCodePlan_OffBelowMinAmount(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]string
		wantErr bool
	}{
		{"off above min-amount", map[string]string{"target": "order", "off": "10", "min-amount": "5"}, true},
		{"off equals min-amount", map[string]string{"target": "order", "off": "10", "min-amount": "10"}, true},
		{"off below min-amount", map[string]string{"target": "order", "off": "10", "min-amount": "15"}, false},
		{"no min-amount", map[string]string{"target": "order", "off": "10"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := amountCodeShortcut.Plan(newPlanInput(t, "amount-code", amountCodeFlags(), c.values))
			if !c.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Param != "--off" {
				t.Fatalf("want a validation error on --off, got %#v", err)
			}
			if want := "--off must be less than --min-amount"; ee.Detail.Message != want {
				t.Fatalf("message = %q, want %q", ee.Detail.Message, want)
			}
		})
	}
}
