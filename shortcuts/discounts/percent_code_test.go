package discounts

import (
	"testing"
)

func percentCodeFlags() map[string]string {
	m := discountCodeFlags()
	m["percent"] = "float"
	return m
}

func TestPercentCodePlan_ZeroPercentErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent": "0"})
	_, err := percentCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when --percent=0")
	}
}

func TestPercentCodePlan_TooHighPercentErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent": "100"})
	_, err := percentCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when --percent=100")
	}
}

func TestPercentCodePlan_OrderTargetSuccess(t *testing.T) {
	in := newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent": "10"})
	_, err := percentCodeShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
