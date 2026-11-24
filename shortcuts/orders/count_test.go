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
	in := shortcuttest.PlanInput(t, "count", orderCountFlags, map[string]string{"status": "placed", "since": "2026-11-01T00:00:00+08:00"})
	p, err := countShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Query["placed_at_min"] != "2026-10-31T16:00:00Z" {
		t.Errorf("placed_at_min = %v, want 2026-10-31T16:00:00Z", p.Query["placed_at_min"])
	}
}
