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

// Every enum-ish flag is validated before the request is built.
func TestMNDiscountPlan_Refusals(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
	}{
		{"invalid --tiers", map[string]string{"tiers": "bad"}},
		{"invalid --scope", map[string]string{"tiers": "2:30", "scope": "invalid-scope"}},
		{"invalid --price-sort", map[string]string{"tiers": "2:30", "price-sort": "sideways"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newPlanInput(t, "mn-discount", mnDiscountFlags(), c.flags)
			if _, err := mnDiscountShortcut.Plan(in); err == nil {
				t.Error("expected a refusal")
			}
		})
	}
}

func TestMNDiscountPlan_HighestScopeSuccess(t *testing.T) {
	in := newPlanInput(t, "mn-discount", mnDiscountFlags(), map[string]string{"tiers": "2:30,3:50", "price-sort": "desc"})
	_, err := mnDiscountShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
