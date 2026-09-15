package discounts

import (
	"testing"
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
