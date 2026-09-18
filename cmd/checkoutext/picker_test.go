package checkoutext

import (
	"encoding/json"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

func TestExtractExtensionOptions(t *testing.T) {
	// Mirrors the real list response: {code,data:{extensions:[...]}}, ids and
	// statuses as the fields the endpoint actually returns.
	body := map[string]any{
		"code": "Success",
		"data": map[string]any{"extensions": []any{
			map[string]any{"name": "Alpha", "extension_id": "E1", "publish_status": "published"},
			map[string]any{"name": "Beta", "extension_id": "E2"},                  // no status → label is just the name
			map[string]any{"extension_id": "E3"},                                  // no name → falls back to id
			map[string]any{"name": "Ghost"},                                       // no id → skipped
			"not-a-row",                                                           // wrong type → skipped
		}},
	}
	opts := extractExtensionOptions(body)
	want := []interact.Option{
		{Label: "Alpha · published", Value: "E1"},
		{Label: "Beta", Value: "E2"},
		{Label: "E3", Value: "E3"},
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

func TestExtractVersionOptions(t *testing.T) {
	// Version array is nested under "extensions"; numbers arrive as json.Number
	// (the client decodes with UseNumber).
	body := map[string]any{
		"data": map[string]any{"extensions": []any{
			map[string]any{"version": "1.0", "id": "v1"},
			map[string]any{"version": json.Number("2"), "id": "v2"},
			map[string]any{"id": "v3"}, // no version → skipped
		}},
	}
	opts := extractVersionOptions(body)
	want := []interact.Option{
		{Label: "1.0", Value: "1.0"},
		{Label: "2", Value: "2"},
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

func TestExtractOptions_EmptyOrMalformed(t *testing.T) {
	for _, body := range []any{
		nil,
		map[string]any{"data": map[string]any{}},                 // no extensions key
		map[string]any{"data": map[string]any{"extensions": []any{}}}, // empty
	} {
		if opts := extractExtensionOptions(body); len(opts) != 0 {
			t.Errorf("extension options from %v = %+v, want empty", body, opts)
		}
		if opts := extractVersionOptions(body); len(opts) != 0 {
			t.Errorf("version options from %v = %+v, want empty", body, opts)
		}
	}
}
