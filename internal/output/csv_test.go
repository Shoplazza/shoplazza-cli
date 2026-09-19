package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestCSV_List_CuratedColumns(t *testing.T) {
	d := map[string]any{"orders": []any{
		map[string]any{"id": "o1", "number": "#1001", "total_price": "299.00", "note": "x"},
		map[string]any{"id": "o2", "number": "#1002", "total_price": "89.00"},
	}}
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, d, FormatCSV); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if lines[0] != "id,number,total_price" {
		t.Errorf("header = %q, want id,number,total_price", lines[0])
	}
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("want 3 lines, got %d: %q", len(lines), buf.String())
	}
	if strings.Contains(buf.String(), "note") || strings.Contains(buf.String(), ",x") {
		t.Errorf("non-allow-listed field leaked: %q", buf.String())
	}
}

// A value with a comma/quote must be RFC-4180 quoted by encoding/csv.
func TestCSV_Escaping(t *testing.T) {
	var buf bytes.Buffer
	items := []any{map[string]any{"id": "1", "title": `a, "b"`}}
	if err := PrintFormatted(&buf, items, FormatCSV); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"a, ""b"""`) {
		t.Errorf("expected quoted/escaped cell, got %q", buf.String())
	}
}

// Nested object/array cells are kept as compact JSON (no data loss).
func TestCSV_NestedCellIsJSON(t *testing.T) {
	if got := csvCell(map[string]any{"email": "a@b.com"}); got != `{"email":"a@b.com"}` {
		t.Errorf("nested object cell = %q", got)
	}
	if got := csvCell([]any{1.0, 2.0}); got != `[1,2]` {
		t.Errorf("nested array cell = %q", got)
	}
}

// A single object (e.g. `order get`) → header + one data row.
func TestCSV_SingleObject(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, map[string]any{"id": "1", "status": "open"}, FormatCSV); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "id,status" {
		t.Errorf("single-object csv = %q", buf.String())
	}
}

// PrintAPISuccess must drop the {ok,data} envelope for CSV.
func TestCSV_PrintAPISuccess_SkipsEnvelope(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintAPISuccess(&buf, map[string]any{"products": []any{map[string]any{"id": "p1"}}}, FormatCSV, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "ok") || strings.Contains(buf.String(), "data") {
		t.Errorf("csv must not wrap in {ok,data}: %q", buf.String())
	}
}

// --jq with --format csv is rejected (jq requires json).
func TestCSV_JQRejected(t *testing.T) {
	var buf bytes.Buffer
	err := PrintAPISuccess(&buf, map[string]any{"id": "1"}, FormatCSV, ".id")
	if err == nil || !strings.Contains(err.Error(), "--jq requires --format json") {
		t.Errorf("expected jq-requires-json error, got %v", err)
	}
}
