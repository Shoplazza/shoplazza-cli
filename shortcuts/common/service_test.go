package common_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/shortcuts/common"
)

func TestAPIPrefix_Constant(t *testing.T) {
	if common.APIPrefix != "/openapi/2026-01" {
		t.Errorf("APIPrefix = %q, want /openapi/2026-01", common.APIPrefix)
	}
}

func TestDryRun_EnvelopeShape(t *testing.T) {
	c := &client.Client{BaseURL: "https://example.test", Headers: map[string]string{}}
	p := common.PlannedRequest{Method: "GET", Path: "/openapi/2026-01/products", Query: map[string]any{"title": "foo"}}
	got := common.DryRun(c, p)

	if got["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", got["dry_run"])
	}
	req, ok := got["request"]
	if !ok {
		t.Fatalf("missing request key in %v", got)
	}
	b, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["method"] != "GET" || m["path"] != "/openapi/2026-01/products" {
		t.Errorf("request method/path mismatch: %v", m)
	}
}

func TestSend_DispatchesByMethod(t *testing.T) {
	var seenMethod, seenPath, seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &client.Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Headers: map[string]string{}}

	cases := []struct {
		name             string
		req              common.PlannedRequest
		wantMethod       string
		wantPath         string
		wantBodyContains string
	}{
		{"GET no query", common.PlannedRequest{Method: "GET", Path: "/x"}, "GET", "/x", ""},
		{"GET with query", common.PlannedRequest{Method: "GET", Path: "/x", Query: map[string]any{"a": "1"}}, "GET", "/x", ""},
		{"POST", common.PlannedRequest{Method: "POST", Path: "/x", Body: map[string]any{"k": "v"}}, "POST", "/x", `"k":"v"`},
		{"PUT", common.PlannedRequest{Method: "PUT", Path: "/x", Body: map[string]any{"k": "v"}}, "PUT", "/x", `"k":"v"`},
		{"DELETE", common.PlannedRequest{Method: "DELETE", Path: "/x"}, "DELETE", "/x", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seenMethod, seenPath, seenBody = "", "", ""
			out, err := common.Send(context.Background(), c, tc.req)
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if seenMethod != tc.wantMethod {
				t.Errorf("server saw method %q, want %q", seenMethod, tc.wantMethod)
			}
			if seenPath != tc.wantPath {
				t.Errorf("server saw path %q, want %q", seenPath, tc.wantPath)
			}
			if tc.wantBodyContains != "" && !contains(seenBody, tc.wantBodyContains) {
				t.Errorf("server saw body %q, want substring %q", seenBody, tc.wantBodyContains)
			}
			if out["ok"] != true {
				t.Errorf("Send returned %v, want {ok:true}", out)
			}
		})
	}
}

func TestSend_UnsupportedMethod(t *testing.T) {
	// PATCH/DELETE are now supported; TRACE is not routed by Send.
	c := &client.Client{BaseURL: "https://example.test", Headers: map[string]string{}}
	_, err := common.Send(context.Background(), c, common.PlannedRequest{Method: "TRACE", Path: "/x"})
	if err == nil {
		t.Error("expected error for unsupported method TRACE, got nil")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
