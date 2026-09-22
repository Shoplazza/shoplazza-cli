package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 1. Backward compat: Headers nil → original JSON path
func TestDoRaw_DefaultJSONPath_BackwardCompat(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"code":"Success","data":{"ok":true}}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	_, err := c.DoRaw(context.Background(), RawRequest{
		Method: "POST", Path: "/x",
		Data: map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatalf("DoRaw err: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotCT)
	}
	if !strings.Contains(gotBody, `"k":"v"`) {
		t.Fatalf("body should be JSON-marshalled: %q", gotBody)
	}
}

// 2. Multipart path: Headers["Content-Type"] non-empty + Data = io.Reader → raw passthrough
func TestDoRaw_MultipartPath(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"code":"Success"}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	mpBody := bytes.NewBufferString("--boundary\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\nraw-content\r\n--boundary--")
	_, err := c.DoRaw(context.Background(), RawRequest{
		Method: "POST", Path: "/upload",
		Data:    mpBody,
		Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=boundary"},
	})
	if err != nil {
		t.Fatalf("DoRaw err: %v", err)
	}
	if gotCT != "multipart/form-data; boundary=boundary" {
		t.Fatalf("Content-Type = %q, want multipart", gotCT)
	}
	if !strings.Contains(gotBody, "raw-content") {
		t.Fatalf("body should be raw passthrough (not JSON-marshalled): %q", gotBody)
	}
}

// 3. Type guard: Headers["Content-Type"] set but Data is unsupported → fail-fast
func TestDoRaw_MultipartPathRejectsUnsupportedDataType(t *testing.T) {
	c := New("http://example.invalid")
	_, err := c.DoRaw(context.Background(), RawRequest{
		Method: "POST", Path: "/x",
		Data:    struct{ Name string }{Name: "x"}, // struct, not io.Reader/[]byte/nil
		Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=b"},
	})
	if err == nil {
		t.Fatalf("expected error for struct Data + multipart Content-Type")
	}
	if !strings.Contains(err.Error(), "Data is not io.Reader/[]byte/nil") {
		t.Fatalf("error should explain type mismatch: %v", err)
	}
}

// 4. Security: c.Headers["Access-Token"] always wins (caller cannot forge)
func TestDoRaw_ClientHeadersWinOverRequestHeaders(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("Access-Token")
		_, _ = io.WriteString(w, `{"code":"Success"}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	c.Headers["Access-Token"] = "real-token"
	_, _ = c.DoRaw(context.Background(), RawRequest{
		Method: "POST", Path: "/x",
		Data:    nil,
		Headers: map[string]string{"Access-Token": "fake-token"},
	})
	if gotToken != "real-token" {
		t.Fatalf("c.Headers must win; got Access-Token = %q", gotToken)
	}
}

// 5. Custom non-sensitive headers pass through
func TestDoRaw_CustomHeadersPassThrough(t *testing.T) {
	var gotIdem, gotTrace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		gotTrace = r.Header.Get("X-Trace-Id")
		_, _ = io.WriteString(w, `{"code":"Success"}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	_, _ = c.DoRaw(context.Background(), RawRequest{
		Method: "POST", Path: "/x",
		Data:    map[string]any{"k": "v"},
		Headers: map[string]string{"Idempotency-Key": "abc-123", "X-Trace-Id": "trace-x"},
	})
	if gotIdem != "abc-123" || gotTrace != "trace-x" {
		t.Fatalf("custom headers not set; idem=%q trace=%q", gotIdem, gotTrace)
	}
}

// 6. Empty-string entry is skipped
func TestDoRaw_EmptyHeaderValueSkipped(t *testing.T) {
	var hadKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadKey = r.Header["X-Empty"]
		_, _ = io.WriteString(w, `{"code":"Success"}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	_, _ = c.DoRaw(context.Background(), RawRequest{
		Method: "POST", Path: "/x",
		Headers: map[string]string{"X-Empty": ""},
	})
	if hadKey {
		t.Fatalf("X-Empty should be skipped, not sent")
	}
}

// Contract: Data == nil + custom Content-Type is allowed. Body must be empty on the wire.
func TestDoRaw_MultipartPathAllowsNilData(t *testing.T) {
	var gotCT string
	var gotBodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBodyLen = len(b)
		_, _ = io.WriteString(w, `{"code":"Success"}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	_, err := c.DoRaw(context.Background(), RawRequest{
		Method:  "POST",
		Path:    "/x",
		Data:    nil,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	})
	if err != nil {
		t.Fatalf("DoRaw err: %v", err)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if gotBodyLen != 0 {
		t.Errorf("body should be empty, got %d bytes", gotBodyLen)
	}
}

// Contract: Data: []byte + custom Content-Type passes raw bytes through.
func TestDoRaw_MultipartPathAcceptsByteSlice(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"code":"Success"}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	_, err := c.DoRaw(context.Background(), RawRequest{
		Method:  "POST",
		Path:    "/x",
		Data:    []byte("<xml>raw</xml>"),
		Headers: map[string]string{"Content-Type": "application/xml"},
	})
	if err != nil {
		t.Fatalf("DoRaw err: %v", err)
	}
	if gotCT != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", gotCT)
	}
	if gotBody != "<xml>raw</xml>" {
		t.Errorf("body raw passthrough failed: %q", gotBody)
	}
}
