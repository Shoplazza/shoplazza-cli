package discounts

import (
	"testing"
)

func freeShippingFlags() map[string]string {
	return map[string]string{
		"off": "float", "min-amount": "float", "min-quantity": "int",
		"countries": "string", "code": "string", "name": "string",
		"start": "string", "end": "string", "combines": "stringslice",
		"limit-max": "int", "limit-user": "int", "customer-segments": "string",
	}
}

func TestFreeShippingCodePlan_DefaultsSuccess(t *testing.T) {
	in := newPlanInput(t, "free-shipping-code", freeShippingFlags(), nil)
	_, err := freeShippingCodeShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFreeShippingCodePlan_BothMinAmountAndQtyErrors(t *testing.T) {
	in := newPlanInput(t, "free-shipping-code", freeShippingFlags(),
		map[string]string{"min-amount": "10", "min-quantity": "2"})
	_, err := freeShippingCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when both --min-amount and --min-quantity set")
	}
}
