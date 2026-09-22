package discounts

import (
	"testing"
)

func flashsaleFlags() map[string]string {
	return map[string]string{
		"value": "float", "type": "string", "variants": "string", "collections": "string",
		"price-rule": "string", "limit-user-variant": "int", "limit-user-product": "int",
		"limit-user-all": "int", "stock": "int", "combines": "stringslice",
		"name": "string", "start": "string", "end": "string", "customer-segments": "string",
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
	_, err := flashsaleShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
