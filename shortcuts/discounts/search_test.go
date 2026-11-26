package discounts

import (
	"testing"
)

func searchFlags() map[string]string {
	return map[string]string{
		"query": "string", "discount-code": "string",
		"progress": "stringslice", "discount-type": "stringslice",
		"discount-target": "stringslice", "discount-method": "stringslice",
		"page-limit": "int", "cursor": "string",
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
	in := newPlanInput(t, "search", searchFlags(), map[string]string{"page-limit": "5", "cursor": "c-2"})
	p, err := searchShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Query["cursor"] != "c-2" || p.Query["page_size"] != 5 {
		t.Errorf("--cursor/--page-limit must reach cursor/page_size, got %v / %v", p.Query["cursor"], p.Query["page_size"])
	}
}
