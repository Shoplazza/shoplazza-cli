package products

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

func TestProductPickerExtract(t *testing.T) {
	resp := map[string]any{"products": []any{
		map[string]any{"id": "p1", "title": "T-shirt", "status": "active"},
		map[string]any{"id": "p2", "title": "Hat"}, // no status → title only
		map[string]any{"title": "no-id"},           // no id → skipped
	}}
	opts := productPicker.Extract(resp)
	want := []interact.Option{
		{Label: "T-shirt · active", Value: "p1"},
		{Label: "Hat", Value: "p2"},
	}
	if len(opts) != len(want) {
		t.Fatalf("got %d options, want %d: %+v", len(opts), len(want), opts)
	}
	for i, w := range want {
		if opts[i] != w {
			t.Errorf("option %d = %+v, want %+v", i, opts[i], w)
		}
	}
}
