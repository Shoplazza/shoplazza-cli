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
	if !strings.Contains(out, "[1]") {
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
	if !strings.Contains(out, "[1]") {
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
	if err := listTable(&buf, items, nil, "", false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "string1") {
		t.Errorf("non-map items table: %s", out)
	}
}

func TestPretty_NestedObjectIndented(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"name": "x", "config": map[string]any{"width": float64(100), "height": float64(50)}}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// The nested object expands with indentation, not inline JSON.
	if !strings.Contains(out, "config:\n") {
		t.Errorf("nested key should head its own line: %s", out)
	}
	if !strings.Contains(out, "\n  width:") || !strings.Contains(out, "100") {
		t.Errorf("nested field should be indented under config: %s", out)
	}
	if strings.Contains(out, `{"width"`) {
		t.Errorf("nested object must not render as inline JSON: %s", out)
	}
}

func TestPretty_ScalarArrayJoined(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"name": "x", "tags": []any{"a", "b", "c"}}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "tags: a, b, c") {
		t.Errorf("scalar array should join inline: %s", out)
	}
}

func TestPretty_FieldPriorityIdFirst(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"zebra": "z", "id": "1", "alpha": "a"}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// id (prioritized) precedes the alphabetical rest.
	if idx, aIdx := strings.Index(out, "id:"), strings.Index(out, "alpha:"); idx < 0 || idx > aIdx {
		t.Errorf("id should lead alphabetical fields: %s", out)
	}
}

func TestTable_FlattensNestedToDotColumns(t *testing.T) {
	var buf bytes.Buffer
	items := []any{
		map[string]any{"summary": "Create", "http": map[string]any{"method": "POST", "path": "/x"}},
	}
	if err := PrintFormatted(&buf, items, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Nested object becomes dot-notation columns (upper-cased), not "{…}".
	if !strings.Contains(out, "HTTP.METHOD") || !strings.Contains(out, "HTTP.PATH") {
		t.Errorf("nested object should flatten to dot columns: %s", out)
	}
	if strings.Contains(out, "{…}") {
		t.Errorf("table must not collapse nested object to placeholder: %s", out)
	}
}

func TestColorEnabled_BufferIsPlain(t *testing.T) {
	var buf bytes.Buffer
	if colorEnabled(&buf) {
		t.Error("a non-terminal writer must not enable color")
	}
	// And the rendered output carries no ANSI escapes.
	_ = PrintFormatted(&buf, map[string]any{"id": "1"}, FormatPretty)
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("plain writer output must not contain ANSI escapes: %q", buf.String())
	}
}

func TestPretty_ListEnvelopeLabelsKey(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"profiles": []any{map[string]any{"name": "x"}}, "logged_in": true}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "profiles:") {
		t.Errorf("list envelope should lead with its key label, not a bare [1]: %s", out)
	}
	if !strings.Contains(out, "\n  [1]") {
		t.Errorf("items should be indented under the key: %s", out)
	}
}

func TestTable_ListEnvelopeCaption(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{"profiles": []any{map[string]any{"name": "x"}}, "logged_in": true}
	if err := PrintFormatted(&buf, m, FormatTable); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "profiles:") {
		t.Errorf("table should caption the list with its key: %s", buf.String())
	}
}

func TestDominantObjectList(t *testing.T) {
	// A clean envelope: one object-list + scalar meta.
	if _, key, ok := dominantObjectList(map[string]any{
		"orders": []any{map[string]any{"id": "1"}}, "total": float64(1),
	}); !ok || key != "orders" {
		t.Errorf("clean envelope should be dominant (key=%q ok=%v)", key, ok)
	}
	// A rich record: object-list PLUS a nested-object sibling → not an envelope.
	if _, _, ok := dominantObjectList(map[string]any{
		"variants": []any{map[string]any{"id": "v"}}, "image": map[string]any{"src": "x"},
	}); ok {
		t.Error("object with a nested-object sibling must not be treated as an envelope")
	}
	// Two object-lists → ambiguous, not a single envelope.
	if _, _, ok := dominantObjectList(map[string]any{
		"variants": []any{map[string]any{"id": "v"}}, "options": []any{map[string]any{"name": "Size"}},
	}); ok {
		t.Error("two object-lists must not resolve to a single dominant list")
	}
}

func TestPretty_RichObjectNotHijackedByList(t *testing.T) {
	var buf bytes.Buffer
	m := map[string]any{
		"id": "1", "title": "Tee",
		"image":    map[string]any{"src": "x"},
		"variants": []any{map[string]any{"id": "v1"}},
	}
	if err := PrintFormatted(&buf, m, FormatPretty); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "title:") || !strings.Contains(out, "variants:") {
		t.Errorf("rich object should render as an object with nested variants: %s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "[1]") {
		t.Errorf("rich object must not be hijacked into a bare variants list: %s", out)
	}
}

func TestListColumnHeaders_NonMap(t *testing.T) {
	items := []any{"not a map"}
	if got := listColumnHeaders(items); got != nil {
		t.Errorf("expected nil headers for non-map items, got %v", got)
	}
}
