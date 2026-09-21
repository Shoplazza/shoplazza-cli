package orders

import (
	"encoding/json"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

func TestOrderPickerExtract(t *testing.T) {
	// id arrives as json.Number (the client decodes with UseNumber); label leads
	// with the order number and appends status/total when present.
	resp := map[string]any{"orders": []any{
		map[string]any{"id": json.Number("1001"), "number": "1001", "financial_status": "paid", "total_price": "29.99"},
		map[string]any{"id": "o2", "number": "1002"}, // no status/total → number only
		map[string]any{"number": "no-id"},            // no id → skipped
	}}
	opts := orderPicker.Extract(resp)
	want := []interact.Option{
		{Label: "#1001 · paid · 29.99", Value: "1001"},
		{Label: "#1002", Value: "o2"},
	}
	assertOptions(t, opts, want)
}

func assertOptions(t *testing.T, got, want []interact.Option) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("option %d = %+v, want %+v", i, got[i], w)
		}
	}
}
