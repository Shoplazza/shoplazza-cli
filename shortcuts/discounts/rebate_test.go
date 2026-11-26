package discounts

import (
	"errors"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func rebateFlags() map[string]string {
	return map[string]string{
		"target": "string", "tiers": "stringslice", "type": "string",
		"products": "stringslice", "collections": "stringslice", "variants": "stringslice",
		"exclude": "bool", "limit-max": "int", "limit-user": "int",
		"limit-order-once": "bool", "combines": "stringslice",
		"name": "string", "start": "string", "end": "string", "customer-segments": "stringslice",
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

// Quantity-threshold rebates accept a 0-discount tier; amount-threshold ones don't.
func TestRebatePlan_ZeroDiscountTier(t *testing.T) {
	cases := []struct {
		typ     string
		wantErr bool
	}{
		{"qty-off", false},
		{"qty-percent", false},
		{"amount-off", true},
		{"amount-percent", true},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			in := newPlanInput(t, "rebate", rebateFlags(), map[string]string{"target": "order", "type": tc.typ, "tiers": "1:0,3:10"})
			_, err := rebateShortcut.Plan(in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("--type=%s --tiers 1:0,3:10: err=%v wantErr=%v", tc.typ, err, tc.wantErr)
			}
		})
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

func TestRebatePlan_TierRefusals(t *testing.T) {
	const belowThreshold = "--tiers: each discount must be less than its threshold"
	const uncappedOneTier = "--limit-order-once=false repeats a single tier for every multiple of its threshold; --tiers has 2 tiers"
	cases := []struct {
		name      string
		values    map[string]string
		wantParam string
		wantMsg   string
	}{
		{"amount-off discount above threshold", map[string]string{"target": "order", "tiers": "5:10"}, "--tiers", belowThreshold},
		{"amount-off discount equals threshold", map[string]string{"target": "order", "tiers": "10:10"}, "--tiers", belowThreshold},
		{"amount-off second tier too large", map[string]string{"target": "order", "tiers": "100:10,200:250"}, "--tiers", belowThreshold},
		{"amount-off product target", map[string]string{"target": "product", "tiers": "5:10", "products": "p-1"}, "--tiers", belowThreshold},
		{"amount-off below threshold", map[string]string{"target": "order", "tiers": "100:10,200:25"}, "", ""},
		{"qty-off compares item count", map[string]string{"target": "order", "type": "qty-off", "tiers": "2:10"}, "", ""},
		{"amount-percent compares a percent", map[string]string{"target": "order", "type": "amount-percent", "tiers": "5:10"}, "", ""},
		{"uncapped amount-off with two tiers", map[string]string{"target": "order", "tiers": "100:10,200:25", "limit-order-once": "false"}, "--limit-order-once", uncappedOneTier},
		{"uncapped qty-off product with two tiers", map[string]string{"target": "product", "type": "qty-off", "tiers": "2:3,4:8", "products": "p-1", "limit-order-once": "false"}, "--limit-order-once", uncappedOneTier},
		{"uncapped with one tier", map[string]string{"target": "order", "tiers": "100:10", "limit-order-once": "false"}, "", ""},
		{"two tiers once per order", map[string]string{"target": "order", "tiers": "100:10,200:25", "limit-order-once": "true"}, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := rebateShortcut.Plan(newPlanInput(t, "rebate", rebateFlags(), c.values))
			if c.wantParam == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Param != c.wantParam {
				t.Fatalf("want a validation error on %s, got %#v", c.wantParam, err)
			}
			if ee.Detail.Message != c.wantMsg {
				t.Fatalf("message = %q, want %q", ee.Detail.Message, c.wantMsg)
			}
		})
	}
}
