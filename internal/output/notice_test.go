package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintAPISuccess_NoticeInJSONEnvelope(t *testing.T) {
	// Arrange
	SetNotice(map[string]any{"update": map[string]any{"latest": "2.0.0"}})
	t.Cleanup(func() { SetNotice(nil) })
	var buf bytes.Buffer

	// Act
	if err := PrintAPISuccess(&buf, map[string]any{"id": "1"}, FormatJSON, ""); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}

	// Assert: _notice rides alongside data, ok stays true.
	out := buf.String()
	if !strings.Contains(out, `"_notice"`) || !strings.Contains(out, `"latest": "2.0.0"`) {
		t.Errorf("expected _notice block, got %q", out)
	}
	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, `"data"`) {
		t.Errorf("envelope shape broken: %q", out)
	}
}

func TestPrintAPISuccess_NoNoticeWhenUnset(t *testing.T) {
	SetNotice(nil)
	var buf bytes.Buffer
	if err := PrintAPISuccess(&buf, map[string]any{"id": "1"}, FormatJSON, ""); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	if strings.Contains(buf.String(), "_notice") {
		t.Errorf("no notice expected, got %q", buf.String())
	}
}

// pretty/table/ndjson are not the machine envelope and must never carry _notice.
func TestPrintAPISuccess_NoticeOnlyInJSON(t *testing.T) {
	SetNotice(map[string]any{"update": map[string]any{"latest": "2.0.0"}})
	t.Cleanup(func() { SetNotice(nil) })
	for _, f := range []string{FormatPretty, FormatTable, FormatNDJSON} {
		var buf bytes.Buffer
		if err := PrintAPISuccess(&buf, map[string]any{"id": "1"}, f, ""); err != nil {
			t.Fatalf("PrintAPISuccess(%s): %v", f, err)
		}
		if strings.Contains(buf.String(), "_notice") {
			t.Errorf("format %s must not carry _notice: %q", f, buf.String())
		}
	}
}
