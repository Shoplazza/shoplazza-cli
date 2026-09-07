package cmdutil

import (
	"strings"
	"testing"
)

// ── ResolveInput ──────────────────────────────────────────────────────────────

func TestResolveInput_Empty(t *testing.T) {
	got, err := ResolveInput("", nil)
	if err != nil || got != "" {
		t.Errorf("empty: got (%q, %v)", got, err)
	}
}

func TestResolveInput_PlainString(t *testing.T) {
	got, err := ResolveInput(`{"k":"v"}`, nil)
	if err != nil || got != `{"k":"v"}` {
		t.Errorf("plain: got (%q, %v)", got, err)
	}
}

func TestResolveInput_SingleQuoteStrip(t *testing.T) {
	got, err := ResolveInput(`'{"k":"v"}'`, nil)
	if err != nil || got != `{"k":"v"}` {
		t.Errorf("single-quote strip: got (%q, %v)", got, err)
	}
}

func TestResolveInput_Stdin(t *testing.T) {
	r := strings.NewReader(`{"from":"stdin"}`)
	got, err := ResolveInput("-", r)
	if err != nil || got != `{"from":"stdin"}` {
		t.Errorf("stdin: got (%q, %v)", got, err)
	}
}

func TestResolveInput_Refusals(t *testing.T) {
	cases := []struct{ name, in string }{
		{"stdin is nil", "-"},
		{"empty @-file path", "@ "},
		{"missing @-file", "@/nonexistent/path/file.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ResolveInput(c.in, nil); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestResolveInput_StdinEmpty(t *testing.T) {
	r := strings.NewReader("   ")
	_, err := ResolveInput("-", r)
	if err == nil {
		t.Error("expected error for empty stdin")
	}
}

// ── ParseOptionalBody ─────────────────────────────────────────────────────────

func TestParseOptionalBody_GetReturnsNil(t *testing.T) {
	body, err := ParseOptionalBody("GET", `{"x":1}`, nil)
	if err != nil || body != nil {
		t.Errorf("GET: got (%v, %v) want (nil, nil)", body, err)
	}
}

func TestParseOptionalBody_PostEmpty(t *testing.T) {
	body, err := ParseOptionalBody("POST", "", nil)
	if err != nil || body != nil {
		t.Errorf("POST empty: got (%v, %v) want (nil, nil)", body, err)
	}
}

func TestParseOptionalBody_PostValidJSON(t *testing.T) {
	body, err := ParseOptionalBody("POST", `{"key":"val"}`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body not map: %T", body)
	}
	if m["key"] != "val" {
		t.Errorf("body[key] = %v, want val", m["key"])
	}
}

// Every verb that may carry a body parses one.
func TestParseOptionalBody_ValidJSON(t *testing.T) {
	for _, method := range []string{"PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			body, err := ParseOptionalBody(method, `{"x":1}`, nil)
			if err != nil || body == nil {
				t.Errorf("got (%v, %v)", body, err)
			}
		})
	}
}

func TestParseOptionalBody_InvalidJSON(t *testing.T) {
	_, err := ParseOptionalBody("POST", `not-json`, nil)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

// ── ParseJSONMap ──────────────────────────────────────────────────────────────

func TestParseJSONMap_Empty(t *testing.T) {
	m, err := ParseJSONMap("", "test", nil)
	if err != nil || len(m) != 0 {
		t.Errorf("empty: got (%v, %v)", m, err)
	}
}

func TestParseJSONMap_ValidJSON(t *testing.T) {
	m, err := ParseJSONMap(`{"k":"v"}`, "test", nil)
	if err != nil || m["k"] != "v" {
		t.Errorf("valid: got (%v, %v)", m, err)
	}
}

func TestParseJSONMap_InvalidJSON(t *testing.T) {
	_, err := ParseJSONMap(`[1,2,3]`, "test", nil)
	if err == nil {
		t.Error("expected error for JSON array (not object)")
	}
}
