package common

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

func TestSendStream_MapsPlannedRequest(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, "binary-bytes")
	}))
	defer srv.Close()
	c := client.New(srv.URL)

	p := PlannedRequest{Method: "GET", Path: "/themes/abc/download", Query: map[string]any{"key": "v"}}
	reader, err := SendStream(context.Background(), c, p)
	if err != nil {
		t.Fatalf("SendStream err: %v", err)
	}
	defer reader.Close()
	b, _ := io.ReadAll(reader)
	if string(b) != "binary-bytes" {
		t.Fatalf("got body %q", b)
	}
	if gotMethod != "GET" || !strings.HasSuffix(gotPath, "/themes/abc/download") || !strings.Contains(gotQuery, "key=v") {
		t.Fatalf("PlannedRequest mapping failed: method=%s path=%s query=%s", gotMethod, gotPath, gotQuery)
	}
}

func TestSendStream_MapsBodyField(t *testing.T) {
	var gotBody string
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	c := client.New(srv.URL)

	p := PlannedRequest{Method: "POST", Path: "/x", Body: map[string]any{"k": "v"}}
	reader, err := SendStream(context.Background(), c, p)
	if err != nil {
		t.Fatalf("SendStream err: %v", err)
	}
	defer reader.Close()
	_, _ = io.ReadAll(reader)

	if !strings.Contains(gotBody, `"k":"v"`) {
		t.Errorf("Body field not mapped to JSON-marshalled request body; got %q", gotBody)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}
