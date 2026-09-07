package discounts

import (
	"testing"
)

func bxgyCodeFlags() map[string]string {
	return map[string]string{
		"products": "string", "variants": "string", "collections": "string",
		"exclude": "bool", "buy-quantity": "int", "buy-amount": "float",
		"get-products": "string", "get-variants": "string", "get-collections": "string",
		"get-quantity": "int", "get-percent": "int", "get-off": "float", "get-free": "bool",
		"code": "string", "name": "string", "start": "string", "end": "string",
		"combines": "stringslice", "limit-max": "int", "limit-user": "int",
		"limit-order": "int", "customer-segments": "string",
	}
}

// The buy side needs exactly one of --buy-quantity / --buy-amount, and the
// get side needs a discount type.
func TestBxgyCodePlan_Refusals(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
	}{
		{"no buy side", map[string]string{
			"products": "p-1", "get-products": "p-2", "get-quantity": "1", "get-free": "true",
		}},
		{"both buy-quantity and buy-amount", map[string]string{
			"products": "p-1", "get-products": "p-2",
			"buy-quantity": "2", "buy-amount": "10",
			"get-quantity": "1", "get-free": "true",
		}},
		{"no get discount type", map[string]string{
			"products": "p-1", "get-products": "p-2",
			"buy-quantity": "2", "get-quantity": "1",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newPlanInput(t, "bxgy-code", bxgyCodeFlags(), c.flags)
			if _, err := bxgyCodeShortcut.Plan(in); err == nil {
				t.Error("expected a refusal")
			}
		})
	}
}

func TestBxgyCodePlan_BuyQuantityGetFreeSuccess(t *testing.T) {
	in := newPlanInput(t, "bxgy-code", bxgyCodeFlags(), map[string]string{
		"products": "p-1", "get-products": "p-2",
		"buy-quantity": "2", "get-quantity": "1", "get-free": "true",
	})
	_, err := bxgyCodeShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
