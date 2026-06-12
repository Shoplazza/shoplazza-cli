package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shoplazza-cli-v2/internal/client"
)

// helpers

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, client.New(srv.URL)
}

func jsonResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ── New / ResolveURL ──────────────────────────────────────────────────────────

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := client.New("http://example.com/")
	if c.BaseURL != "http://example.com" {
		t.Errorf("BaseURL = %q, want without trailing slash", c.BaseURL)
	}
}

func TestResolveURL_WithBase(t *testing.T) {
	c := client.New("http://api.example.com")
	got := c.ResolveURL("/openapi/2026-01/products")
	if got != "http://api.example.com/openapi/2026-01/products" {
		t.Errorf("ResolveURL = %q", got)
	}
}

func TestResolveURL_EmptyBase(t *testing.T) {
	c := client.New("")
	got := c.ResolveURL("/foo/bar")
	if got != "/foo/bar" {
		t.Errorf("ResolveURL(emptyBase) = %q, want /foo/bar", got)
	}
}

func TestResolveURL_NoLeadingSlash(t *testing.T) {
	c := client.New("http://api.example.com")
	got := c.ResolveURL("foo/bar")
	if !strings.HasPrefix(got, "http://api.example.com") {
		t.Errorf("ResolveURL missing base: %q", got)
	}
}

// ── BuildRequestSummary ───────────────────────────────────────────────────────

func TestBuildRequestSummary(t *testing.T) {
	c := client.New("http://api.example.com")
	sum := c.BuildRequestSummary("GET", "/products", nil, nil)
	if sum.Method != "GET" {
		t.Errorf("Method = %q", sum.Method)
	}
	if sum.Path != "/products" {
		t.Errorf("Path = %q", sum.Path)
	}
	if !strings.Contains(sum.URL, "/products") {
		t.Errorf("URL = %q, want to contain /products", sum.URL)
	}
}

// ── SetBearerToken ────────────────────────────────────────────────────────────

func TestSetBearerToken(t *testing.T) {
	var receivedToken string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("Access-Token")
		jsonResp(w, map[string]any{})
	})
	c.SetBearerToken("test-token-abc")

	var out map[string]any
	_ = c.GetJSON(context.Background(), "/", &out)
	if receivedToken != "test-token-abc" {
		t.Errorf("Access-Token = %q, want test-token-abc", receivedToken)
	}
}

func TestSetBearerToken_Empty(t *testing.T) {
	var receivedToken string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("Access-Token")
		jsonResp(w, map[string]any{})
	})
	c.SetBearerToken("") // no-op

	var out map[string]any
	_ = c.GetJSON(context.Background(), "/", &out)
	if receivedToken != "" {
		t.Errorf("empty token should not set header, got %q", receivedToken)
	}
}

// ── GetJSON ───────────────────────────────────────────────────────────────────

func TestGetJSON(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResp(w, map[string]any{"count": 42})
	})
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/count", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	// Client uses json.Decoder.UseNumber to preserve precision for snowflake-
	// style numeric IDs, so JSON numbers arrive as json.Number, not float64.
	if got, _ := out["count"].(json.Number); got.String() != "42" {
		t.Errorf("count = %v (%T), want json.Number 42", out["count"], out["count"])
	}
}

func TestGetJSON_HTTPError(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	var out map[string]any
	err := c.GetJSON(context.Background(), "/missing", &out)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	httpErr, ok := err.(*client.HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", httpErr.StatusCode)
	}
}

// ── GetJSONWithQuery ──────────────────────────────────────────────────────────

func TestGetJSONWithQuery_StringParam(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items", map[string]any{"status": "active"}, &out)
	if !strings.Contains(gotQuery, "status=active") {
		t.Errorf("query %q missing status=active", gotQuery)
	}
}

func TestGetJSONWithQuery_SliceParam(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items",
		map[string]any{"ids": []string{"a", "b"}}, &out)
	if !strings.Contains(gotQuery, "ids=a") || !strings.Contains(gotQuery, "ids=b") {
		t.Errorf("query %q missing ids=a and ids=b", gotQuery)
	}
}

func TestGetJSONWithQuery_AnySliceParam(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items",
		map[string]any{"tags": []any{"x", "y"}}, &out)
	if !strings.Contains(gotQuery, "tags=x") {
		t.Errorf("query %q missing tags=x", gotQuery)
	}
}

func TestGetJSONWithQuery_IntParam(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items",
		map[string]any{"page_size": 10}, &out)
	if !strings.Contains(gotQuery, "page_size=10") {
		t.Errorf("query %q missing page_size=10", gotQuery)
	}
}

func TestGetJSONWithQuery_NilParam_Skipped(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items",
		map[string]any{"skip_me": nil, "keep": "yes"}, &out)
	if strings.Contains(gotQuery, "skip_me") {
		t.Errorf("nil param should be skipped, query=%q", gotQuery)
	}
	if !strings.Contains(gotQuery, "keep=yes") {
		t.Errorf("keep=yes should be present, query=%q", gotQuery)
	}
}

func TestGetJSONWithQuery_EmptyStringParam_Skipped(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items",
		map[string]any{"empty": "", "keep": "yes"}, &out)
	if strings.Contains(gotQuery, "empty") {
		t.Errorf("empty string param should be skipped, query=%q", gotQuery)
	}
}

// ── PostJSON ──────────────────────────────────────────────────────────────────

func TestPostJSON(t *testing.T) {
	var received map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		jsonResp(w, map[string]any{"id": "new-1"})
	})
	var out map[string]any
	err := c.PostJSON(context.Background(), "/items", map[string]any{"name": "foo"}, &out)
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if out["id"] != "new-1" {
		t.Errorf("id = %v, want new-1", out["id"])
	}
	if received["name"] != "foo" {
		t.Errorf("sent name = %v, want foo", received["name"])
	}
}

// ── PutJSON ───────────────────────────────────────────────────────────────────

func TestPutJSON(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		jsonResp(w, map[string]any{"updated": true})
	})
	var out map[string]any
	if err := c.PutJSON(context.Background(), "/items/1", map[string]any{"name": "bar"}, &out); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}
	if out["updated"] != true {
		t.Errorf("updated = %v, want true", out["updated"])
	}
}

// ── DeleteJSON ────────────────────────────────────────────────────────────────

func TestDeleteJSON_EmptyBody(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})
	var out map[string]any
	if err := c.DeleteJSON(context.Background(), "/items/1", &out); err != nil {
		t.Fatalf("DeleteJSON: %v", err)
	}
}

func TestDeleteJSON_WithBody(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, map[string]any{"deleted": true})
	})
	var out map[string]any
	if err := c.DeleteJSON(context.Background(), "/items/1", &out); err != nil {
		t.Fatalf("DeleteJSON: %v", err)
	}
	if out["deleted"] != true {
		t.Errorf("deleted = %v, want true", out["deleted"])
	}
}

// ── DoRaw ─────────────────────────────────────────────────────────────────────

func TestDoRaw_GET(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResp(w, map[string]any{"items": []any{}})
	})
	resp, err := c.DoRaw(context.Background(), client.RawRequest{
		Method: "GET",
		Path:   "/items",
	})
	if err != nil {
		t.Fatalf("DoRaw GET: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestDoRaw_POST(t *testing.T) {
	var received map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		jsonResp(w, map[string]any{"id": "x"})
	})
	resp, err := c.DoRaw(context.Background(), client.RawRequest{
		Method: "POST",
		Path:   "/items",
		Data:   map[string]any{"key": "val"},
	})
	if err != nil {
		t.Fatalf("DoRaw POST: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if received["key"] != "val" {
		t.Errorf("sent key = %v, want val", received["key"])
	}
}

func TestDoRaw_WithQueryParams(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	_, err := c.DoRaw(context.Background(), client.RawRequest{
		Method: "GET",
		Path:   "/items",
		Params: map[string]any{"status": "active"},
	})
	if err != nil {
		t.Fatalf("DoRaw with params: %v", err)
	}
	if !strings.Contains(gotQuery, "status=active") {
		t.Errorf("query %q missing status=active", gotQuery)
	}
}

func TestDoRaw_HTTPError(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"crash"}`))
	})
	_, err := c.DoRaw(context.Background(), client.RawRequest{Method: "GET", Path: "/"})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	httpErr, ok := err.(*client.HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Error(), "500") {
		t.Errorf("Error() = %q, want to contain 500", httpErr.Error())
	}
	// The error must name the failing endpoint so a server 500 is not anonymous.
	if httpErr.Method != "GET" || httpErr.Path != "/" {
		t.Errorf("HTTPError endpoint = %q %q, want GET /", httpErr.Method, httpErr.Path)
	}
	if !strings.Contains(httpErr.Error(), "GET") || !strings.Contains(httpErr.Error(), "/") {
		t.Errorf("Error() = %q, want to name the method+path", httpErr.Error())
	}
}

// TestHTTPError_CarriesEndpoint_AllPaths verifies every HTTPError construction
// path (doJSON via GetJSON, DoRaw, SendStream) stamps Method+Path, so a 500
// from any client entry point self-identifies its endpoint.
func TestHTTPError_CarriesEndpoint_AllPaths(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":"ServerError"}`))
	})
	assertEndpoint := func(t *testing.T, err error, wantMethod, wantPath string) {
		t.Helper()
		var he *client.HTTPError
		if !errors.As(err, &he) {
			t.Fatalf("expected *HTTPError, got %T (%v)", err, err)
		}
		if he.Method != wantMethod || he.Path != wantPath {
			t.Errorf("endpoint = %q %q, want %q %q", he.Method, he.Path, wantMethod, wantPath)
		}
	}
	// doJSON path (GetJSON)
	var out map[string]any
	assertEndpoint(t, c.GetJSON(context.Background(), "/themes/task/abc", &out), "GET", "/themes/task/abc")
	// DoRaw path
	_, err := c.DoRaw(context.Background(), client.RawRequest{Method: "post", Path: "/themes/upload"})
	assertEndpoint(t, err, "POST", "/themes/upload")
	// SendStream path
	_, err = c.SendStream(context.Background(), client.RawRequest{Method: "GET", Path: "/themes/x/download"})
	assertEndpoint(t, err, "GET", "/themes/x/download")
}

func TestDoRaw_PlainTextBody(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain response"))
	})
	resp, err := c.DoRaw(context.Background(), client.RawRequest{Method: "GET", Path: "/"})
	if err != nil {
		t.Fatalf("DoRaw plain text: %v", err)
	}
	if resp.Body != "plain response" {
		t.Errorf("Body = %v, want 'plain response'", resp.Body)
	}
}

func TestDoRaw_EmptyBody(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	resp, err := c.DoRaw(context.Background(), client.RawRequest{Method: "DELETE", Path: "/items/1"})
	if err != nil {
		t.Fatalf("DoRaw empty body: %v", err)
	}
	if resp.Body != nil {
		t.Errorf("Body = %v, want nil for empty response", resp.Body)
	}
}

// ── Envelope unwrapping ───────────────────────────────────────────────────────

func TestGetJSON_UnwrapsDataEnvelope(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, map[string]any{
			"code": "Success",
			"data": map[string]any{"id": "unwrapped"},
		})
	})
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out["id"] != "unwrapped" {
		t.Errorf("envelope unwrap: id = %v, want 'unwrapped'", out["id"])
	}
}

func TestGetJSON_NoEnvelope_PassThrough(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, map[string]any{"products": []any{}})
	})
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out["products"] == nil {
		t.Error("products field missing in non-envelope response")
	}
}

func TestGetJSON_Nil_Out(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// nil out pointer — should not error on empty body
	if err := c.GetJSON(context.Background(), "/", nil); err != nil {
		t.Fatalf("GetJSON nil out: %v", err)
	}
}

// ── Edge cases ────────────────────────────────────────────────────────────────

// DoRaw: invalid JSON with JSON content-type.
func TestDoRaw_InvalidJSONBody(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{this is not valid JSON`))
	})
	_, err := c.DoRaw(context.Background(), client.RawRequest{Method: "GET", Path: "/"})
	if err == nil {
		t.Error("expected error for invalid JSON in JSON content-type response")
	}
}

// DoRaw: a non-2xx whose body fails to parse (JSON content-type but HTML/
// truncated payload) must still yield *HTTPError — otherwise the status code
// is lost and a 403 cannot be reclassified to auth.
func TestDoRaw_NonJSONErrorBody_403(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<html>denied</html>`))
	})
	_, err := c.DoRaw(context.Background(), client.RawRequest{Method: "GET", Path: "/secure"})
	var he *client.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T (%v)", err, err)
	}
	if he.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want 403", he.StatusCode)
	}
	if he.Body != `<html>denied</html>` {
		t.Errorf("Body = %q, want raw body", he.Body)
	}
	if he.Method != "GET" || he.Path != "/secure" {
		t.Errorf("endpoint = %q %q, want GET /secure", he.Method, he.Path)
	}
}

func TestDoRaw_NonJSONErrorBody_500(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"truncated":`))
	})
	_, err := c.DoRaw(context.Background(), client.RawRequest{Method: "POST", Path: "/items"})
	var he *client.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T (%v)", err, err)
	}
	if he.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", he.StatusCode)
	}
	if he.Body != `{"truncated":` {
		t.Errorf("Body = %q, want raw body", he.Body)
	}
}

// GetJSON: envelope with code=Success but no data key → body passes through.
func TestGetJSON_EnvelopeNoDataKey(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"Success","other":"field"}`))
	})
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/", &out); err != nil {
		t.Fatalf("GetJSON envelope no data: %v", err)
	}
	if out["code"] != "Success" {
		t.Errorf("code = %v, want 'Success' (no-data envelope should pass through)", out["code"])
	}
}

// GetJSON: envelope code not "Success" passes through unchanged.
func TestGetJSON_EnvelopeNonSuccessCode(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"Error","data":{"id":"x"}}`))
	})
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/", &out); err != nil {
		t.Fatalf("GetJSON non-success code: %v", err)
	}
	if out["code"] != "Error" {
		t.Errorf("non-Success envelope should pass through; code = %v", out["code"])
	}
}

// GetJSONWithQuery: empty []string items are skipped.
func TestGetJSONWithQuery_EmptySliceItems_Skipped(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	_ = c.GetJSONWithQuery(context.Background(), "/items",
		map[string]any{"ids": []string{"", "b", ""}}, &out)
	if gotQuery != "ids=b" {
		t.Errorf("query %q should be ids=b (empty items skipped)", gotQuery)
	}
}

// GetJSON: non-array/non-map JSON passes through unmarshalUnwrapped.
func TestGetJSON_ArrayResponse(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[1,2,3]`))
	})
	var out []int
	if err := c.GetJSON(context.Background(), "/", &out); err != nil {
		t.Fatalf("GetJSON array response: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("array len = %d, want 3", len(out))
	}
}

// ── JSON marshal-error paths ──────────────────────────────────────────────────
// A channel cannot be marshaled by encoding/json — these exercise the
// json.Marshal error path in doJSON and DoRaw.

func TestPostJSON_UnmarshalablePayload(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be reached with unmarshalable payload")
		w.WriteHeader(200)
	})
	var out map[string]any
	err := c.PostJSON(context.Background(), "/items", make(chan int), &out)
	if err == nil {
		t.Error("expected error marshaling channel payload")
	}
}

func TestDoRaw_UnmarshalableData(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be reached with unmarshalable data")
		w.WriteHeader(200)
	})
	_, err := c.DoRaw(context.Background(), client.RawRequest{
		Method: "POST",
		Path:   "/items",
		Data:   make(chan int), // cannot be marshaled by encoding/json
	})
	if err == nil {
		t.Error("expected error marshaling channel data")
	}
}

// TestDoRaw_NoTimeoutBypassesClientTimeout: theme zip uploads can outlive the
// client-wide Timeout. With NoTimeout the per-call client strips the global
// timeout (ctx still governs cancellation); without it the same slow request
// must keep failing — proving the flag is what changes behavior.
func TestDoRaw_NoTimeoutBypassesClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // longer than the 50ms client timeout
		jsonResp(w, map[string]any{"ok": true})
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)
	c.HTTPClient.Timeout = 50 * time.Millisecond

	if _, err := c.DoRaw(context.Background(), client.RawRequest{
		Method: "GET", Path: "/slow",
	}); err == nil {
		t.Fatal("unflagged request should hit the 50ms client timeout")
	}

	if _, err := c.DoRaw(context.Background(), client.RawRequest{
		Method: "GET", Path: "/slow", NoTimeout: true,
	}); err != nil {
		t.Fatalf("NoTimeout request must bypass the client timeout: %v", err)
	}
}

// TestDoRaw_NoTimeoutStillHonorsContext: NoTimeout trades the global timeout
// for ctx-driven cancellation — a canceled ctx must still abort the request.
func TestDoRaw_NoTimeoutStillHonorsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		jsonResp(w, map[string]any{"ok": true})
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.DoRaw(ctx, client.RawRequest{
		Method: "GET", Path: "/slow", NoTimeout: true,
	}); err == nil {
		t.Fatal("ctx deadline must still abort a NoTimeout request")
	}
}

// ── PatchJSON ─────────────────────────────────────────────────────────────────

func TestPatchJSON_SendsCorrectMethod(t *testing.T) {
	var gotMethod string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		jsonResp(w, map[string]any{"id": 42})
	})
	var out map[string]any
	if err := c.PatchJSON(context.Background(), "/items/1", map[string]any{"name": "x"}, &out); err != nil {
		t.Fatalf("PatchJSON: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
}

// ── DeleteJSONWithQuery ───────────────────────────────────────────────────────

func TestDeleteJSONWithQuery_SendsQueryParams(t *testing.T) {
	var gotQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		jsonResp(w, map[string]any{})
	})
	var out map[string]any
	q := map[string]any{"force": true}
	if err := c.DeleteJSONWithQuery(context.Background(), "/items/1", q, &out); err != nil {
		t.Fatalf("DeleteJSONWithQuery: %v", err)
	}
	if !strings.Contains(gotQuery, "force") {
		t.Errorf("query params missing 'force': %s", gotQuery)
	}
}

// ── RawResponse.RequestID ─────────────────────────────────────────────────────

func TestRawResponse_RequestID_Present(t *testing.T) {
	r := client.RawResponse{Headers: map[string][]string{"Request-Id": {"req-abc-123"}}}
	if got := r.RequestID(); got != "req-abc-123" {
		t.Errorf("RequestID() = %q, want req-abc-123", got)
	}
}

func TestRawResponse_RequestID_Absent(t *testing.T) {
	r := client.RawResponse{}
	if got := r.RequestID(); got != "" {
		t.Errorf("RequestID() = %q, want empty string when header absent", got)
	}
}
