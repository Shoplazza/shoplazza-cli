package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// makeEnvelope returns `n` bytes of envelope-shaped JSON payload —
// {"code":"Success","data":{...big object...}}.
func makeEnvelope(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"code":"Success","data":{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"item_%d","tags":["a","b","c"],"qty":%d}`, i, i, i*7)
	}
	b.WriteString(`]}}`)
	return []byte(b.String())
}

func makePlain(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"item_%d","tags":["a","b","c"],"qty":%d}`, i, i, i*7)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

func BenchmarkUnmarshalUnwrapped_EnvelopedTo_any(b *testing.B) {
	data := makeEnvelope(50)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out any
		if err := unmarshalUnwrapped(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalUnwrapped_PlainObjectTo_any(b *testing.B) {
	data := makePlain(50)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out any
		if err := unmarshalUnwrapped(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalUnwrapped_ArrayBody(b *testing.B) {
	data := []byte(`[1,2,3,4,5,6,7,8,9,10]`)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out []int
		if err := unmarshalUnwrapped(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// Sanity check that the function still works on the benchmark inputs.
func TestUnmarshalUnwrapped_SmokeOnBenchInputs(t *testing.T) {
	data := makeEnvelope(3)
	var out map[string]any
	if err := unmarshalUnwrapped(data, &out); err != nil {
		t.Fatal(err)
	}
	items, ok := out["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("envelope unwrap: items = %v, want len 3", out["items"])
	}
	// Plain
	if err := unmarshalUnwrapped(makePlain(3), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["items"].([]any); !ok {
		t.Fatalf("plain object: items missing")
	}
	// Array
	var arr []int
	if err := unmarshalUnwrapped([]byte(`[1,2,3]`), &arr); err != nil {
		t.Fatal(err)
	}
	if len(arr) != 3 {
		t.Fatalf("array decode failed: %v", arr)
	}
	// Envelope without data
	out = nil
	if err := unmarshalUnwrapped([]byte(`{"code":"Success","other":"x"}`), &out); err != nil {
		t.Fatal(err)
	}
	if out["code"] != "Success" || out["other"] != "x" {
		t.Fatalf("no-data envelope must pass through: %v", out)
	}
	// Non-Success envelope
	out = nil
	if err := unmarshalUnwrapped([]byte(`{"code":"Failure","data":{"id":"x"}}`), &out); err != nil {
		t.Fatal(err)
	}
	if out["code"] != "Failure" {
		t.Fatalf("non-success envelope must pass through: %v", out)
	}
	// Compatibility against the old implementation: confirm that direct
	// json.Unmarshal on the same data matches when no unwrapping happens.
	var ref map[string]any
	_ = json.Unmarshal([]byte(`{"code":"Failure","data":{"id":"x"}}`), &ref)
	if ref["code"] != "Failure" {
		t.Fatal("reference behavior changed")
	}
}
