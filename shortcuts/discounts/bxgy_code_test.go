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

func TestBxgyCodePlan_NoBuySideErrors(t *testing.T) {
	// products + get-products + get-quantity + get-free but no buy-quantity/buy-amount
	in := newPlanInput(t, "bxgy-code", bxgyCodeFlags(), map[string]string{
		"products": "p-1", "get-products": "p-2", "get-quantity": "1", "get-free": "true",
	})
	_, err := bxgyCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when neither --buy-quantity nor --buy-amount is set")
	}
}

func TestBxgyCodePlan_BothBuySideMutuallyExclusive(t *testing.T) {
	in := newPlanInput(t, "bxgy-code", bxgyCodeFlags(), map[string]string{
		"products": "p-1", "get-products": "p-2",
		"buy-quantity": "2", "buy-amount": "10",
		"get-quantity": "1", "get-free": "true",
	})
	_, err := bxgyCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when both --buy-quantity and --buy-amount are set")
	}
}

func TestBxgyCodePlan_NoGetDiscountErrors(t *testing.T) {
	// valid buy side, valid get side, but no get-percent/get-off/get-free
	in := newPlanInput(t, "bxgy-code", bxgyCodeFlags(), map[string]string{
		"products": "p-1", "get-products": "p-2",
		"buy-quantity": "2", "get-quantity": "1",
	})
	_, err := bxgyCodeShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when no get discount type is specified")
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
