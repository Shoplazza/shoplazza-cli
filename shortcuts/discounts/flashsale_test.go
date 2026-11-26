package discounts

import (
	"strings"
	"testing"
)

func flashsaleFlags() map[string]string {
	return map[string]string{
		"value": "float", "type": "string", "variants": "stringslice", "collections": "stringslice",
		"price-rule": "string", "limit-user-variant": "int", "limit-user-product": "int",
		"limit-user-all": "int", "stock": "int", "combines": "stringslice",
		"name": "string", "start": "string", "end": "string", "customer-segments": "stringslice",
	}
}

func TestFlashsalePlan_InvalidTypeErrors(t *testing.T) {
	in := newPlanInput(t, "flashsale", flashsaleFlags(), map[string]string{"type": "invalid", "value": "10"})
	_, err := flashsaleShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for invalid --type")
	}
}

func TestFlashsalePlan_PercentOutOfRangeErrors(t *testing.T) {
	in := newPlanInput(t, "flashsale", flashsaleFlags(), map[string]string{"type": "percent", "value": "0"})
	_, err := flashsaleShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when percent value out of range")
	}
}

func TestFlashsalePlan_PercentSuccess(t *testing.T) {
	in := newPlanInput(t, "flashsale", flashsaleFlags(), map[string]string{"type": "percent", "value": "20", "price-rule": "price"})
	p, err := flashsaleShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(p.DryRunHint, "every product in the store") || !strings.Contains(p.DryRunHint, "--variants") {
		t.Errorf("no scope: hint %q should say every product in the store and name --variants", p.DryRunHint)
	}
	if strings.Contains(p.DryRunHint, "--products") {
		t.Errorf("flashsale has no --products flag: hint %q", p.DryRunHint)
	}

	for _, scope := range []string{"variants", "collections"} {
		in = newPlanInput(t, "flashsale", flashsaleFlags(), map[string]string{"value": "20", scope: "x-1"})
		if p, _ = flashsaleShortcut.Plan(in); p.DryRunHint != "" {
			t.Errorf("--%s given: want no hint, got %q", scope, p.DryRunHint)
		}
	}
}
