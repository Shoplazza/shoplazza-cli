package orders

import (
	"testing"
)

var updateTrackingFlags = map[string]string{
	"order-id": "string", "fulfillment-id": "string",
	"tracking": "string", "company": "string",
	"tracking-url": "string", "notify": "bool",
}

func TestUpdateTrackingPlan_BasicSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "update-tracking", updateTrackingFlags, map[string]string{
		"order-id": "ord-1", "fulfillment-id": "ful-1", "tracking": "TRK123",
	})
	_, err := updateTrackingShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateTrackingPlan_WithNotifySuccess(t *testing.T) {
	in := newOrderPlanInput(t, "update-tracking", updateTrackingFlags, map[string]string{
		"order-id": "ord-1", "fulfillment-id": "ful-1", "tracking": "TRK123", "notify": "true",
	})
	_, err := updateTrackingShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
