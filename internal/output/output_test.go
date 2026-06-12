package output_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

// ── PrintJSON ─────────────────────────────────────────────────────────────────

func TestPrintJSON_Map(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintJSON(&buf, map[string]any{"ok": true, "count": 3})
	if err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"ok"`) || !strings.Contains(got, `"count"`) {
		t.Errorf("PrintJSON output missing expected keys: %s", got)
	}
	// must end with newline
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("PrintJSON output missing trailing newline")
	}
}

func TestPrintJSON_Slice(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintJSON(&buf, []any{1, 2, 3})
	if err != nil {
		t.Fatalf("PrintJSON slice: %v", err)
	}
	if !strings.Contains(buf.String(), "[") {
		t.Errorf("PrintJSON slice missing '[': %s", buf.String())
	}
}

func TestPrintJSON_Nil(t *testing.T) {
	var buf bytes.Buffer
	if err := output.PrintJSON(&buf, nil); err != nil {
		t.Fatalf("PrintJSON nil: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "null" {
		t.Errorf("PrintJSON(nil) = %q, want null", buf.String())
	}
}

// ── PrintText ─────────────────────────────────────────────────────────────────

func TestPrintText(t *testing.T) {
	var buf bytes.Buffer
	if err := output.PrintText(&buf, "hello world"); err != nil {
		t.Fatalf("PrintText: %v", err)
	}
	if buf.String() != "hello world\n" {
		t.Errorf("PrintText = %q, want %q", buf.String(), "hello world\n")
	}
}

func TestPrintText_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := output.PrintText(&buf, ""); err != nil {
		t.Fatalf("PrintText empty: %v", err)
	}
	if buf.String() != "\n" {
		t.Errorf("PrintText('') = %q, want newline", buf.String())
	}
}

// ── ExitError.Error() nil-detail branches ────────────────────────────────────

func TestExitError_Error_NilDetailWrappedErr(t *testing.T) {
	cause := errors.New("wrapped cause")
	err := &output.ExitError{Code: 5, Err: cause}
	if err.Error() != "wrapped cause" {
		t.Errorf("Error() = %q, want 'wrapped cause'", err.Error())
	}
}

func TestExitError_Error_NilDetailNilErr(t *testing.T) {
	err := &output.ExitError{Code: 7}
	if err.Error() != "exit 7" {
		t.Errorf("Error() = %q, want 'exit 7'", err.Error())
	}
}

// ── PrintAPISuccess with --jq ─────────────────────────────────────────────────

func TestPrintAPISuccess_JQ_FieldFromEnvelope(t *testing.T) {
	var buf bytes.Buffer
	body := map[string]any{"foo": "bar"}
	if err := output.PrintAPISuccess(&buf, body, "json", ".data.foo"); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	if buf.String() != "bar\n" {
		t.Errorf("jq .data.foo = %q, want %q", buf.String(), "bar\n")
	}
}

func TestPrintAPISuccess_JQ_OkField(t *testing.T) {
	var buf bytes.Buffer
	if err := output.PrintAPISuccess(&buf, map[string]any{}, "json", ".ok"); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	if buf.String() != "true\n" {
		t.Errorf("jq .ok = %q, want %q", buf.String(), "true\n")
	}
}

func TestPrintAPISuccess_JQ_ObjectResult(t *testing.T) {
	var buf bytes.Buffer
	body := map[string]any{"name": "Alice", "age": float64(30)}
	if err := output.PrintAPISuccess(&buf, body, "json", ".data"); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	// Object result is indented JSON (2-space, multi-line).
	if !strings.HasPrefix(got, "{\n") || !strings.HasSuffix(got, "}") {
		t.Errorf("object result = %q, want indented JSON object", got)
	}
	if !strings.Contains(got, `"name": "Alice"`) || !strings.Contains(got, `"age": 30`) {
		t.Errorf("object result = %q, missing expected fields (with spaces after colon)", got)
	}
}

func TestPrintAPISuccess_JQ_MultipleResults(t *testing.T) {
	var buf bytes.Buffer
	body := map[string]any{"items": []any{
		map[string]any{"id": "a"},
		map[string]any{"id": "b"},
		map[string]any{"id": "c"},
	}}
	if err := output.PrintAPISuccess(&buf, body, "json", ".data.items[].id"); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	if buf.String() != "a\nb\nc\n" {
		t.Errorf("jq multi = %q, want %q", buf.String(), "a\nb\nc\n")
	}
}

func TestPrintAPISuccess_JQ_InvalidExpr(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintAPISuccess(&buf, map[string]any{}, "json", ".[")
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid --jq expression") {
		t.Errorf("err = %q, want it to contain 'invalid --jq expression'", err.Error())
	}
}

func TestPrintAPISuccess_JQ_WithPrettyFormat(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintAPISuccess(&buf, map[string]any{}, "pretty", ".ok")
	if err == nil || !strings.Contains(err.Error(), "--jq requires --format json") {
		t.Errorf("expected '--jq requires --format json' error, got %v", err)
	}
}

func TestPrintAPISuccess_JQ_WithTableFormat(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintAPISuccess(&buf, map[string]any{}, "table", ".ok")
	if err == nil || !strings.Contains(err.Error(), "--jq requires --format json") {
		t.Errorf("expected '--jq requires --format json' error, got %v", err)
	}
}

func TestPrintAPISuccess_JQ_EmptyPassthrough(t *testing.T) {
	var buf bytes.Buffer
	body := map[string]any{"foo": "bar"}
	if err := output.PrintAPISuccess(&buf, body, "json", ""); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"ok": true`) || !strings.Contains(got, `"foo": "bar"`) {
		t.Errorf("empty-jq passthrough = %q, want envelope with body", got)
	}
}

func TestPrintAPISuccess_JQ_NilBody(t *testing.T) {
	var buf bytes.Buffer
	if err := output.PrintAPISuccess(&buf, nil, "json", ".data"); err != nil {
		t.Fatalf("PrintAPISuccess: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Errorf("nil body + .data = %q, want {}", buf.String())
	}
}

func TestPrintAPISuccess_JQ_RuntimeError(t *testing.T) {
	var buf bytes.Buffer
	// `.foo + 1` on a number — gojq raises a runtime type error.
	err := output.PrintAPISuccess(&buf, map[string]any{"x": float64(1)}, "json", ".data.x | .foo")
	if err == nil {
		t.Fatalf("expected runtime jq error, got nil")
	}
	if !strings.Contains(err.Error(), "jq:") {
		t.Errorf("err = %q, want it to contain 'jq:'", err.Error())
	}
}

func TestPrintBody_JQ(t *testing.T) {
	var buf bytes.Buffer
	body := map[string]any{"dry_run": true, "request": map[string]any{"method": "GET"}}
	if err := output.PrintBody(&buf, body, "json", ".request.method"); err != nil {
		t.Fatalf("PrintBody: %v", err)
	}
	if buf.String() != "GET\n" {
		t.Errorf("PrintBody jq = %q, want %q", buf.String(), "GET\n")
	}
}

func TestPrintBody_JQ_WithPrettyFormat(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintBody(&buf, map[string]any{}, "pretty", ".foo")
	if err == nil || !strings.Contains(err.Error(), "--jq requires --format json") {
		t.Errorf("expected '--jq requires --format json' error, got %v", err)
	}
}

// ── WriteErrorEnvelope nil/no-op paths ───────────────────────────────────────

func TestWriteErrorEnvelope_Nil(t *testing.T) {
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, nil) // must not panic or write anything
	if buf.Len() != 0 {
		t.Errorf("WriteErrorEnvelope(nil) wrote %q, want empty", buf.String())
	}
}

func TestWriteErrorEnvelope_NilDetail(t *testing.T) {
	var buf bytes.Buffer
	err := &output.ExitError{Code: 1, Err: errors.New("bare")}
	output.WriteErrorEnvelope(&buf, err) // Detail is nil → no-op
	if buf.Len() != 0 {
		t.Errorf("WriteErrorEnvelope(nil Detail) wrote %q, want empty", buf.String())
	}
}
