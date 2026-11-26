package discounts

import (
	"strings"
	"testing"
)

func percentCodeFlags() map[string]string {
	m := discountCodeFlags()
	m["percent-off"] = "float"
	return m
}

func TestPercentCodePlan_ZeroPercentErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent-off": "0"})
	_, err := percentCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when --percent-off=0")
	}
}

func TestPercentCodePlan_TooHighPercentErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent-off": "100"})
	_, err := percentCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when --percent-off=100")
	}
}

func TestPercentCodePlan_OrderTargetSuccess(t *testing.T) {
	in := newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent-off": "10"})
	p, err := percentCodeShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	code := p.Body.(map[string]any)["discount"].(map[string]any)["discount_info"].(map[string]any)["discount_codes"].([]string)[0]
	if !strings.Contains(p.DryRunHint, "--code "+code) {
		t.Errorf("generated code: hint %q should name --code %s", p.DryRunHint, code)
	}

	in = newPlanInput(t, "percent-code", percentCodeFlags(), map[string]string{"target": "order", "percent-off": "10", "code": "SAVE10"})
	if p, _ = percentCodeShortcut.Plan(in); p.DryRunHint != "" {
		t.Errorf("--code given: want no hint, got %q", p.DryRunHint)
	}
}
