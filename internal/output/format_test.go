package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatScalar(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{nil, ""},
		{map[string]any{"a": 1}, `{"a":1}`},
	}
	for _, c := range cases {
		got := formatScalar(c.v, 0)
		if got != c.want {
			t.Errorf("formatScalar(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestFormatScalar_Truncate(t *testing.T) {
	v := map[string]any{"key": "very long value that exceeds the limit"}
	got := formatScalar(v, 10)
	if len(got) != 10 || !strings.HasSuffix(got, "...") {
		t.Errorf("truncated: got %q (len %d)", got, len(got))
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]any{"z": 1, "a": 2, "m": 3}
	got := sortedKeys(m)
	want := []string{"a", "m", "z"}
	for i, k := range want {
		if got[i] != k {
			t.Errorf("sortedKeys[%d] = %q, want %q", i, got[i], k)
		}
	}
}

func TestExtractListKey_Found(t *testing.T) {
	m := map[string]any{"orders": []any{"a", "b"}, "total": 2}
	list, key := extractListKey(m)
	if key != "orders" || len(list) != 2 {
		t.Errorf("extractListKey: list=%v key=%q", list, key)
	}
}

func TestExtractListKey_TypedSlice(t *testing.T) {
	m := map[string]any{"items": []map[string]any{{"id": "1"}, {"id": "2"}}}
	list, key := extractListKey(m)
	if key != "items" || len(list) != 2 {
		t.Errorf("extractListKey typed: list=%v key=%q", list, key)
	}
}

func TestExtractListKey_NotFound(t *testing.T) {
	m := map[string]any{"count": 5, "name": "test"}
	list, key := extractListKey(m)
	if list != nil || key != "" {
		t.Errorf("expected nil,\"\"; got %v,%q", list, key)
	}
}

// ── PrintFormatted ────────────────────────────────────────────────────────────

func TestPrintFormatted_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, map[string]any{"k": "v"}, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"k"`) {
		t.Errorf("JSON output missing key: %s", buf.String())
	}
}

func TestPrintFormatted_Pretty_Map(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, map[string]any{"name": "Alice", "age": float64(30)}, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") || !strings.Contains(out, "Alice") {
		t.Errorf("pretty output missing fields: %s", out)
	}
}

func TestPrintFormatted_Pretty_Slice(t *testing.T) {
	var buf bytes.Buffer
	items := []any{
		map[string]any{"id": "1", "title": "A"},
		map[string]any{"id": "2", "title": "B"},
	}
	if err := PrintFormatted(&buf, items, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "--- [1] ---") {
		t.Errorf("pretty slice missing index header: %s", out)
	}
}

func TestPrintFormatted_Pretty_Scalar(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, "hello", FormatPretty); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("scalar pretty: %s", buf.String())
	}
}

func TestPrintFormatted_Pretty_SingleKeyNested(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"order": map[string]any{"id": "123", "status": "open"}}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "id") || !strings.Contains(out, "123") {
		t.Errorf("single-key nested pretty: %s", out)
	}
}

func TestPrintFormatted_Pretty_SingleKeyList(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"orders": []any{map[string]any{"id": "1"}}}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "--- [1] ---") {
		t.Errorf("single-key list pretty: %s", out)
	}
}

func TestPrintFormatted_Pretty_ListWithMeta(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{
		"orders": []any{map[string]any{"id": "1"}},
		"total":  float64(1),
	}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "total") {
		t.Errorf("meta field missing: %s", out)
	}
}

func TestPrintFormatted_Table_List(t *testing.T) {
	var buf bytes.Buffer
	items := []any{
		map[string]any{"id": "1", "name": "A"},
		map[string]any{"id": "2", "name": "B"},
	}
	if err := PrintFormatted(&buf, items, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("table headers missing: %s", out)
	}
}

func TestPrintFormatted_Table_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, []any{}, FormatTable); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(no items)") {
		t.Errorf("empty table: %s", buf.String())
	}
}

func TestPrintFormatted_Table_SingleObject(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"id": "1", "status": "open"}
	if err := PrintFormatted(&buf, m, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "KEY") || !strings.Contains(out, "VALUE") {
		t.Errorf("object table header missing: %s", out)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "status") {
		t.Errorf("object table keys missing: %s", out)
	}
}

func TestPrintFormatted_Table_MapWithList(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{
		"orders": []any{map[string]any{"id": "1"}},
		"total":  float64(1),
	}
	if err := PrintFormatted(&buf, m, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") {
		t.Errorf("list-in-map table: %s", out)
	}
}

func TestPrintFormatted_Table_MapSingleKeyObject(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"order": map[string]any{"id": "1", "status": "open"}}
	if err := PrintFormatted(&buf, m, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "KEY") {
		t.Errorf("single-key object table: %s", out)
	}
}

func TestPrintFormatted_Table_Scalar(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, "hello", FormatTable); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("scalar table: %s", buf.String())
	}
}

func TestPrintListTable_NonMapItems(t *testing.T) {
	var buf bytes.Buffer
	items := []any{"string1", "string2"}
	if err := printListTable(&buf, items, nil, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "string1") {
		t.Errorf("non-map items table: %s", out)
	}
}

func TestListColumnHeaders_NonMap(t *testing.T) {
	items := []any{"not a map"}
	if got := listColumnHeaders(items); got != nil {
		t.Errorf("expected nil headers for non-map items, got %v", got)
	}
}
