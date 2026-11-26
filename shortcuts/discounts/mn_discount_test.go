package discounts

import (
	"strings"
	"testing"
)

func mnDiscountFlags() map[string]string {
	return map[string]string{
		"tiers": "stringslice", "scope": "string",
		"products": "stringslice", "collections": "stringslice", "variants": "stringslice",
		"exclude": "bool", "price-sort": "string",
		"combines": "stringslice", "limit-max": "int", "limit-user": "int",
		"name": "string", "start": "string", "end": "string",
		"customer-segments": "stringslice",
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
	p, err := mnDiscountShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(p.DryRunHint, "every product in the store") || !strings.Contains(p.DryRunHint, "--products") {
		t.Errorf("no scope: hint %q should say every product in the store and name --products", p.DryRunHint)
	}

	in = newPlanInput(t, "mn-discount", mnDiscountFlags(), map[string]string{"tiers": "2:30", "products": "p-1", "exclude": "true"})
	if p, _ = mnDiscountShortcut.Plan(in); p.DryRunHint != "" {
		t.Errorf("--products --exclude given: want no hint, got %q", p.DryRunHint)
	}
}
