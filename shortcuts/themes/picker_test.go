package themes

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

func TestThemePickerExtract(t *testing.T) {
	resp := map[string]any{"themes": []any{
		map[string]any{"id": "t1", "name": "Dawn", "theme_type": "main"},
		map[string]any{"id": "t2", "name": "Dev"}, // no role → name only
		map[string]any{"name": "no-id"},           // no id → skipped
	}}
	opts := themePicker.Extract(resp)
	want := []interact.Option{
		{Label: "Dawn · main", Value: "t1"},
		{Label: "Dev", Value: "t2"},
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
