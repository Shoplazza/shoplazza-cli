package rawapi_test

import (
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/rawapi"
)

// ── NormalizePath ─────────────────────────────────────────────────────────────

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/openapi/2026-01/products", "/openapi/2026-01/products"},
		{"openapi/2026-01/products", "/openapi/2026-01/products"},
		{"", "/"},
		{"  ", "/"},
	}
	for _, tc := range cases {
		got := rawapi.NormalizePath(tc.input)
		if got != tc.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── ResolveTemplatedPath ──────────────────────────────────────────────────────

func TestResolveTemplatedPath_NoParams(t *testing.T) {
	path, remaining, err := rawapi.ResolveTemplatedPath("/openapi/products", nil)
	if err != nil {
		t.Fatalf("ResolveTemplatedPath: %v", err)
	}
	if path != "/openapi/products" {
		t.Errorf("path = %q, want /openapi/products", path)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %v, want empty", remaining)
	}
}

func TestResolveTemplatedPath_SingleParam(t *testing.T) {
	path, remaining, err := rawapi.ResolveTemplatedPath(
		"/openapi/products/{product_id}",
		map[string]any{"product_id": "gid_123", "title": "shoe"},
	)
	if err != nil {
		t.Fatalf("ResolveTemplatedPath: %v", err)
	}
	if !strings.Contains(path, "gid_123") {
		t.Errorf("path %q should contain gid_123", path)
	}
	if strings.Contains(path, "{product_id}") {
		t.Errorf("path %q should not contain template placeholder", path)
	}
	// title is a query param, not a path param
	if _, ok := remaining["title"]; !ok {
		t.Error("title should remain as query param")
	}
	// product_id should be consumed
	if _, ok := remaining["product_id"]; ok {
		t.Error("product_id should be consumed from params")
	}
}

func TestResolveTemplatedPath_MissingParam(t *testing.T) {
	_, _, err := rawapi.ResolveTemplatedPath("/openapi/products/{id}", nil)
	if err == nil {
		t.Error("expected error for missing path param")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error should mention 'id': %v", err)
	}
}

func TestResolveTemplatedPath_MultipleParams(t *testing.T) {
	path, _, err := rawapi.ResolveTemplatedPath(
		"/openapi/{service}/{id}",
		map[string]any{"service": "products", "id": "gid_001"},
	)
	if err != nil {
		t.Fatalf("ResolveTemplatedPath: %v", err)
	}
	if !strings.Contains(path, "products") || !strings.Contains(path, "gid_001") {
		t.Errorf("path %q missing substituted values", path)
	}
}

// ── BuildTemplatedRequest ─────────────────────────────────────────────────────

func TestBuildTemplatedRequest_SimpleGET(t *testing.T) {
	req, err := rawapi.BuildTemplatedRequest("GET", "/products", "", "", nil)
	if err != nil {
		t.Fatalf("BuildTemplatedRequest: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("Method = %q, want GET", req.Method)
	}
	if req.Path != "/products" {
		t.Errorf("Path = %q, want /products", req.Path)
	}
}

func TestBuildTemplatedRequest_WithPathParam(t *testing.T) {
	req, err := rawapi.BuildTemplatedRequest(
		"GET", "/products/{id}",
		`{"id":"gid_123"}`, "", nil,
	)
	if err != nil {
		t.Fatalf("BuildTemplatedRequest: %v", err)
	}
	if !strings.Contains(req.Path, "gid_123") {
		t.Errorf("path %q should contain gid_123", req.Path)
	}
}

func TestBuildTemplatedRequest_BothStdin(t *testing.T) {
	_, err := rawapi.BuildTemplatedRequest("POST", "/items", "-", "-", strings.NewReader(`{}`))
	if err == nil {
		t.Error("expected error when both params and data read from stdin")
	}
}

func TestBuildTemplatedRequest_InvalidParams(t *testing.T) {
	_, err := rawapi.BuildTemplatedRequest("GET", "/items", "not-json", "", nil)
	if err == nil {
		t.Error("expected error for invalid params JSON")
	}
}

func TestBuildTemplatedRequest_MissingPathParam(t *testing.T) {
	_, err := rawapi.BuildTemplatedRequest("GET", "/products/{id}", "", "", nil)
	if err == nil {
		t.Error("expected error for missing path param")
	}
}
