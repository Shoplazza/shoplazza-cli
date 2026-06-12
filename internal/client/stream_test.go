package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
