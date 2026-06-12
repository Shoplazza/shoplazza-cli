package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

func TestExitErrorMessage(t *testing.T) {
	err := output.Errorf(output.ExitAuth, "auth", "token expired for %s", "store.myshoplazza.com")
	if err.Code != output.ExitAuth {
		t.Errorf("Code = %d, want %d", err.Code, output.ExitAuth)
	}
	if !strings.Contains(err.Error(), "token expired") {
		t.Errorf("Error() = %q, want to contain 'token expired'", err.Error())
	}
}

func TestErrWithHint(t *testing.T) {
	err := output.ErrWithHint(output.ExitValidation, "validation", "missing flag", "use --store <domain>")
	if err.Detail.Hint != "use --store <domain>" {
		t.Errorf("Hint = %q, want 'use --store <domain>'", err.Detail.Hint)
	}
	if err.Code != output.ExitValidation {
		t.Errorf("Code = %d, want %d", err.Code, output.ExitValidation)
	}
}

func TestWriteErrorEnvelope_JSON(t *testing.T) {
	err := output.ErrWithHint(output.ExitAuth, "auth", "session expired", "run auth login again")
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, err)

	var env map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &env); jsonErr != nil {
		t.Fatalf("envelope is not valid JSON: %v\nbody: %s", jsonErr, buf.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Error("ok should be false")
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj["type"] != "auth" {
		t.Errorf("error.type = %v, want 'auth'", errObj["type"])
	}
	if errObj["message"] != "session expired" {
		t.Errorf("error.message = %v, want 'session expired'", errObj["message"])
	}
	if errObj["hint"] != "run auth login again" {
		t.Errorf("error.hint = %v, want 'run auth login again'", errObj["hint"])
	}
}

func TestWriteErrorEnvelope_DoesNotHTMLEscape(t *testing.T) {
	err := output.ErrWithHint(output.ExitValidation, "validation", "missing flag", "pass --id <extensionId> or run inside extensions/<id>/")
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, err)
	raw := buf.String()
	if strings.Contains(raw, "\\u003c") || strings.Contains(raw, "\\u003e") {
		t.Errorf("envelope must not HTML-escape '<'/'>' to \\u003c/\\u003e: %s", raw)
	}
	if !strings.Contains(raw, "<extensionId>") {
		t.Errorf("envelope should contain the literal hint with <extensionId>: %s", raw)
	}
	// still valid, decodable JSON
	var env map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &env); jsonErr != nil {
		t.Fatalf("envelope is not valid JSON: %v", jsonErr)
	}
}

func TestWriteErrorEnvelope_NoHint(t *testing.T) {
	err := output.ErrAuth("network timeout")
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, err)

	var env map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &env); jsonErr != nil {
		t.Fatalf("not valid JSON: %v", jsonErr)
	}
	errObj, _ := env["error"].(map[string]any)
	// hint should be absent when empty
	if _, hasHint := errObj["hint"]; hasHint {
		t.Error("hint field should not appear when empty")
	}
}

func TestExitErrorWraps(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	err := output.ErrNetwork("cannot reach API: %v", cause)
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the wrapped cause")
	}
}

// TestErrAPI_5xxPassesThroughMessage: a 5xx error surfaces the server's OWN
// message unchanged (no masking), so an openapi 500 like
// {"code":"ServerError","message":"..."} reaches the user verbatim. The generic
// "contact staff" message is used ONLY when the body carries no message.
func TestErrAPI_5xxPassesThroughMessage(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		err := output.ErrAPI(status, `{"code":"ServerError","message":"db is down"}`, "req-1")
		if err.Code != output.ExitAPI {
			t.Errorf("status %d: Code = %d, want %d", status, err.Code, output.ExitAPI)
		}
		if err.Detail.Message != "db is down" {
			t.Errorf("status %d: Message = %q, want server message passed through verbatim", status, err.Detail.Message)
		}
		if err.Detail.Detail == nil || err.Detail.Detail.StatusCode != status {
			t.Errorf("status %d: status code not preserved in detail: %+v", status, err.Detail.Detail)
		}
		if err.Detail.Hint != "" {
			t.Errorf("status %d: expected no hint, got %q", status, err.Detail.Hint)
		}
	}
}

// TestErrAPI_5xxEmptyBodyFallsBackToGeneric: when a 5xx carries no usable
// message, fall back to the generic "contact staff" message rather than an
// empty string.
func TestErrAPI_5xxEmptyBodyFallsBackToGeneric(t *testing.T) {
	for _, body := range []string{``, `{}`, `{"code":"ServerError"}`, `{"message":""}`} {
		err := output.ErrAPI(500, body, "")
		if !strings.Contains(err.Detail.Message, "Please contact Shoplazza staff") {
			t.Errorf("empty 5xx body %q: Message = %q, want generic fallback", body, err.Detail.Message)
		}
	}
}

// TestWithEndpoint_RendersInDetail verifies the failing endpoint (method+path)
// lands in the error.detail block, so a 500 self-identifies which request broke.
func TestWithEndpoint_RendersInDetail(t *testing.T) {
	err := output.ErrAPI(500, `{"code":"ServerError"}`, "").
		WithEndpoint("GET", "/openapi/2026-01/themes/task/abc")
	if err.Detail.Detail == nil {
		t.Fatal("detail context is nil")
	}
	if err.Detail.Detail.Method != "GET" || err.Detail.Detail.Path != "/openapi/2026-01/themes/task/abc" {
		t.Errorf("endpoint not in detail: %+v", err.Detail.Detail)
	}
	// And it must survive serialization into the wire envelope (parse it back
	// so we don't depend on the encoder's whitespace/indentation).
	var buf bytes.Buffer
	output.WriteErrorEnvelope(&buf, err)
	var env struct {
		Error struct {
			Detail struct {
				StatusCode int    `json:"status_code"`
				Method     string `json:"method"`
				Path       string `json:"path"`
			} `json:"detail"`
		} `json:"error"`
	}
	if e := json.Unmarshal(buf.Bytes(), &env); e != nil {
		t.Fatalf("envelope is not valid JSON: %v\n%s", e, buf.String())
	}
	d := env.Error.Detail
	if d.StatusCode != 500 || d.Method != "GET" || d.Path != "/openapi/2026-01/themes/task/abc" {
		t.Errorf("envelope detail = %+v, want {500 GET /openapi/2026-01/themes/task/abc}", d)
	}
}

// TestWithEndpoint_NoopWhenEmpty: enrichment with empty method+path must not
// fabricate an empty detail block (keeps non-HTTP errors clean).
func TestWithEndpoint_NoopWhenEmpty(t *testing.T) {
	err := output.ErrValidation("bad input").WithEndpoint("", "")
	if err.Detail != nil && err.Detail.Detail != nil {
		t.Errorf("empty endpoint should not create a detail context: %+v", err.Detail.Detail)
	}
}

func TestErrAPI_Non5xxCleanMessage(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"errors array", `{"errors":["Scopes is required"]}`, "Scopes is required"},
		{"errors joined", `{"errors":["a","b"]}`, "a; b"},
		{"message field", `{"message":"bad request"}`, "bad request"},
		{"error field", `{"error":"nope"}`, "nope"},
		{"raw fallback", `plain text boom`, "plain text boom"},
		{"bare json string", `"Unauthorized"`, "Unauthorized"},
		{"bare json string trailing newline", "\"Unauthorized\"\n", "Unauthorized"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := output.ErrAPI(422, c.body, "")
			if err.Detail.Message != c.want {
				t.Errorf("Message = %q, want %q", err.Detail.Message, c.want)
			}
			if strings.Contains(err.Detail.Message, "http request failed") {
				t.Errorf("raw wrapper leaked: %q", err.Detail.Message)
			}
			if err.Detail.Hint != "" {
				t.Errorf("expected no hint, got %q", err.Detail.Hint)
			}
		})
	}
}

func TestErrAPI_EmptyBody(t *testing.T) {
	err := output.ErrAPI(404, "", "")
	if err.Detail.Message != "request failed with status 404" {
		t.Errorf("Message = %q, want minimal status message", err.Detail.Message)
	}
}

func TestErrAPIAuthHint(t *testing.T) {
	const hint = "run 'shoplazza auth login -s --scope' or 'shoplazza store use -s' to re-authenticate"
	body := `{"code":"session_not_found","errors":["store not found: ssa.stg.myshoplaza.com"]}`
	err := output.ErrAPIAuthHint(404, body, hint)

	if err.Code != output.ExitAuth {
		t.Errorf("Code = %d, want ExitAuth", err.Code)
	}
	if err.Detail.Type != output.TypeAuth {
		t.Errorf("Type = %q, want auth", err.Detail.Type)
	}
	if err.Detail.Code != "session_not_found" {
		t.Errorf("Code = %q, want session_not_found", err.Detail.Code)
	}
	if err.Detail.Message != "store not found: ssa.stg.myshoplaza.com" {
		t.Errorf("Message = %q, want clean parsed message", err.Detail.Message)
	}
	if strings.Contains(err.Detail.Message, "http request failed") {
		t.Errorf("raw wrapper leaked: %q", err.Detail.Message)
	}
	if err.Detail.Hint != hint {
		t.Errorf("Hint = %q, want preserved hint", err.Detail.Hint)
	}
	if err.Detail.Detail == nil || err.Detail.Detail.StatusCode != 404 {
		t.Errorf("Detail.StatusCode = %v, want 404", err.Detail.Detail)
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		fn   *output.ExitError
		code int
	}{
		{output.ErrAuth("x"), output.ExitAuth},
		{output.ErrValidation("x"), output.ExitValidation},
		{output.ErrNetwork("x"), output.ExitNetwork},
	}
	for _, c := range cases {
		if c.fn.Code != c.code {
			t.Errorf("got code %d, want %d", c.fn.Code, c.code)
		}
	}
}

func TestErrInternal_ExitCodeAndMessage(t *testing.T) {
	err := output.ErrInternal("unexpected marshal failure: %v", "boom")
	if err.Code != output.ExitInternal {
		t.Errorf("code = %d, want %d (ExitInternal)", err.Code, output.ExitInternal)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("message should contain 'boom': %s", err.Error())
	}
}
