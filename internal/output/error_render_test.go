package output

import (
	"bytes"
	"strings"
	"testing"
)

func sampleErr() *ExitError {
	return &ExitError{
		Code: ExitValidation,
		Detail: &ErrDetail{
			Type:    TypeValidation,
			Message: "no target store",
			Hint:    "run 'shoplazza auth store use --store-domain <domain>'",
		},
	}
}

// writeErrorHuman renders a readable line, not JSON, with the hint on its own line.
func TestWriteErrorHuman_Line(t *testing.T) {
	var b bytes.Buffer
	writeErrorHuman(&b, sampleErr(), false) // color off for a stable assertion
	out := b.String()
	if !strings.Contains(out, "Error: no target store") {
		t.Errorf("missing Error line:\n%s", out)
	}
	if !strings.Contains(out, "Hint: run 'shoplazza auth store use") {
		t.Errorf("missing Hint line:\n%s", out)
	}
	if strings.Contains(out, "{") || strings.Contains(out, "\"ok\"") {
		t.Errorf("human error must not be JSON:\n%s", out)
	}
}

// The API request context (endpoint + request id) is surfaced for humans.
func TestWriteErrorHuman_RequestContext(t *testing.T) {
	var b bytes.Buffer
	e := &ExitError{Detail: &ErrDetail{
		Message: "server said no",
		Detail:  &ErrorContext{Method: "POST", Path: "/openapi/x", RequestID: "req_123"},
	}}
	writeErrorHuman(&b, e, false)
	out := b.String()
	for _, want := range []string{"Error: server said no", "Request: POST /openapi/x", "Request ID: req_123"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

// WriteError keeps the JSON envelope for machine formats and for a non-terminal
// stderr even in pretty mode — the contract that agents parse stderr as JSON.
func TestWriteError_MachineStaysJSON(t *testing.T) {
	for _, format := range []string{FormatJSON, FormatNDJSON, FormatCSV, FormatPretty, FormatTable} {
		var b bytes.Buffer // a buffer is never a terminal
		WriteError(&b, sampleErr(), format)
		out := b.String()
		if !strings.Contains(out, `"ok": false`) {
			t.Errorf("format %q to a non-TTY writer must be the JSON envelope:\n%s", format, out)
		}
		if strings.Contains(out, "Error: no target store") {
			t.Errorf("format %q to a non-TTY writer must not be the human line:\n%s", format, out)
		}
	}
}

func TestIsHumanFormat(t *testing.T) {
	for _, f := range []string{FormatPretty, FormatTable} {
		if !isHumanFormat(f) {
			t.Errorf("%q should be human", f)
		}
	}
	for _, f := range []string{FormatJSON, FormatNDJSON, FormatCSV, ""} {
		if isHumanFormat(f) {
			t.Errorf("%q should not be human", f)
		}
	}
}

// colorErrLabel wraps the label only when color is on.
func TestColorErrLabel(t *testing.T) {
	if got := colorErrLabel("Error:", false); got != "Error:" {
		t.Errorf("color off: got %q", got)
	}
	if got := colorErrLabel("Error:", true); !strings.Contains(got, "Error:") || got == "Error:" {
		t.Errorf("color on should wrap: got %q", got)
	}
}
