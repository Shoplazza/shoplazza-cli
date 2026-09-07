package discounts

import (
	"testing"
)

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
