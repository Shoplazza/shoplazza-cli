package output

import (
	"bytes"
	"strings"
	"testing"
)

// A rich orders record must render only the curated columns (in order), not its
// 40 raw fields, and nested objects/arrays must not become wide JSON columns.
func TestTable_Orders_CuratedColumns(t *testing.T) {
	item := map[string]any{
		"id":               "o1",
		"number":           "#1001",
		"financial_status": "paid",
		"total_price":      "299.00",
		"currency":         "USD",
		"placed_at":        "2026-09-16",
		// noise fields that must NOT appear as columns:
		"note":       "some internal note",
		"sub_total":  "299.00",
		"customer":   map[string]any{"email": "a@b.com"}, // nested
		"line_items": []any{map[string]any{"x": 1}},      // nested
	}
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, map[string]any{"orders": []any{item}}, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{"NUMBER", "FINANCIAL_STATUS", "TOTAL_PRICE", "PLACED_AT"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected curated column %q, got:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"NOTE", "SUB_TOTAL", "CUSTOMER", "LINE_ITEMS"} {
		if strings.Contains(out, notWant) {
			t.Errorf("column %q should be excluded, got:\n%s", notWant, out)
		}
	}
}

func TestTableCell_NestedSummary(t *testing.T) {
	if got := tableCell(map[string]any{"a": 1, "b": 2}); got != "{…}" {
		t.Errorf("object cell = %q, want {…}", got)
	}
	if got := tableCell([]any{1, 2, 3}); got != "[3]" {
		t.Errorf("array cell = %q, want [3]", got)
	}
	if got := tableCell("plain"); got != "plain" {
		t.Errorf("scalar cell = %q, want plain", got)
	}
}

func TestSelectColumns_FallbackWhenUnknownKey(t *testing.T) {
	items := []any{map[string]any{"id": "1", "weird": "x"}}
	if cols := selectColumns("not_a_domain", items); cols != nil {
		t.Errorf("unknown key should return nil (fall back), got %v", cols)
	}
	// A known key but the record has none of its allow-listed columns → nil.
	if cols := selectColumns("orders", []any{map[string]any{"weird": "x"}}); cols != nil {
		t.Errorf("no present columns should return nil, got %v", cols)
	}
}

func TestSelectColumns_PreservesAllowlistOrderAndPresence(t *testing.T) {
	items := []any{map[string]any{"total_price": "9", "id": "1", "unknown": "z"}}
	cols := selectColumns("orders", items)
	// order follows the allow-list (id before total_price), unknown dropped.
	want := []string{"id", "total_price"}
	if len(cols) != len(want) {
		t.Fatalf("cols = %v, want %v", cols, want)
	}
	for i := range want {
		if cols[i] != want[i] {
			t.Errorf("cols[%d] = %q, want %q", i, cols[i], want[i])
		}
	}
}

// Fallback path (bare list, no envelope key) keeps showing all keys.
func TestTable_BareList_FallsBackToAllKeys(t *testing.T) {
	var buf bytes.Buffer
	items := []any{map[string]any{"id": "1", "custom": "keep"}}
	if err := PrintFormatted(&buf, items, FormatTable); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CUSTOM") {
		t.Errorf("bare list should keep all columns, got:\n%s", buf.String())
	}
}

// Pretty list under a known key is curated per item.
func TestPretty_Products_Curated(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"products": []any{map[string]any{
		"id": "p1", "title": "Tee", "note": "hidden", "variants": []any{1, 2},
	}}}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "title") || !strings.Contains(out, "Tee") {
		t.Errorf("curated field missing: %s", out)
	}
	if strings.Contains(out, "hidden") || strings.Contains(out, "variants") {
		t.Errorf("non-curated field leaked into pretty: %s", out)
	}
}
