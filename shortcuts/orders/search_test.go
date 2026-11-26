package orders

import (
	"reflect"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

var orderSearchFlags = map[string]string{
	"keyword": "string", "status": "stringslice",
	"financial-status": "stringslice", "fulfillment-status": "stringslice",
	"customer-id": "string", "since": "string", "until": "string",
	"page-limit": "int", "cursor": "string",
}

func TestOrderSearchPlan_DefaultsSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", orderSearchFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrderSearchPlan_WithFiltersSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", orderSearchFlags, map[string]string{
		"status": "placed", "financial-status": "paid,refunded", "page-limit": "5",
		"since": "2026-11-01 08:00", "until": "1800000000", "cursor": "c-2",
	})
	p, err := searchShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Query["cursor"] != "c-2" || p.Query["page_size"] != 5 {
		t.Errorf("--cursor/--page-limit must reach cursor/page_size, got %v / %v", p.Query["cursor"], p.Query["page_size"])
	}
	if !reflect.DeepEqual(p.Query["status"], []string{"placed"}) || !reflect.DeepEqual(p.Query["financial_status"], []string{"paid", "refunded"}) {
		t.Errorf("status filters must be sent as arrays, got status=%#v financial_status=%#v", p.Query["status"], p.Query["financial_status"])
	}
	if p.Query["placed_at_min"] != "2026-11-01T08:00:00Z" || p.Query["placed_at_max"] != "2027-01-15T08:00:00Z" {
		t.Errorf("--since/--until must reach placed_at_* as RFC3339 UTC, got %v / %v", p.Query["placed_at_min"], p.Query["placed_at_max"])
	}
	in = shortcuttest.PlanInput(t, "search", orderSearchFlags, map[string]string{"since": "yesterday-ish"})
	if _, err := searchShortcut.Plan(in); err == nil {
		t.Error("an unparseable --since must be rejected, not sent as-is")
	}
}
