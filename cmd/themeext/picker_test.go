package themeext

import (
	"encoding/json"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
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

// TestAppPickerOptions pins value = client_id, label = name + client_id, and the
// skip rule for apps with no client_id.
func TestAppPickerOptions(t *testing.T) {
	apps := []app.App{
		{ClientID: "cid1", Name: "Checkout Helper"}, // name + client_id
		{ClientID: "cid2", Name: ""},                // no name → client_id label
		{ClientID: "", Name: "no client id"},        // no client_id → skipped
	}

	opts := appPickerOptions(apps)

	if len(opts) != 2 {
		t.Fatalf("got %d options, want 2 (the client-id-less app is skipped): %+v", len(opts), opts)
	}
	want := []struct{ label, value string }{
		{"Checkout Helper · cid1", "cid1"},
		{"cid2", "cid2"},
	}
	for i, w := range want {
		if opts[i].Label != w.label || opts[i].Value != w.value {
			t.Errorf("opt[%d] = {%q,%q}, want {%q,%q}", i, opts[i].Label, opts[i].Value, w.label, w.value)
		}
	}
}
