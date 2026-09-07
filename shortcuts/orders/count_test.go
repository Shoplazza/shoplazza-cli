package orders

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

var orderCountFlags = map[string]string{
	"status": "string", "financial-status": "string",
	"fulfillment-status": "string", "since": "string", "until": "string",
}

func TestOrderCountPlan_DefaultsSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "count", orderCountFlags, nil)
	_, err := countShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrderCountPlan_WithStatusSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "count", orderCountFlags, map[string]string{"status": "placed"})
	_, err := countShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
