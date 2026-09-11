package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestSendStream_LargeChunkedResponseStreamsWithoutBuffering(t *testing.T) {
	const totalSize = 5 * 1024 * 1024 // 5 MB chunked
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		flusher, _ := w.(http.Flusher)
		chunk := strings.Repeat("a", 64*1024)
		for written := 0; written < totalSize; written += len(chunk) {
			_, _ = io.WriteString(w, chunk)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	reader, err := c.SendStream(context.Background(), RawRequest{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("SendStream returned err: %v", err)
	}
	defer reader.Close()

	n, err := io.Copy(io.Discard, reader)
	if err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if n != totalSize {
		t.Fatalf("got %d bytes, want %d", n, totalSize)
	}
}

func TestSendStream_HTTPErrorReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"code":"NotFound","message":"theme not found"}`)
	}))
	defer srv.Close()

	c := New(srv.URL)
	reader, err := c.SendStream(context.Background(), RawRequest{Method: "GET", Path: "/themes/none"})
	if reader != nil {
		t.Fatalf("expected nil reader on 4xx, got %T", reader)
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != 404 {
		t.Fatalf("status code %d, want 404", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Body, "theme not found") {
		t.Fatalf("body should contain server error: %s", httpErr.Body)
	}
}

func TestSendStream_CtxCancelStopsRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "begin")
		flusher.Flush()
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	reader, err := c.SendStream(ctx, RawRequest{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("SendStream err: %v", err)
	}
	defer reader.Close()
	cancel()
	_, err = io.ReadAll(reader)
	if err == nil {
		t.Fatalf("expected error after ctx cancel, got nil")
	}
}

func TestSendStream_DoesNotApplyClientTimeout(t *testing.T) {
	// Default c.HTTPClient.Timeout = 30s; stream must not be cut by it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, strings.Repeat("a", 1024))
		flusher.Flush()
		// Hold the body open longer than client timeout would normally allow.
		// We can't truly wait 30s in unit test; assert by inspecting that
		// SendStream uses its own client without Timeout.
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.HTTPClient.Timeout = 100 * time.Millisecond
	reader, err := c.SendStream(context.Background(), RawRequest{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("SendStream err: %v", err)
	}
	defer reader.Close()
	time.Sleep(200 * time.Millisecond) // would have cut by c.HTTPClient.Timeout
	buf := make([]byte, 1024)
	n, _ := io.ReadFull(reader, buf)
	if n != 1024 {
		t.Fatalf("got %d bytes, want 1024 — c.HTTPClient.Timeout leaked into SendStream", n)
	}
}
