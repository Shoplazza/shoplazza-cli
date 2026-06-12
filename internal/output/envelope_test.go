package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

// Wire-compat regression: when Extra is unset the envelope JSON carries
// exactly the same well-known keys with the same omitempty semantics as
// before MarshalJSON was customised. Key ordering may differ; tests
// throughout the repo decode via json.Unmarshal and never byte-compare.
func TestErrDetail_MarshalJSON_WireCompat_NoExtras(t *testing.T) {
	e := output.ErrWithHint(output.ExitAuth, output.TypeAuth, "session expired", "run login")
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, e)

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("envelope not valid JSON: %v\nbody: %s", err, buf.String())
	}
	if parsed["ok"] != false {
		t.Errorf("ok = %v, want false", parsed["ok"])
	}
	errMap, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatalf("error block missing or not object: %v", parsed["error"])
	}
	if errMap["type"] != "auth" {
		t.Errorf("type = %v, want auth", errMap["type"])
	}
	if errMap["message"] != "session expired" {
		t.Errorf("message = %v", errMap["message"])
	}
	if errMap["hint"] != "run login" {
		t.Errorf("hint = %v", errMap["hint"])
	}
	if _, has := errMap["code"]; has {
		t.Errorf("code must be omitted when empty (got %v)", errMap["code"])
	}
	if _, has := errMap["detail"]; has {
		t.Errorf("detail must be omitted when nil (got %v)", errMap["detail"])
	}
	if _, has := errMap["extra"]; has {
		t.Errorf("extra key must not appear in wire format: %v", errMap["extra"])
	}
}

// Hint and code omission semantics preserved when not set.
func TestErrDetail_MarshalJSON_OmitsEmpty(t *testing.T) {
	e := output.ErrAuth("plain message")
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, e)

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	errMap := parsed["error"].(map[string]any)
	if _, has := errMap["hint"]; has {
		t.Errorf("hint must be omitted when empty (got %v)", errMap["hint"])
	}
	if _, has := errMap["code"]; has {
		t.Errorf("code must be omitted when empty (got %v)", errMap["code"])
	}
}

// Extra entries merge into the top level of the error object (not nested
// under "extra"). Domain helpers depend on this shape.
func TestErrDetail_MarshalJSON_ExtraMergesTopLevel(t *testing.T) {
	e := output.Errorf(output.ExitNetwork, output.TypeNetwork, "timeout").
		WithField("task", map[string]any{"id": "t1", "status": float64(0)}).
		WithField("elapsed_seconds", 180.2)

	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, e)

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v\nbody: %s", err, buf.String())
	}
	errMap := parsed["error"].(map[string]any)
	if errMap["type"] != "network" {
		t.Errorf("type = %v", errMap["type"])
	}
	if errMap["elapsed_seconds"] != 180.2 {
		t.Errorf("elapsed_seconds = %v (type %T), want 180.2", errMap["elapsed_seconds"], errMap["elapsed_seconds"])
	}
	task, ok := errMap["task"].(map[string]any)
	if !ok {
		t.Fatalf("task = %v, want map", errMap["task"])
	}
	if task["id"] != "t1" {
		t.Errorf("task.id = %v", task["id"])
	}
}

// API errors (ErrAPI) keep their string "code" (business code) in the
// wire format. WithField does not interfere with that field.
func TestErrDetail_MarshalJSON_APIErrorRetainsCode(t *testing.T) {
	e := output.ErrAPI(404, `{"code":"NOT_FOUND","message":"theme missing"}`, "req-1")
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, e)

	var parsed map[string]any
	_ = json.Unmarshal(buf.Bytes(), &parsed)
	errMap := parsed["error"].(map[string]any)
	if errMap["code"] != "NOT_FOUND" {
		t.Errorf("code = %v, want NOT_FOUND", errMap["code"])
	}
	if errMap["message"] != "theme missing" {
		t.Errorf("message = %v", errMap["message"])
	}
	detail, ok := errMap["detail"].(map[string]any)
	if !ok {
		t.Fatalf("detail = %v, want object", errMap["detail"])
	}
	if detail["status_code"] != float64(404) {
		t.Errorf("detail.status_code = %v", detail["status_code"])
	}
}

// WithHint sets the hint on a freshly-built error and chains.
func TestExitError_WithHint(t *testing.T) {
	e := output.Errorf(output.ExitInternal, output.TypeInternal, "boom").
		WithHint("retry later")
	if e.Detail.Hint != "retry later" {
		t.Errorf("Hint = %q", e.Detail.Hint)
	}
}

// Envelope() exposes the int exit code under "code" and includes Extra.
func TestExitError_Envelope_Shape(t *testing.T) {
	e := output.Errorf(output.ExitNetwork, output.TypeNetwork, "timeout").
		WithField("elapsed_seconds", 180.2).
		WithHint("retry")
	env := e.Envelope()
	if env["code"] != output.ExitNetwork {
		t.Errorf("code = %v, want %d", env["code"], output.ExitNetwork)
	}
	if env["type"] != "network" {
		t.Errorf("type = %v", env["type"])
	}
	if env["message"] != "timeout" {
		t.Errorf("message = %v", env["message"])
	}
	if env["hint"] != "retry" {
		t.Errorf("hint = %v", env["hint"])
	}
	if env["elapsed_seconds"] != 180.2 {
		t.Errorf("elapsed_seconds = %v", env["elapsed_seconds"])
	}
}

// Envelope() omits message/hint when unset but always carries code+type.
func TestExitError_Envelope_OmitsEmpty(t *testing.T) {
	e := &output.ExitError{Code: output.ExitInternal, Detail: &output.ErrDetail{Type: output.TypeInternal}}
	env := e.Envelope()
	if env["code"] != output.ExitInternal {
		t.Errorf("code = %v", env["code"])
	}
	if env["type"] != "internal" {
		t.Errorf("type = %v", env["type"])
	}
	if _, has := env["message"]; has {
		t.Errorf("message must be omitted when empty: %v", env["message"])
	}
	if _, has := env["hint"]; has {
		t.Errorf("hint must be omitted when empty: %v", env["hint"])
	}
}
