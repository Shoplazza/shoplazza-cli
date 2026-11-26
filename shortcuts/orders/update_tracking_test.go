package orders

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

var updateTrackingFlags = declaredFlags(updateTrackingShortcut)

func TestUpdateTrackingPlan_BasicSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "update-tracking", updateTrackingFlags, map[string]string{
		"order-id": "ord-1", "fulfillment-id": "ful-1", "tracking": "TRK123",
		"company-code": "sf", "tracking-url": "https://t.example/TRK123",
	})
	plan, err := updateTrackingShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := plan.Body.(map[string]any)["fulfillment"].(map[string]any)
	if body["tracking_company_code"] != "sf" || body["tracking_url"] != "https://t.example/TRK123" {
		t.Errorf("--company-code / --tracking-url must reach the body: %v", body)
	}
}

func TestUpdateTrackingPlan_WithNotifySuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "update-tracking", updateTrackingFlags, map[string]string{
		"order-id": "ord-1", "fulfillment-id": "ful-1", "tracking": "TRK123", "notify": "true",
	})
	_, err := updateTrackingShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
