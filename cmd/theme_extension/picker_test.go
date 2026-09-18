package theme_extension

import (
	"encoding/json"
	"testing"
)

// TestThemePickerOptions pins the field mapping across the id/title forms a JSON
// decode yields, and the skip/fallback rules.
func TestThemePickerOptions(t *testing.T) {
	themes := []map[string]any{
		{"id": "111", "title": "Main", "role": "main"}, // string id + role suffix
		{"theme_id": "222", "name": "Draft"},           // alt keys, no role
		{"id": json.Number("333")},                     // numeric id, no title → id label
		{"id": float64(444), "title": ""},              // float id, empty title → id label
		{"title": "no id, dropped"},                    // no id → skipped
	}

	opts := themePickerOptions(themes)

	if len(opts) != 4 {
		t.Fatalf("got %d options, want 4 (the id-less row is skipped): %+v", len(opts), opts)
	}
	want := []struct{ label, value string }{
		{"Main · main", "111"},
		{"Draft", "222"},
		{"333", "333"},
		{"444", "444"},
	}
	for i, w := range want {
		if opts[i].Label != w.label || opts[i].Value != w.value {
			t.Errorf("opt[%d] = {%q,%q}, want {%q,%q}", i, opts[i].Label, opts[i].Value, w.label, w.value)
		}
	}
}
