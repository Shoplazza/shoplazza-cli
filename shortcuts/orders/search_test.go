package orders

import (
	"testing"
)

var orderSearchFlags = map[string]string{
	"keyword": "string", "status": "string",
	"financial-status": "string", "fulfillment-status": "string",
	"customer-id": "string", "since": "string", "until": "string",
	"page-limit": "int",
}

func TestOrderSearchPlan_DefaultsSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "search", orderSearchFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrderSearchPlan_WithFiltersSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "search", orderSearchFlags, map[string]string{
		"status": "placed", "page-limit": "5",
	})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
