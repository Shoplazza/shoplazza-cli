package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintNDJSON_ListStreamsOnePerLine(t *testing.T) {
	// Arrange: a list-shaped payload like a shortcut list read.
	payload := map[string]any{
		"products": []any{
			map[string]any{"id": "1", "title": "A"},
			map[string]any{"id": "2", "title": "B"},
		},
		"has_more": true,
	}
	var buf bytes.Buffer

	// Act
	if err := PrintFormatted(&buf, payload, FormatNDJSON); err != nil {
		t.Fatalf("PrintFormatted: %v", err)
	}

	// Assert: exactly two lines, one compact record each, meta dropped.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], `"id":"1"`) || !strings.Contains(lines[1], `"id":"2"`) {
		t.Errorf("unexpected ndjson lines: %q", buf.String())
	}
	if strings.Contains(buf.String(), "has_more") {
		t.Errorf("ndjson should stream records only, got meta: %q", buf.String())
	}
}

func TestPrintNDJSON_BareArray(t *testing.T) {
	var buf bytes.Buffer
	items := []any{map[string]any{"n": float64(1)}, map[string]any{"n": float64(2)}}
	if err := PrintFormatted(&buf, items, FormatNDJSON); err != nil {
		t.Fatalf("PrintFormatted: %v", err)
	}
	if got := strings.Count(buf.String(), "\n"); got != 2 {
		t.Errorf("want 2 lines, got %d: %q", got, buf.String())
	}
}

func TestPrintNDJSON_ObjectSingleLine(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, map[string]any{"id": "x"}, FormatNDJSON); err != nil {
		t.Fatalf("PrintFormatted: %v", err)
	}
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Errorf("want 1 line, got %d: %q", got, buf.String())
	}
}

func TestPrintNDJSON_NoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintFormatted(&buf, map[string]any{"domain": "<my-store>"}, FormatNDJSON); err != nil {
		t.Fatalf("PrintFormatted: %v", err)
	}
	if !strings.Contains(buf.String(), "<my-store>") {
		t.Errorf("expected literal '<my-store>', got %q", buf.String())
	}
}

// PrintAPISuccess must skip the {ok,data} envelope for ndjson and stream the
// records inside data.
func TestPrintAPISuccess_NDJSONSkipsEnvelope(t *testing.T) {
	var buf bytes.Buffer
	body := map[string]any{"orders": []any{
		map[string]any{"id": "o1"},
		map[string]any{"id": "o2"},
	}}
	if err := PrintAPISuccess(&buf, body, FormatNDJSON, ""); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	if strings.Contains(buf.String(), `"ok"`) {
		t.Errorf("ndjson must not wrap in {ok,data}: %q", buf.String())
	}
	if n := strings.Count(strings.TrimRight(buf.String(), "\n"), "\n"); n != 1 {
		t.Errorf("want 2 record lines, got %d: %q", n+1, buf.String())
	}
}

// Subtype and param must serialize into the error envelope, omitted when empty.
func TestErrorEnvelope_SubtypeParam(t *testing.T) {
	e := ErrValidation("bad flag").WithSubtype(SubtypeUnknownFlag).WithParam("--bogus")
	var buf bytes.Buffer
	WriteErrorEnvelope(&buf, e)
	out := buf.String()
	if !strings.Contains(out, `"subtype": "unknown_flag"`) {
		t.Errorf("missing subtype in envelope: %q", out)
	}
	if !strings.Contains(out, `"param": "--bogus"`) {
		t.Errorf("missing param in envelope: %q", out)
	}

	// Omitempty: a plain error has neither key.
	buf.Reset()
	WriteErrorEnvelope(&buf, ErrValidation("plain"))
	if strings.Contains(buf.String(), "subtype") || strings.Contains(buf.String(), "param") {
		t.Errorf("empty subtype/param must be omitted: %q", buf.String())
	}
}
