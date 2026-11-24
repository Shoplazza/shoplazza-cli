package cmdutil

import (
	"encoding/json"
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
	const in = `{"id":638982951773671231,"key":"val","price":1.50}`
	body, err := ParseOptionalBody("POST", in, nil)
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
	if out, _ := json.Marshal(body); string(out) != in {
		t.Errorf("re-encoded body = %s, want %s", out, in)
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
	for _, c := range []struct{ in, want string }{
		{`not-json`, `--data is not valid JSON object: invalid character 'o' in literal null (expecting 'u') (line 1, column 2)`},
		{`{"a":1} x`, `--data is not valid JSON object: unexpected 'x' after the end of the object (line 1, column 9)`},
		{`{"a":1}{"b":2}`, `--data is not valid JSON object: unexpected '{' after the end of the object (line 1, column 8)`},
		{`[1,2] ]`, `--data is not valid JSON object: unexpected ']' after the end of the array (line 1, column 7)`},
	} {
		if _, err := ParseOptionalBody("POST", c.in, nil); err == nil || err.Error() != c.want {
			t.Errorf("%s: error = %v, want %s", c.in, err, c.want)
		}
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
	m, err := ParseJSONMap(`{"k":"v","id":638982951773671231}`, "test", nil)
	if err != nil || m["k"] != "v" || m["id"] != json.Number("638982951773671231") {
		t.Errorf("valid: got (%v, %v)", m, err)
	}
}

func TestParseJSONMap_InvalidJSON(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`[1,2,3]`, `--data must be a JSON object, got an array`},
		{`"text"`, `--data must be a JSON object, got a string`},
		{`{"k":"v"} x`, `--data is not valid JSON object: unexpected 'x' after the end of the object (line 1, column 11)`},
		{`{"product":{"title":"a"}}}`, `--data is not valid JSON object: unexpected '}' after the end of the object (line 1, column 26)`},
		{`{"customer":{"first_name":"a"} "x":1}`, `--data is not valid JSON object: invalid character '"' after object key:value pair (line 1, column 32)`},
		{"{\n  \"a\": 1\n  \"b\": 2\n}", `--data is not valid JSON object: invalid character '"' after object key:value pair (line 3, column 3)`},
		{`{'product':{}}`, `--data is not valid JSON object: invalid character '\'' looking for beginning of object key string (line 1, column 2)`},
		{`{"product":{"title":"a"}`, `--data is not valid JSON object: a closing } or ] is missing`},
		{` `, `--data is empty`},
	} {
		if _, err := ParseJSONMap(c.in, "--data", nil); err == nil || err.Error() != c.want {
			t.Errorf("%q: error = %v, want %s", c.in, err, c.want)
		}
	}
}
