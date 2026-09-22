package discounts

import "testing"

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

// ── percentCodeShortcut ───────────────────────────────────────────────────────

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

// ── freeShippingCodeShortcut ──────────────────────────────────────────────────

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

// ── flashsaleShortcut ─────────────────────────────────────────────────────────

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

// ── rebateShortcut ────────────────────────────────────────────────────────────

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

// ── searchShortcut ────────────────────────────────────────────────────────────

func searchFlags() map[string]string {
	return map[string]string{
		"query": "string", "discount-code": "string",
		"progress": "stringslice", "discount-type": "stringslice",
		"discount-target": "stringslice", "discount-method": "stringslice",
		"page-limit": "int",
	}
}

func TestSearchPlan_DefaultsSuccess(t *testing.T) {
	in := newPlanInput(t, "search", searchFlags(), nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchPlan_WithPageLimitSuccess(t *testing.T) {
	in := newPlanInput(t, "search", searchFlags(), map[string]string{"page-limit": "5"})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── mnDiscountShortcut ────────────────────────────────────────────────────────

func mnDiscountFlags() map[string]string {
	return map[string]string{
		"tiers": "string", "scope": "string",
		"products": "string", "collections": "string", "variants": "string",
		"exclude": "bool", "price-sort": "string",
		"combines": "stringslice", "limit-max": "int", "limit-user": "int",
		"name": "string", "start": "string", "end": "string",
		"customer-segments": "string",
	}
}

func TestMNDiscountPlan_InvalidTiersErrors(t *testing.T) {
	in := newPlanInput(t, "mn-discount", mnDiscountFlags(), map[string]string{"tiers": "bad"})
	_, err := mnDiscountShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for invalid --tiers")
	}
}

func TestMNDiscountPlan_InvalidScopeErrors(t *testing.T) {
	in := newPlanInput(t, "mn-discount", mnDiscountFlags(), map[string]string{"tiers": "2:30", "scope": "invalid-scope"})
	_, err := mnDiscountShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for invalid --scope")
	}
}

func TestMNDiscountPlan_InvalidPriceSortErrors(t *testing.T) {
	in := newPlanInput(t, "mn-discount", mnDiscountFlags(), map[string]string{"tiers": "2:30", "price-sort": "sideways"})
	_, err := mnDiscountShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for invalid --price-sort")
	}
}

func TestMNDiscountPlan_HighestScopeSuccess(t *testing.T) {
	in := newPlanInput(t, "mn-discount", mnDiscountFlags(), map[string]string{"tiers": "2:30,3:50", "price-sort": "desc"})
	_, err := mnDiscountShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── bxgyCodeShortcut ──────────────────────────────────────────────────────────

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
