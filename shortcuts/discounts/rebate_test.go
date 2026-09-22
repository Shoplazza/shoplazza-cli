package discounts

import (
	"testing"
)

func rebateFlags() map[string]string {
	return map[string]string{
		"target": "string", "tiers": "string", "type": "string",
		"products": "string", "collections": "string", "variants": "string",
		"exclude": "bool", "limit-max": "int", "limit-user": "int",
		"limit-order-once": "bool", "combines": "stringslice",
		"name": "string", "start": "string", "end": "string", "customer-segments": "string",
	}
}

func TestRebatePlan_InvalidTargetErrors(t *testing.T) {
	in := newPlanInput(t, "rebate", rebateFlags(), map[string]string{"target": "invalid", "tiers": "100:10"})
	_, err := rebateShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for invalid --target")
	}
}

func TestRebatePlan_InvalidTiersErrors(t *testing.T) {
	in := newPlanInput(t, "rebate", rebateFlags(), map[string]string{"target": "order", "tiers": "bad"})
	_, err := rebateShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for invalid --tiers")
	}
}

func TestRebatePlan_OrderTargetSuccess(t *testing.T) {
	in := newPlanInput(t, "rebate", rebateFlags(), map[string]string{"target": "order", "tiers": "100:10,200:25"})
	_, err := rebateShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --target=order with a product scope is rejected by the API; catch it locally.
func TestRebatePlan_OrderTargetWithScopeErrors(t *testing.T) {
	for _, scope := range []string{"products", "collections", "variants"} {
		in := newPlanInput(t, "rebate", rebateFlags(), map[string]string{"target": "order", "tiers": "100:10", scope: "id1,id2"})
		if _, err := rebateShortcut.Plan(in); err == nil {
			t.Errorf("--target=order with --%s should error locally (avoid server 422 'selection is invalid')", scope)
		}
	}
}
