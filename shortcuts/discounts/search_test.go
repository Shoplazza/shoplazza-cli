package discounts

import (
	"testing"
)

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
